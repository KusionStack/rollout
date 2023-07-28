/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rollout

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"code.alipay.com/paas-core/kydra/pkg/clusterinfo"
	"code.alipay.com/paas-core/kydra/pkg/multicluster"

	"github.com/KusionStack/rollout/api"
	rolloutv1alpha1 "github.com/KusionStack/rollout/api/v1alpha1"
	"github.com/KusionStack/rollout/pkg/utils"
	"github.com/KusionStack/rollout/pkg/utils/expectations"
	workflowutil "github.com/KusionStack/rollout/pkg/workflow"
	"github.com/KusionStack/rollout/pkg/workload"
)

const (
	controllerName = "rollout-controller"
)

var (
	expectation          = expectations.NewResourceVersionExpectation()
	workflowExpectations *expectations.ActiveExpectations
)

// RolloutReconciler reconciles a Rollout object
type RolloutReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// SetupWithManager sets up the controller with the Manager.
func (r *RolloutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	workflowExpectations = expectations.NewActiveExpectations(mgr.GetClient())
	r.Recorder = mgr.GetEventRecorderFor(controllerName)
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: int(5 * utils.MaxConcurrencyReconcileTimes)}).
		For(&rolloutv1alpha1.Rollout{}, builder.WithPredicates(NamespacePredicate(), RolloutPredicate())).
		Watches(multicluster.FedKind(&source.Kind{Type: &rolloutv1alpha1.Workflow{}}), &handler.EnqueueRequestForObject{}, builder.WithPredicates(WorkflowPredicate())).
		Watches(multicluster.FedKind(&source.Kind{Type: &rolloutv1alpha1.RolloutStrategy{}}), &handler.EnqueueRequestForObject{}).
		// TODO: add watched workload here
		Complete(r)
}

//+kubebuilder:rbac:groups=rollout.kafe.kusionstack.io,resources=rollouts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rollout.kafe.kusionstack.io,resources=rollouts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rollout.kafe.kusionstack.io,resources=rollouts/finalizers,verbs=update

