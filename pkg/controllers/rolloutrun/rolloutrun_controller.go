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
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
	"kusionstack.io/rollout/pkg/features/rolloutclasspredicate"
	trafficcontrol "kusionstack.io/rollout/pkg/trafficrouting/control"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/utils/expectations"
	"kusionstack.io/rollout/pkg/workload"
)

const (
	ControllerName = "rolloutrun"
)

// RolloutRunReconciler reconciles a Rollout object
type RolloutRunReconciler struct {
	*mixin.ReconcilerMixin

	workloadRegistry registry.WorkloadRegistry

	rvExpectation expectations.ResourceVersionExpectationInterface

	executor *executor.Executor
}

func NewReconciler(mgr manager.Manager, workloadRegistry registry.WorkloadRegistry) *RolloutRunReconciler {
	r := &RolloutRunReconciler{
		ReconcilerMixin:  mixin.NewReconcilerMixin(ControllerName, mgr),
		workloadRegistry: workloadRegistry,
		rvExpectation:    expectations.NewResourceVersionExpectation(),
	}

	r.executor = executor.NewDefaultExecutor(r.Logger)
	return r
}

// SetupWithManager sets up the controller with the Manager.
func (r *RolloutRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.workloadRegistry == nil {
		return fmt.Errorf("workload manager must be set")
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&rolloutv1alpha1.RolloutRun{},
			builder.WithPredicates(
				predicate.ResourceVersionChangedPredicate{},
				// NOTE: This controller only watches one kind of resource,
				// so we can use predicate to filter events by rollout-class
				rolloutclasspredicate.RolloutClassMatchesPredicate,
			))

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
	logger := r.Logger.WithValues("rolloutRun", req.String())
	logger.V(4).Info("started reconciling rolloutRun")
	defer logger.V(4).Info("finished reconciling rolloutRun")

	obj := &rolloutv1alpha1.RolloutRun{}
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
		logger.Error(err, "handleFinalizer failed")
		return ctrl.Result{}, nil
	}

	if obj.IsCompleted() {
		// rolloutRun is completed, skip syncing
		return reconcile.Result{}, nil
	}

	newStatus := obj.Status.DeepCopy()

	accessor, workloads, canaryWorkloads, err := r.findWorkloadsCrossCluster(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}

	var result ctrl.Result
	result, err = r.syncRolloutRun(ctx, obj, newStatus, accessor, workloads)

	if tempErr := r.cleanupAnnotation(ctx, obj); tempErr != nil {
		logger.Error(tempErr, "failed to clean up annotation")
	}

	updateStatus := r.updateStatusOnly(ctx, obj, newStatus, workloads, canaryWorkloads)
	if updateStatus != nil {
		logger.Error(updateStatus, "failed to update status")
		return reconcile.Result{}, updateStatus
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

func (r *RolloutRunReconciler) handleFinalizer(ctx context.Context, rolloutRun *rolloutv1alpha1.RolloutRun) error {
	if rolloutRun.IsCompleted() {
		// remove finalizer when rolloutRun is completed
		if err := kubeutilclient.RemoveFinalizerAndUpdate(ctx, r.Client, rolloutRun, rollout.FinalizerRolloutProtection); err != nil {
			return err
		}
	} else if rolloutRun.DeletionTimestamp.IsZero() {
		// add finalizer when rolloutRun is not completed and not deleted
		if err := kubeutilclient.AddFinalizerAndUpdate(ctx, r.Client, rolloutRun, rollout.FinalizerRolloutProtection); err != nil {
			return err
		}
	}

	return nil
}

func (r *RolloutRunReconciler) cleanupAnnotation(ctx context.Context, obj *rolloutv1alpha1.RolloutRun) error {
	// delete manual command annotations from rollout
	_, err := kubeutilclient.UpdateOnConflict(clusterinfo.WithCluster(ctx, clusterinfo.Fed), r.Client, r.Client, obj, func(in *rolloutv1alpha1.RolloutRun) error {
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

func (r *RolloutRunReconciler) findTrafficTopology(ctx context.Context, obj *rolloutv1alpha1.RolloutRun) ([]rolloutv1alpha1.TrafficTopology, error) {
	topologies := make([]rolloutv1alpha1.TrafficTopology, 0)
	for _, name := range obj.Spec.TrafficTopologyRefs {
		key := client.ObjectKey{Namespace: obj.Namespace, Name: name}
		var topology rolloutv1alpha1.TrafficTopology
		err := r.Client.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), key, &topology)
		if err != nil {
			return nil, err
		}
		topologies = append(topologies, topology)
	}

	return topologies, nil
}

func (r *RolloutRunReconciler) syncRolloutRun(
	ctx context.Context,
	obj *rolloutv1alpha1.RolloutRun,
	newStatus *rolloutv1alpha1.RolloutRunStatus,
	accesor workload.Accessor,
	workloads *workload.Set,
) (ctrl.Result, error) {
	key := utils.ObjectKeyString(obj)
	logger := r.Logger.WithValues("rolloutRun", key)

	ownerKind, ownerName := r.findOwnerKindName(obj)

	topologies, err := r.findTrafficTopology(ctx, obj)
	if err != nil {
		logger.Error(err, "failed to find traffic topology")
		return ctrl.Result{}, err
	}

	for _, obj := range topologies {
		readyCond := condition.GetCondition(obj.Status.Conditions, "Ready")
		if readyCond == nil || readyCond.Status != metav1.ConditionTrue {
			logger.Info("still waiting for traffic topology ready, skip reconciling", "topology", obj.Name)
			return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	var (
		done   bool
		result ctrl.Result
	)

	trafficManager, err := trafficcontrol.NewManager(ctx, r.Client, topologies)
	if err != nil {
		return ctrl.Result{}, err
	}

	executorCtx := &executor.ExecutorContext{
		Context:        ctx,
		Client:         r.Client,
		Recorder:       r.Recorder,
		Accessor:       accesor,
		OwnerKind:      ownerKind,
		OwnerName:      ownerName,
		RolloutRun:     obj,
		NewStatus:      newStatus,
		Workloads:      workloads,
		TrafficManager: trafficManager,
	}
	if done, result, err = r.executor.Do(executorCtx); err != nil {
		return ctrl.Result{}, err
	}
	if done {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionFalse,
			rolloutv1alpha1.RolloutReasonProgressingCompleted,
			"rolloutRun is completed",
		)
		if newStatus.Phase == rolloutv1alpha1.RolloutRunPhaseCanceled {
			newCondition.Reason = rolloutv1alpha1.RolloutReasonProgressingCanceled
			newCondition.Message = "rolloutRun is canceled"
		}
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	} else if newStatus.Error != nil {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionFalse,
			rolloutv1alpha1.RolloutReasonProgressingError,
			"rolloutRun stop rolling since error exist",
		)
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	} else {
		newCondition := condition.NewCondition(
			rolloutv1alpha1.RolloutConditionProgressing,
			metav1.ConditionTrue,
			rolloutv1alpha1.RolloutReasonProgressingRunning,
			"rolloutRun is running",
		)
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *newCondition)
	}
	return result, nil
}

func (r *RolloutRunReconciler) findOwnerKindName(rolloutRun *rolloutv1alpha1.RolloutRun) (string, string) {
	owner := metav1.GetControllerOf(rolloutRun)
	if owner != nil {
		return owner.Kind, owner.Name
	}
	return "", ""
}

func (r *RolloutRunReconciler) findWorkloadsCrossCluster(ctx context.Context, obj *rolloutv1alpha1.RolloutRun) (workload.Accessor, *workload.Set, *workload.Set, error) {
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
		return nil, nil, nil, err
	}

	list, canaryList, err := workload.List(ctx, r.Client, accessor, obj.Namespace, match)
	if err != nil {
		return nil, nil, nil, err
	}
	return accessor, workload.NewSet(list...), workload.NewSet(canaryList...), nil
}

