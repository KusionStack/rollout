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

package rolloutrun

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/apis/rollout/v1alpha1/condition"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/utils/expectations"
	"kusionstack.io/rollout/pkg/workload"
	workloadregistry "kusionstack.io/rollout/pkg/workload/registry"
)

const (
	ControllerName = "rolloutrun-controller"
)

// RolloutRunReconciler reconciles a Rollout object
type RolloutRunReconciler struct {
	*mixin.ReconcilerMixin

	workloadRegistry workloadregistry.Registry

	rvExpectation expectations.ResourceVersionExpectationInterface
}

func NewReconciler(mgr manager.Manager, workloadRegistry workloadregistry.Registry) *RolloutRunReconciler {
	return &RolloutRunReconciler{
		ReconcilerMixin:  mixin.NewReconcilerMixin(ControllerName, mgr),
		workloadRegistry: workloadRegistry,
		rvExpectation:    expectations.NewResourceVersionExpectation(),
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *RolloutRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.workloadRegistry == nil {
		return fmt.Errorf("workload manager must be set")
	}

	b := ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 5}).
		For(&rolloutv1alpha1.RolloutRun{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}))

	_, err := b.Build(r)
	return err
}

//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=rolloutruns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=rolloutruns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=rolloutruns/finalizers,verbs=update
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=rolloutstrategies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RolloutRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	key := req.String()

	logger := r.Logger.WithValues(
		"rolloutRun", key, "traceId", uuid.New().String(),
	)

	logger.V(2).Info("start reconciling rolloutRun")
	defer logger.V(2).Info("finish reconciling rolloutRun")

	obj := &rolloutv1alpha1.RolloutRun{}
	err := r.Client.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), req.NamespacedName, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if !r.satisfiedExpectations(obj) {
		return reconcile.Result{}, nil
	}

	if err = r.handleFinalizer(obj); err != nil {
		logger.Error(err, "handleFinalizer failed")
		return ctrl.Result{}, nil
	}

	newStatus := obj.Status.DeepCopy()

	workloads, err := r.findWorkloadsCrossCluster(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}

	var result ctrl.Result
	cmd := obj.Annotations[rollout.AnnoManualCommandKey]
	result, err = r.syncRolloutRun(ctx, obj, newStatus)

	updateStatus := r.updateStatusOnly(ctx, obj, newStatus, workloads)
	if updateStatus != nil {
		logger.Error(err, "failed to update status")
		return reconcile.Result{}, updateStatus
	}

	if _, exist := obj.Annotations[rollout.AnnoManualCommandKey]; !exist && len(cmd) > 0 {
		patch := utils.DeleteAnnotation(types.JSONPatchType, rollout.AnnoManualCommandKey)
		if patchError := r.Client.Patch(clusterinfo.ContextFed, obj, patch); patchError != nil {
			logger.Error(err, "failed to patch status")
			return reconcile.Result{}, patchError
		}
	}

	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
}

func (r *RolloutRunReconciler) satisfiedExpectations(instance *rolloutv1alpha1.RolloutRun) bool {
	key := utils.ObjectKeyString(instance)
	logger := r.Logger.WithValues("rolloutRun", key)

	if !r.rvExpectation.SatisfiedExpectations(key, instance.ResourceVersion) {
		logger.Info("rolloutRun does not statisfy resourceVersion expectation, skip reconciling")
		return false
	}

	return true
}

func (r *RolloutRunReconciler) handleFinalizer(rolloutRun *rolloutv1alpha1.RolloutRun) error {
	if rolloutRun.DeletionTimestamp.IsZero() {
		if err := utils.AddAndUpdateFinalizer(r.Client, rolloutRun, rollout.FinalizerRolloutProtection); err != nil {
			return err
		}
		return nil
	}

	if rolloutRun.IsCompleted() {
		if err := utils.RemoveAndUpdateFinalizer(r.Client, rolloutRun, rollout.FinalizerRolloutProtection); err != nil {
			return err
		}
		return nil
	}

	return nil
}