//+kubebuilder:rbac:groups=rollout.kafe.kusionstack.io,resources=rolloutstrategies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RolloutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	key := fmt.Sprintf("%s/%s", req.Namespace, req.Name)
	logger := log.FromContext(ctx)
	logger.Info("start to reconcile...")
	defer logger.Info("end reconciling")

	instance := &rolloutv1alpha1.Rollout{}
	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			cleanExpectations(key)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// check if it is necessary to clean up in deletion
	deleted, err := r.cleanUpForDeletionIfNecessary(instance, ctx)
	if err != nil {
		if errors.IsConflict(err) {
			logger.Info("conflicted when clean up for deletion...")
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}
	if deleted {
		logger.Info("deleted...")
		return reconcile.Result{}, nil
	}

	// check expectation
	if !expectation.SatisfiedExpectations(key, instance.ResourceVersion) {
		r.Recorder.Eventf(instance, corev1.EventTypeNormal, "NotSatisfied", "expectation not satisfied for Rollout: %s", key)
		return reconcile.Result{}, nil
	}

	// ensure finalizer
	if !utils.ContainsFinalizer(instance, api.FinalizerRolloutProtection) {
		utils.AddFinalizer(&instance.ObjectMeta, api.FinalizerRolloutProtection)
		err := r.Update(ctx, instance)
		if err != nil {
			if errors.IsConflict(err) {
				logger.Info("conflicted when add finalizer...")
				return reconcile.Result{Requeue: true}, nil
			}
			logger.Error(err, "failed to append finalizer")
			return reconcile.Result{}, err
		}
	}

	// do upgrade
	err = r.manageUpgrade(ctx, instance)
	if err != nil {
		if errors.IsConflict(err) {
			logger.Error(err, "conflicted when operation manageUpgrade")
			return reconcile.Result{Requeue: true}, nil
		}
		logger.Error(err, "failed when operation manageUpgrade")
		return reconcile.Result{}, err
	}

	// TODO: update status even if error occurs while r.manageUpgrade
	if err := r.updateStatus(ctx, instance); err != nil {
		logger.Error(err, "failed to updateStatus")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *RolloutReconciler) cleanUpForDeletionIfNecessary(instance *rolloutv1alpha1.Rollout, ctx context.Context) (bool, error) {
	if instance.DeletionTimestamp != nil {
		cleanExpectations(fmt.Sprintf("%s/%s", instance.Namespace, instance.Name))

		active, _, err := r.getActiveWorkflow(instance)
		if err != nil {
			return true, err
		}
		if active != nil {
			// TODO(hengzhuo): for now workflow phase is empty, add phase to workflow later or use other way to check workflow status
			r.Recorder.Eventf(instance, corev1.EventTypeNormal, "WaitFlowEnd", "cannot delete rollout %s for workflow %s is in: %s", instance.Name, active.Name, active.Status.Phase)
			// waiting next events
			return true, nil
		}

		// remove finalizer
		has := utils.RemoveFinalizer(&instance.ObjectMeta, api.FinalizerRolloutProtection)
		if has {
			if err := r.Update(ctx, instance); err != nil {
				r.Recorder.Eventf(instance, corev1.EventTypeWarning, "RemoveFinalizerFailed", "failed to remove finalizer for %s: %s", instance.Name, err)
				return true, err
			}
		}

		return true, nil
	}

	// no need to clean
	return false, nil
}

func (r *RolloutReconciler) manageUpgrade(ctx context.Context, instance *rolloutv1alpha1.Rollout) error {
	logger := log.FromContext(ctx)
	activeWorkflow, currentWorkflow, err := r.getActiveWorkflow(instance)
	if err != nil {
		return err
	}
	if activeWorkflow != nil {
		// has processing workflow
		return r.syncWorkflow(instance, activeWorkflow, false)
	}

	// clean annos
	if err = r.cleanAnnotations(instance); err != nil {
		return err
	}

	// check trigger satisfied
	var needTrigger bool
	if instance.Spec.TriggerCondition.ManualMode != nil {
		needTrigger, err = r.checkTriggerManualMode(instance)
		if err != nil {
			return err
		}
		if !needTrigger {
			return nil
		}
	}

	workloadWrappers, err := r.getLocalWorkloads(instance)
	if err != nil {
		return err
	}
	if len(workloadWrappers) == 0 {
		return nil
	}

	// check auto mode
	if !needTrigger && instance.Spec.TriggerCondition.AutoMode != nil {
		var msg string
		needTrigger, msg, err = r.checkTriggerAutoMode(instance, workloadWrappers)
		if err != nil {
			return err
		}
		logger.Info(msg, "needTrigger", needTrigger)
		r.Recorder.Eventf(instance, corev1.EventTypeNormal, "CheckTriggerAutoMode", "trigger: %v, msg: %s", needTrigger, msg)
	}

	if !needTrigger {
		// sync status with current workflow if it's final status
		return r.syncWorkflow(instance, currentWorkflow, false)
	}

	// need trigger
	strategy, err := r.parseUpgradeStrategy(instance)
	if err != nil {
		return err
	}

	rolloutID := utils.RandStr()
	instance.Status.ObservedRolloutID = rolloutID
	workflow, err := constructWorkflow(instance, strategy, workloadWrappers, rolloutID)
	if err != nil {
		return err
	}

	// update rolloutID to all workloads
	_, err = utils.SlowStartBatch(len(workloadWrappers), 1, true, func(i int, _ error) error {
		return r.updateWorkloadRolloutID(ctx, workloadWrappers[i], rolloutID)
	})
	if err != nil {
		return err
	}

	if err = workflowExpectations.Expect(instance, expectations.Workflow, workflow.Name, expectations.Create); err != nil {
		panic(fmt.Sprintf("fail to expect creation of workflow %s: %s", workflow.Name, err))
	}
	if err = r.Create(clusterinfo.ContextFed, workflow); err != nil {
		workflowExpectations.DeleteItem(instance, expectations.Workflow, workflow.Name)
		return err
	}
	instance.Status.WorkflowInfo.Name = workflow.Name
	return nil
}

func (r *RolloutReconciler) checkTriggerManualMode(instance *rolloutv1alpha1.Rollout) (trigger bool, err error) {
	// only trigger when rolloutId != ObservedRolloutID
	if instance.Spec.TriggerCondition.ManualMode.RolloutId == "" {
		return false, fmt.Errorf("empty RolloutId in ManualMode")
	}
	if instance.Spec.TriggerCondition.ManualMode.RolloutId != instance.Status.ObservedRolloutID {
		// update status
		instance.Status.ObservedRolloutID = instance.Spec.TriggerCondition.ManualMode.RolloutId
		return true, nil
	}
	return
}

func (r *RolloutReconciler) checkTriggerAutoMode(instance *rolloutv1alpha1.Rollout, workloadWrappers []workload.Interface) (trigger bool, msg string, err error) {
	if workloadWrappers == nil || len(workloadWrappers) < 1 {
		return
	}

	var msgs []string
	//var selector labels.Selector
	changedCount := 0

	if instance.Spec.TriggerCondition.AutoMode.BarrierMatcher != nil {
		// use label selector
		//selector, err = metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{*instance.Spec.TriggerCondition.AutoMode.BarrierMatcher}})
		if err != nil {
			return false, "", err
		}
	}

	currentRolloutID := instance.Status.ObservedRolloutID
	msgs = append(msgs, fmt.Sprintf("currentRolloutID: %s", currentRolloutID))
	for _ = range workloadWrappers {
		// TODO: check if all workloads are prepared to be upgraded
	}

	return changedCount == len(workloadWrappers), strings.Join(msgs, "; "), nil
}

func (r *RolloutReconciler) parseUpgradeStrategy(instance *rolloutv1alpha1.Rollout) (*rolloutv1alpha1.BatchStrategy, error) {
	ctx := clusterinfo.WithCluster(context.TODO(), clusterinfo.Fed)
	strategyRef := instance.Spec.StrategyRef
	if strategyRef == "" {
		return nil, fmt.Errorf("empty strategyRef")
	}

	strategy := &rolloutv1alpha1.RolloutStrategy{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      strategyRef,
		Namespace: instance.GetNamespace(),
	}, strategy)
	if err != nil {
		return nil, err
	}

	if strategy.Spec.Batch == nil || strategy.Spec.Batch.BatchTemplate == nil {
		return nil, fmt.Errorf("invalid BatchStrategy")
	}

	return strategy.Spec.Batch, nil
}