func (r *RolloutRunReconciler) syncWorkloadStatus(newStatus *rolloutv1alpha1.RolloutRunStatus, workloads, canaryWorkloads *workload.Set) {
	// generate workload status
	allWorkloads := workloads.ToSlice()
	sort.Slice(allWorkloads, func(i, j int) bool {
		iInfo := allWorkloads[i]
		jInfo := allWorkloads[j]

		if iInfo.ClusterName == jInfo.ClusterName {
			return iInfo.Name < jInfo.Name
		}

		return iInfo.ClusterName < jInfo.ClusterName
	})
	workloadStatuses := make([]rolloutv1alpha1.RolloutWorkloadStatus, len(allWorkloads))
	for i := range allWorkloads {
		info := allWorkloads[i]
		workloadStatuses[i] = info.APIStatus()
	}
	newStatus.TargetStatuses = workloadStatuses

	// generate canary workload status
	allCanaryWorkloads := canaryWorkloads.ToSlice()
	if len(allCanaryWorkloads) > 0 {
		sort.Slice(allCanaryWorkloads, func(i, j int) bool {
			iInfo := allCanaryWorkloads[i]
			jInfo := allCanaryWorkloads[j]

			if iInfo.ClusterName == jInfo.ClusterName {
				return iInfo.Name < jInfo.Name
			}

			return iInfo.ClusterName < jInfo.ClusterName
		})
		canaryWorkloadStatuses := make([]rolloutv1alpha1.RolloutWorkloadStatus, len(allCanaryWorkloads))
		for i := range allCanaryWorkloads {
			info := allCanaryWorkloads[i]
			canaryWorkloadStatuses[i] = info.APIStatus()
		}

		if newStatus.CanaryStatus == nil {
			newStatus.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{}
		}
		newStatus.CanaryStatus.Targets = canaryWorkloadStatuses
	}
}

func (r *RolloutRunReconciler) updateStatusOnly(ctx context.Context, obj *rolloutv1alpha1.RolloutRun, newStatus *rolloutv1alpha1.RolloutRunStatus, workloads, canaryWorkloads *workload.Set) error {
	// generate workload status
	r.syncWorkloadStatus(newStatus, workloads, canaryWorkloads)

	if equality.Semantic.DeepEqual(obj.Status, *newStatus) {
		// no change
		return nil
	}
	key := utils.ObjectKeyString(obj)
	now := metav1.Now()
	newStatus.LastUpdateTime = &now
	_, err := kubeutilclient.UpdateOnConflict(clusterinfo.WithCluster(ctx, clusterinfo.Fed), r.Client, r.Client.Status(), obj, func(in *rolloutv1alpha1.RolloutRun) error {
		in.Status = *newStatus
		in.Status.ObservedGeneration = in.Generation
		return nil
	})
	if err != nil {
		r.Recorder.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateStatus", "failed to update rolloutRun %q status: %v", key, err)
		r.Logger.Error(err, "failed to update rolloutRun status", "rolloutRun", key)
		return err
	}

	r.rvExpectation.ExpectUpdate(key, obj.ResourceVersion) // nolint
	return nil
}