func (r *RolloutRunReconciler) syncRolloutRun(ctx context.Context, obj *rolloutv1alpha1.RolloutRun, newStatus *rolloutv1alpha1.RolloutRunStatus) (ctrl.Result, error) {
	key := utils.ObjectKeyString(obj)
	logger := r.Logger.WithValues("rolloutRun", key)

	rollout := &rolloutv1alpha1.Rollout{}
	if err := r.findRollout(ctx, obj, rollout); err != nil {
		return ctrl.Result{}, err
	}

	var (
		done   bool
		err    error
		result ctrl.Result
	)
	defaultExecutor := executor.NewDefaultExecutor(logger)
	executorCtx := &executor.ExecutorContext{
		Rollout: rollout, RolloutRun: obj, NewStatus: newStatus,
	}
	if done, result, err = defaultExecutor.Do(ctx, executorCtx); err != nil {
		logger.Error(err, "defaultExecutor do err")
		return ctrl.Result{}, err
	}
	if done {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionFalse,
			rolloutv1alpha1.RolloutReasonProgressingCompleted,
			"rollout is completed",
		)
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseSucceeded
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	} else if newStatus.Error != nil {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionFalse,
			rolloutv1alpha1.RolloutReasonProgressingError,
			"rollout stop rolling since error exist",
		)
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	} else if newStatus.Phase == rolloutv1alpha1.RolloutRunPhaseCanceled {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionFalse,
			rolloutv1alpha1.RolloutReasonProgressingCanceled,
			"rollout is canceled",
		)
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	}
	return result, nil
}

// findRollout get Rollout from rolloutRun
func (r *RolloutRunReconciler) findRollout(ctx context.Context, rolloutRun *rolloutv1alpha1.RolloutRun, rollout *rolloutv1alpha1.Rollout) error {
	ref, exist := utils.Find(
		rolloutRun.OwnerReferences,
		func(o *metav1.OwnerReference) bool {
			return o.Kind == "Rollout"
		},
	)
	if !exist {
		objectKey := types.NamespacedName{Namespace: rolloutRun.Namespace, Name: rolloutRun.Name}
		return fmt.Errorf("rollout not exist in rolloutRun(%v) ownerReferences", objectKey)
	}

	objectKey := types.NamespacedName{Namespace: rolloutRun.Namespace, Name: ref.Name}
	if err := r.Client.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), objectKey, rollout); err != nil {
		return err
	}

	return nil
}

func (r *RolloutRunReconciler) findWorkloadsCrossCluster(ctx context.Context, obj *rolloutv1alpha1.RolloutRun) (*workload.Set, error) {
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
	store, err := r.workloadRegistry.Get(gvk)
	if err != nil {
		return nil, err
	}
	list, err := store.List(ctx, obj.GetNamespace(), match)
	if err != nil {
		return nil, err
	}
	return workload.NewWorkloadSet(list...), nil
}

func (r *RolloutRunReconciler) syncWorkloadStatus(newStatus *rolloutv1alpha1.RolloutRunStatus, workloads *workload.Set) {
	allWorkloads := workloads.ToSlice()
	sort.Slice(allWorkloads, func(i, j int) bool {
		iInfo := allWorkloads[i].GetInfo()
		jInfo := allWorkloads[j].GetInfo()

		if iInfo.Cluster == jInfo.Cluster {
			return iInfo.Name < jInfo.Name
		}

		return iInfo.Cluster < jInfo.Cluster
	})
	workloadStatuses := make([]rolloutv1alpha1.RolloutWorkloadStatus, len(allWorkloads))
	for i, w := range allWorkloads {
		workloadStatuses[i] = w.GetStatus()
	}
	newStatus.TargetStatuses = workloadStatuses
}

func (r *RolloutRunReconciler) updateStatusOnly(ctx context.Context, instance *rolloutv1alpha1.RolloutRun, newStatus *rolloutv1alpha1.RolloutRunStatus, workloads *workload.Set) error {
	// generate workload status
	r.syncWorkloadStatus(newStatus, workloads)

	if equality.Semantic.DeepEqual(instance.Status, *newStatus) {
		// no change
		return nil
	}
	key := utils.ObjectKeyString(instance)
	now := metav1.Now()
	newStatus.LastUpdateTime = &now
	_, err := utils.UpdateOnConflict(clusterinfo.WithCluster(ctx, clusterinfo.Fed), r.Client, r.Client.Status(), instance, func() error {
		instance.Status = *newStatus
		instance.Status.ObservedGeneration = instance.Generation
		return nil
	})

	if err != nil {
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, "FailedUpdateStatus", "failed to update rolloutRun %q status: %v", key, err)
		r.Logger.Error(err, "failed to update rolloutRun status", "rolloutRun", key)
		return err
	}

	r.Logger.V(2).Info("succeed to update rolloutRun status", "rolloutRun", key)
	r.rvExpectation.ExpectUpdate(key, instance.ResourceVersion) // nolint
	return nil
}