func (r *RolloutReconciler) getLocalWorkloads(instance *rolloutv1alpha1.Rollout) ([]workload.Interface, error) {
	switch instance.Spec.App.WorkloadRef.GroupVersionKind() {
	// TODO: add more supported workload types
	default:
		return nil, fmt.Errorf("unsupported workload gvk: %s", instance.Spec.App.WorkloadRef.GroupVersionKind())
	}
}

func (r *RolloutReconciler) updateStatus(ctx context.Context, instance *rolloutv1alpha1.Rollout) error {
	logger := log.FromContext(ctx)
	logger.Info("start to update rollout status")

	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	oldInstance := &rolloutv1alpha1.Rollout{}
	if gotErr := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, oldInstance); gotErr != nil {
		return gotErr
	}

	if equality.Semantic.DeepEqual(oldInstance.Status, instance.Status) {
		return nil
	}

	instance.Status.LastUpdatedAt = &metav1.Time{Time: time.Now()}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if gotErr := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, oldInstance); gotErr != nil {
			return gotErr
		}
		oldInstance.Status = instance.Status

		key := fmt.Sprintf("%s/%s", instance.Namespace, instance.Name)
		expectation.ExpectUpdate(key, instance.ResourceVersion)
		err := r.Status().Update(ctx, oldInstance)
		if err != nil {
			expectation.DeleteExpectations(key)
		}

		return err
	}); err != nil {
		logger.Error(err, "fail to update rollout status")
		return err
	}

	return nil
}

func (r *RolloutReconciler) getCurrentWorkflow(instance *rolloutv1alpha1.Rollout) (*rolloutv1alpha1.Workflow, error) {
	if instance.Status.WorkflowInfo.Name == "" {
		return nil, nil
	}

	workflow := &rolloutv1alpha1.Workflow{}
	err := r.Get(clusterinfo.ContextFed, types.NamespacedName{
		Name:      instance.Status.WorkflowInfo.Name,
		Namespace: instance.GetNamespace(),
	}, workflow)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return workflow, nil
}

func (r *RolloutReconciler) getActiveWorkflow(instance *rolloutv1alpha1.Rollout) (activeWorkflow *rolloutv1alpha1.Workflow, currentWorkflow *rolloutv1alpha1.Workflow, err error) {
	currentWorkflow, err = r.getCurrentWorkflow(instance)
	if err != nil {
		return
	}

	if currentWorkflow != nil && !workflowutil.IsWorkflowFinalPhase(currentWorkflow) {
		activeWorkflow = currentWorkflow
		return
	}

	// label selector
	workflowList := &rolloutv1alpha1.WorkflowList{}
	err = r.List(clusterinfo.ContextFed, workflowList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			api.LabelRolloutControl:   "true",
			api.LabelRolloutReference: instance.Name,
		}),
		Namespace: instance.Namespace,
	})
	if err != nil {
		return nil, nil, err
	}
	if workflowList.Items == nil || len(workflowList.Items) < 1 {
		return nil, nil, nil
	}
	for i := range workflowList.Items {
		if !workflowutil.IsWorkflowFinalPhase(&workflowList.Items[i]) {
			activeWorkflow = &workflowList.Items[i]
			return
		}
	}
	return
}

func (r *RolloutReconciler) syncWorkflow(instance *rolloutv1alpha1.Rollout, workflow *rolloutv1alpha1.Workflow, dryRun bool) (err error) {
	if workflow == nil {
		return nil
	}

	// update status from workflow
	if err = workflowutil.CalcRollout(instance.Status.Stages, &instance.Status.WorkflowInfo, workflow, getUpdateStatusCbFn(instance, workflow)); err != nil {
		return
	}

	// update workflow if needed
	fn, err := r.getSyncRetryFn(instance, workflow, dryRun)
	if err != nil {
		return
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, fn)
}

