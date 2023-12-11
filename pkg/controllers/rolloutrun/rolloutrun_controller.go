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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/apis/rollout/v1alpha1/condition"
	workflowapi "kusionstack.io/rollout/apis/workflow"
	workflowv1alpha1 "kusionstack.io/rollout/apis/workflow/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/utils/eventhandler"
	"kusionstack.io/rollout/pkg/utils/expectations"
	workflowutil "kusionstack.io/rollout/pkg/workflow"
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

	expectation   expectations.ControllerExpectationsInterface
	rvExpectation expectations.ResourceVersionExpectationInterface
}

func NewReconciler(mgr manager.Manager, workloadRegistry workloadregistry.Registry) *RolloutRunReconciler {
	return &RolloutRunReconciler{
		ReconcilerMixin:  mixin.NewReconcilerMixin(ControllerName, mgr),
		workloadRegistry: workloadRegistry,
		expectation:      expectations.NewControllerExpectations(),
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
		For(&rolloutv1alpha1.RolloutRun{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Watches(
			multicluster.FedKind(&source.Kind{Type: &workflowv1alpha1.Workflow{}}),
			eventhandler.EqueueRequestForOwnerWithCreationObserved(&rolloutv1alpha1.RolloutRun{}, true, r.expectation),
		)

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
			r.expectation.DeleteExpectations(key)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if !r.satisfiedExpectations(obj) {
		return reconcile.Result{}, nil
	}

	// caculate new status
	newStatus := r.caculateStatus(obj)

	workloads, err := r.findWorkloadsCrossCluster(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.handleProgressing(ctx, obj, newStatus, workloads)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.updateStatusOnly(ctx, obj, newStatus, workloads)
	if err != nil {
		logger.Error(err, "failed to update status")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *RolloutRunReconciler) satisfiedExpectations(instance *rolloutv1alpha1.RolloutRun) bool {
	key := utils.ObjectKeyString(instance)
	logger := r.Logger.WithValues("rolloutRun", key)

	if !r.rvExpectation.SatisfiedExpectations(key, instance.ResourceVersion) {
		logger.Info("rolloutRun does not statisfy resourceVersion expectation, skip reconciling")
		return false
	}

	if !r.expectation.SatisfiedExpectations(key) {
		logger.Info("rolloutRun does not statisfy controller expectation, skip reconciling")
		return false
	}
	return true
}

func (r *RolloutRunReconciler) caculateStatus(obj *rolloutv1alpha1.RolloutRun) *rolloutv1alpha1.RolloutRunStatus {
	newStatus := obj.Status.DeepCopy()
	newStatus.ObservedGeneration = obj.Generation

	// TODO: calculate rollout run template hash

	if obj.DeletionTimestamp != nil {
		if newStatus.Phase != rolloutv1alpha1.RolloutPhaseTerminating {
			newStatus.Phase = rolloutv1alpha1.RolloutPhaseTerminating
			termnatingCond := condition.NewCondition(rolloutv1alpha1.RolloutConditionTerminating, metav1.ConditionTrue, "Terminating", "rolloutRun is deleted")
			newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *termnatingCond)
		}
		return newStatus
	}

	return newStatus
}

func (r *RolloutRunReconciler) handleProgressing(ctx context.Context, obj *rolloutv1alpha1.RolloutRun, newStatus *rolloutv1alpha1.RolloutRunStatus, workloads *workload.Set) error {
	key := utils.ObjectKeyString(obj)
	logger := r.Logger.WithValues("rolloutRun", key)

	currentWorkflow, err := r.getWorkflowForRun(ctx, obj)
	if client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed to get current workflow of rolloutRun")
		return err
	}

	// TODO: how to deal with rollout spec upgrading, continue the progressing workflow and take effect next time?
	if currentWorkflow != nil {
		// currentWorkflow is progressing
		return r.syncWorkflow(obj, currentWorkflow, newStatus)
	}

	logger.Info("about to construct workflow", "workflow", obj.Name)

	workflow, err := constructRunWorkflow(obj, workloads)
	if err != nil {
		r.Recorder.Eventf(obj, corev1.EventTypeWarning, "FailedCreate", "failed to construct a new workflow %s: %v", obj.Name, err)
		logger.Error(err, "failed to construct workflow", "workflow", obj.Name)
		return err
	}

	// expect a workflow creation observed in cache
	// NOTO: we have to set expectation before we create the workflow to avoid
	//       that the creation event comes so fast that we don't have time to set it
	r.expectation.ExpectCreations(key, 1) // nolint

	if err = r.Client.Create(clusterinfo.ContextFed, workflow); err != nil {
		r.expectation.DeleteExpectations(key)
		r.Recorder.Eventf(obj, corev1.EventTypeWarning, "FailedCreate", "failed to create a new workflow %s: %v", workflow.Name, err)
		logger.Error(err, "failed to create workflow", "workflow", workflow.Name)
		return err
	}

	r.Recorder.Eventf(obj, corev1.EventTypeNormal, "WorkflowCreated", "create a new workflow %s", workflow.Name)
	logger.Info("a new workflow has been created", "workflow", workflow.Name)

	// update status
	cond := condition.NewCondition(rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionTrue, "WorkflowCreated", "a new workflow is created")
	resetRolloutRunStatus(newStatus, rolloutv1alpha1.RolloutPhaseProgressing, cond)

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

func (r *RolloutRunReconciler) getWorkflowForRun(_ context.Context, obj *rolloutv1alpha1.RolloutRun) (*workflowv1alpha1.Workflow, error) {
	workflow := &workflowv1alpha1.Workflow{}
	err := r.Client.Get(clusterinfo.ContextFed, types.NamespacedName{
		Name:      obj.Name,
		Namespace: obj.Namespace,
	}, workflow)
	if err != nil {
		return nil, err
	}
	return workflow, nil
}

func (r *RolloutRunReconciler) syncWorkflow(obj *rolloutv1alpha1.RolloutRun, workflow *workflowv1alpha1.Workflow, newStatus *rolloutv1alpha1.RolloutRunStatus) error {
	if workflow == nil {
		cond := condition.NewCondition(rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionFalse, rolloutv1alpha1.RolloutReasonProgressingUnTriggered, "rolloutRun is not triggered")
		resetRolloutRunStatus(newStatus, rolloutv1alpha1.RolloutPhaseInitialized, cond)
		return nil
	}

	newStatus.Phase = rolloutv1alpha1.RolloutPhaseProgressing
	var cond *rolloutv1alpha1.Condition
	if workflow.Status.IsSucceeded() {
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCompleted
		cond = condition.NewCondition(rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionFalse, rolloutv1alpha1.RolloutReasonProgressingCompleted, "workflow is complated")
	} else if workflow.Status.IsCanceled() {
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCompleted
		cond = condition.NewCondition(rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionFalse, rolloutv1alpha1.RolloutReasonProgressingCanceled, "workflow is canceled")
	} else if workflow.Status.IsFailed() {
		cond = condition.NewCondition(rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionTrue, rolloutv1alpha1.RolloutReasonProgressingError, "workflow is failed")
	} else {
		cond = condition.NewCondition(rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionTrue, rolloutv1alpha1.RolloutReasonProgressingRunning, "workflow is running")
	}
	if cond != nil {
		newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *cond)
	}

	batchStatusRecords, err := workflowutil.GetBatchStatusRecords(workflow)
	if err != nil {
		return err
	}
	if len(batchStatusRecords) > 0 {
		batchStatus := workflowutil.GetCurrentBatch(batchStatusRecords)
		newStatus.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
			RolloutBatchStatus: batchStatus,
			Records:            batchStatusRecords,
		}
	}

	// handle manual command
	return r.handleManualCommand(obj, workflow, newStatus)
}

func (r *RolloutRunReconciler) handleManualCommand(instance *rolloutv1alpha1.RolloutRun, workflow *workflowv1alpha1.Workflow, newStatus *rolloutv1alpha1.RolloutRunStatus) error {
	if len(instance.Annotations) == 0 {
		return nil
	}

	command, ok := instance.Annotations[rollout.AnnoManualCommandKey]
	if !ok {
		return nil
	}

	var workflowModifyFunc func(*workflowv1alpha1.Workflow)

	switch command {
	case rollout.AnnoManualCommandResume:
		if workflow.Status.IsPaused() && newStatus.BatchStatus.CurrentBatchState == rolloutv1alpha1.BatchStepStatePaused {
			taskName := getSuspendTaskNameByBatchIndex(workflow, newStatus.BatchStatus.CurrentBatchIndex)
			if len(taskName) > 0 {
				resumeContext := []string{taskName}
				data, _ := json.Marshal(resumeContext)
				workflowModifyFunc = func(w *workflowv1alpha1.Workflow) {
					if w.Annotations == nil {
						w.Annotations = map[string]string{}
					}
					w.Annotations[workflowapi.AnnoWorkflowResumeSuspendTasks] = string(data)
				}
			}
		}
	case rollout.AnnoManualCommandPause:
		if !workflow.Status.IsPaused() && workflow.Spec.Status != workflowv1alpha1.WorkflowSpecStatusPaused {
			workflowModifyFunc = func(w *workflowv1alpha1.Workflow) {
				w.Spec.Status = workflowv1alpha1.WorkflowSpecStatusPaused
			}
		}
	case rollout.AnnoManualCommandCancel:
		if !workflow.Status.IsCanceled() && workflow.Spec.Status != workflowv1alpha1.WorkflowSpecStatusCanceled {
			workflowModifyFunc = func(w *workflowv1alpha1.Workflow) {
				w.Spec.Status = workflowv1alpha1.WorkflowSpecStatusCanceled
			}
		}
	}

	// update workflow if necessary
	if workflowModifyFunc != nil {
		op, err := utils.UpdateOnConflict(clusterinfo.ContextFed, r.Client, r.Client, workflow, func() error {
			workflowModifyFunc(workflow)
			return nil
		})
		if err != nil {
			return err
		}
		r.Logger.Info("handle manual command, update workflow", "workflow", utils.ObjectKeyString(workflow), "rolloutRun", utils.ObjectKeyString(instance), "result", op)
	}

	// delete mamual command from rollout
	op, err := utils.UpdateOnConflict(clusterinfo.ContextFed, r.Client, r.Client, instance, func() error {
		delete(instance.Annotations, rollout.AnnoManualCommandKey)
		return nil
	})

	if err != nil {
		return err
	}

	key := utils.ObjectKeyString(instance)
	r.Logger.Info("handle manual command, delete command from rolloutRun annotation", "rolloutRun", key, "result", op)
	r.rvExpectation.ExpectUpdate(key, instance.ResourceVersion) // nolint

	return nil
}

func getSuspendTaskNameByBatchIndex(workflow *workflowv1alpha1.Workflow, batchIndex int32) string {
	for _, t := range workflow.Spec.Tasks {
		if t.TaskSpec.Suspend == nil {
			continue
		}
		indexStr := t.Labels[workflowapi.LabelBatchIndex]
		if len(indexStr) == 0 {
			continue
		}
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			continue
		}

		if index == int(batchIndex) {
			return t.Name
		}
	}
	return ""
}
