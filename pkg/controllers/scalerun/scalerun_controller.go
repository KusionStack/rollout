// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scalerun

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/kube-api/rollout/v1alpha1/condition"
	kubeutilclient "kusionstack.io/kube-utils/client"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/controllers/scalerun/executor"
	"kusionstack.io/rollout/pkg/features/rolloutclasspredicate"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/utils/expectations"
	"kusionstack.io/rollout/pkg/workload"
)

const (
	ControllerName = "scalerun"
)

// ScaleRunReconciler reconciles a Rollout object
type ScaleRunReconciler struct {
	*mixin.ReconcilerMixin

	workloadRegistry registry.WorkloadRegistry

	rvExpectation expectations.ResourceVersionExpectationInterface

	executor *executor.Executor
}

func NewReconciler(mgr manager.Manager, workloadRegistry registry.WorkloadRegistry) *ScaleRunReconciler {
	r := &ScaleRunReconciler{
		ReconcilerMixin:  mixin.NewReconcilerMixin(ControllerName, mgr),
		workloadRegistry: workloadRegistry,
		rvExpectation:    expectations.NewResourceVersionExpectation(),
	}

	r.executor = executor.NewDefaultExecutor(r.Logger)
	return r
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScaleRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.workloadRegistry == nil {
		return fmt.Errorf("workload manager must be set")
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&rolloutv1alpha1.ScaleRun{},
			builder.WithPredicates(
				predicate.ResourceVersionChangedPredicate{},
				// NOTE: This controller only watches one kind of resource,
				// so we can use predicate to filter events by rollout-class
				rolloutclasspredicate.RolloutClassMatchesPredicate,
			))

	_, err := b.Build(r)
	return err
}

//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=scaleruns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=scaleruns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=scaleruns/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ScaleRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.WithValues("scalerun", req.String())
	logger.V(4).Info("started reconciling scalerun")
	defer logger.V(4).Info("finished reconciling scalerun")

	obj := &rolloutv1alpha1.ScaleRun{}
	err := r.Client.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), req.NamespacedName, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			r.rvExpectation.DeleteExpectations(req.String())
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if !r.satisfiedExpectations(obj) {
		return reconcile.Result{}, nil
	}

	if err = r.handleFinalizer(ctx, obj); err != nil {
		logger.Error(err, "scalerun handleFinalizer failed")
		return ctrl.Result{}, nil
	}

	if obj.IsCompleted() {
		// scaleRun is completed, skip syncing
		return reconcile.Result{}, nil
	}

	newStatus := obj.Status.DeepCopy()

	accessor, workloads, err := r.findWorkloadsCrossCluster(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}

	var result ctrl.Result
	result, err = r.syncScaleRun(ctx, obj, newStatus, accessor, workloads)

	if tempErr := r.cleanupAnnotation(ctx, obj); tempErr != nil {
		logger.Error(tempErr, "failed to clean up scalerun annotation")
	}

	updateStatus := r.updateStatusOnly(ctx, obj, newStatus)
	if updateStatus != nil {
		logger.Error(updateStatus, "failed to update scalerun status")
		return reconcile.Result{}, updateStatus
	}

	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
}

func (r *ScaleRunReconciler) satisfiedExpectations(instance *rolloutv1alpha1.ScaleRun) bool {
	key := utils.ObjectKeyString(instance)
	logger := r.Logger.WithValues("scalerun", key)

	if !r.rvExpectation.SatisfiedExpectations(key, instance.ResourceVersion) {
		logger.Info("scalerun does not statisfy resourceVersion expectation, skip reconciling")
		return false
	}

	return true
}

func (r *ScaleRunReconciler) handleFinalizer(ctx context.Context, scaleRun *rolloutv1alpha1.ScaleRun) error {
	if scaleRun.IsCompleted() {
		// remove finalizer when scaleRun is completed
		if err := kubeutilclient.RemoveFinalizerAndUpdate(ctx, r.Client, scaleRun, rollout.FinalizerRolloutProtection); err != nil {
			return err
		}
	} else if scaleRun.DeletionTimestamp.IsZero() {
		// add finalizer when scaleRun is not completed and not deleted
		if err := kubeutilclient.AddFinalizerAndUpdate(ctx, r.Client, scaleRun, rollout.FinalizerRolloutProtection); err != nil {
			return err
		}
	}

	return nil
}