func (r *RolloutReconciler) cleanAnnotations(instance *rolloutv1alpha1.Rollout) (err error) {
	// update workflow if needed
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// check need-trigger
		currentRollout := &rolloutv1alpha1.Rollout{}
		ctx := clusterinfo.ContextFed
		err = r.Get(ctx, types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      instance.Name,
		}, currentRollout)
		if err != nil {
			return err
		}

		needUpdate := false
		if currentRollout.Annotations != nil {
			if _, has := currentRollout.Annotations[api.AnnoRolloutResumeContext]; has {
				needUpdate = true
				delete(currentRollout.Annotations, api.AnnoRolloutResumeContext)
			}
		}
		if currentRollout.Labels != nil {
			if _, has := currentRollout.Labels[api.LabelRolloutManualCommand]; has {
				needUpdate = true
				delete(currentRollout.Labels, api.LabelRolloutManualCommand)
			}
		}

		if needUpdate {
			return r.Update(ctx, currentRollout)
		}
		return nil
	})
	return err
}

func (r *RolloutReconciler) updateWorkloadRolloutID(ctx context.Context, wrapper workload.Interface, rolloutID string) (err error) {
	logger := log.FromContext(ctx)
	workload := wrapper.GetObj()

	if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if workload.GetLabels() == nil {
			workload.SetLabels(map[string]string{})
		}
		if val, ok := workload.GetLabels()[api.LabelRolloutID]; !ok || rolloutID != val {
			workload.GetLabels()[api.LabelRolloutID] = rolloutID
			return r.Update(clusterinfo.WithCluster(context.Background(), wrapper.GetCluster()), workload)
		}
		return nil
	}); err != nil {
		logger.Error(err, "fail to update workload rolloutId")
		return err
	}
	return err
}

func (r *RolloutReconciler) getSyncRetryFn(instance *rolloutv1alpha1.Rollout, workflow *rolloutv1alpha1.Workflow, dryRun bool) (func() error, error) {
	// check need-trigger
	ctx := clusterinfo.ContextFed
	currentRollout := &rolloutv1alpha1.Rollout{}
	if dryRun {
		currentRollout = instance
	} else {
		err := r.Get(ctx, types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      instance.Name,
		}, currentRollout)
		if err != nil {
			return nil, err
		}
	}

	currentWorkflow := &rolloutv1alpha1.Workflow{}
	if dryRun {
		currentWorkflow = workflow
	} else {
		ctx := clusterinfo.ContextFed
		err := r.Get(ctx, types.NamespacedName{
			Name:      workflow.Name,
			Namespace: workflow.GetNamespace(),
		}, currentWorkflow)
		if err != nil {
			return nil, err
		}
	}

	return func() error {
		needUpdateRollout, needUpdateWorkflow := workflowutil.SyncFlow(currentRollout.Labels, currentRollout.Annotations, currentRollout.Status.Stages, workflow)
		if !dryRun {
			if needUpdateRollout {
				if err := r.Update(ctx, currentRollout); err != nil {
					return err
				}
			}
			if needUpdateWorkflow {
				if err := r.Update(ctx, currentWorkflow); err != nil {
					return err
				}
			}
		}
		return nil
	}, nil
}

func getUpdateStatusCbFn(instance *rolloutv1alpha1.Rollout, workflow *rolloutv1alpha1.Workflow) workflowutil.UpdateStatusCallBack {
	return func(stages []rolloutv1alpha1.Stage, latestProcessingStageIndex, latestBatchStageIndex int) {
		instance.Status.CurrentStageIndex = int32(latestProcessingStageIndex + 1)
		// update total workloadDetail
		if instance.Status.Stages[latestProcessingStageIndex].WorkloadDetails != nil {
			summary := &rolloutv1alpha1.RolloutSummary{}
			for _, detail := range instance.Status.Stages[latestProcessingStageIndex].WorkloadDetails {
				if detail != nil {
					summary.Replicas += detail.Replicas
					summary.UpdatedReplicas += detail.UpdatedReplicas
					summary.UpdatedReadyReplicas += detail.UpdatedReadyReplicas
					summary.UpdatedAvailableReplicas += detail.UpdatedAvailableReplicas
				}
			}
			instance.Status.Summary = summary
		}

		// calc rollout condition
		instance.Status.Conditions = workflow.Status.Conditions

		instance.Status.Stages = stages
	}
}

func cleanExpectations(key string) {
	expectation.DeleteExpectations(key)
	workflowExpectations.DeleteByKey(key)
}