func (r *ScaleRunReconciler) cleanupAnnotation(ctx context.Context, obj *rolloutv1alpha1.ScaleRun) error {
	// delete manual command annotations from rollout
	_, err := kubeutilclient.UpdateOnConflict(clusterinfo.WithCluster(ctx, clusterinfo.Fed), r.Client, r.Client, obj, func(in *rolloutv1alpha1.ScaleRun) error {
		delete(in.Annotations, rollout.AnnoManualCommandKey)
		return nil
	})
	if err != nil {
		return err
	}
	key := utils.ObjectKeyString(obj)
	r.rvExpectation.ExpectUpdate(key, obj.ResourceVersion) // nolint
	return nil
}

func (r *ScaleRunReconciler) syncScaleRun(
	ctx context.Context,
	obj *rolloutv1alpha1.ScaleRun,
	newStatus *rolloutv1alpha1.ScaleRunStatus,
	accesor workload.Accessor,
	workloads *workload.Set,
) (ctrl.Result, error) {
	var (
		done   bool
		result ctrl.Result
		err    error
	)

	executorCtx := &executor.ExecutorContext{
		Context:   ctx,
		Client:    r.Client,
		Recorder:  r.Recorder,
		Accessor:  accesor,
		ScaleRun:  obj,
		NewStatus: newStatus,
		Workloads: workloads,
	}
	if done, result, err = r.executor.Do(executorCtx); err != nil {
		return ctrl.Result{}, err
	}
	if done || newStatus.Phase == rolloutv1alpha1.RolloutRunPhaseSucceeded {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionFalse,
			rolloutv1alpha1.RolloutReasonProgressingCompleted,
			"scaleRun is completed",
		)
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	} else if newStatus.Phase == rolloutv1alpha1.RolloutRunPhaseCanceled {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionFalse,
			rolloutv1alpha1.RolloutReasonProgressingCanceled,
			"scaleRun is canceled",
		)
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	} else if newStatus.Error != nil {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionFalse,
			rolloutv1alpha1.RolloutReasonProgressingError,
			"scaleRun stop rolling since error exist",
		)
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	} else {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionTrue,
			rolloutv1alpha1.RolloutReasonProgressingRunning,
			"scaleRun is running",
		)
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	}
	return result, nil
}

func (r *ScaleRunReconciler) findWorkloadsCrossCluster(ctx context.Context, obj *rolloutv1alpha1.ScaleRun) (workload.Accessor, *workload.Set, error) {
	all := make([]rolloutv1alpha1.CrossClusterObjectNameReference, 0)

	for _, b := range obj.Spec.Batch.Batches {
		for _, t := range b.Targets {
			all = append(all, t.CrossClusterObjectNameReference)
		}
	}
	match := rolloutv1alpha1.ResourceMatch{
		Names: all,
	}

	gvk := schema.FromAPIVersionAndKind(obj.Spec.TargetType.APIVersion, obj.Spec.TargetType.Kind)
	accessor, err := r.workloadRegistry.Get(gvk)
	if err != nil {
		return nil, nil, err
	}

	list, _, err := workload.List(ctx, r.Client, accessor, obj.Namespace, match)
	if err != nil {
		return nil, nil, err
	}
	return accessor, workload.NewSet(list...), nil
}

func (r *ScaleRunReconciler) updateStatusOnly(ctx context.Context, obj *rolloutv1alpha1.ScaleRun, newStatus *rolloutv1alpha1.ScaleRunStatus) error {
	if equality.Semantic.DeepEqual(obj.Status, *newStatus) {
		// no change
		return nil
	}
	key := utils.ObjectKeyString(obj)
	now := metav1.Now()
	newStatus.LastUpdateTime = &now
	_, err := kubeutilclient.UpdateOnConflict(clusterinfo.WithCluster(ctx, clusterinfo.Fed), r.Client, r.Client.Status(), obj, func(in *rolloutv1alpha1.ScaleRun) error {
		in.Status = *newStatus
		in.Status.ObservedGeneration = in.Generation
		return nil
	})
	if err != nil {
		r.Recorder.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateStatus", "failed to update scaleRun %q status: %v", key, err)
		r.Logger.Error(err, "failed to update scaleRun status", "scaleRun", key)
		return err
	}

	r.rvExpectation.ExpectUpdate(key, obj.ResourceVersion) // nolint
	return nil
}
