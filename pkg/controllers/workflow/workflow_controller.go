/*
 * Copyright 2023 The KusionStack Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"code.alipay.com/paas-core/kydra/pkg/clusterinfo"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/KusionStack/rollout/api"
	"github.com/KusionStack/rollout/api/v1alpha1"
	"github.com/KusionStack/rollout/pkg/dag"
	"github.com/KusionStack/rollout/pkg/event"
	resolvetask "github.com/KusionStack/rollout/pkg/task"
	"github.com/KusionStack/rollout/pkg/workflow"
)

// WorkflowReconciler reconciles a Workflow object
type WorkflowReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=workflows/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=workflows/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Workflow object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *WorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	logger := log.FromContext(ctx)

	logger.Info("Reconciling Workflow")
	var flow v1alpha1.Workflow
	if err := r.Get(ctx, req.NamespacedName, &flow); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch Workflow")
		return ctrl.Result{}, err
	}

	// deal with terminating flow
	if flow.DeletionTimestamp != nil {
		// todo: update status
		return ctrl.Result{}, nil
	}

	originStatus := flow.Status.DeepCopy()
	beforeCondition := flow.Status.GetCondition(v1alpha1.ConditionSucceeded)

	// Initialize status if necessary
	if !flow.HasStarted() {
		resolvedTasks, err := r.resolveTasks(ctx, &flow)
		if err != nil {
			logger.Error(err, "failed to resolve tasks")
			return ctrl.Result{}, err
		}
		taskStatuses := make([]v1alpha1.WorkflowTaskStatus, len(resolvedTasks))
		for i, resolvedTask := range resolvedTasks {
			taskStatuses[i] = v1alpha1.WorkflowTaskStatus{
				Name:             resolvedTask.WorkflowTask.Name,
				SuccessCondition: nil,
			}
			if resolvedTask.Task != nil {
				taskStatuses[i].SuccessCondition = resolvedTask.Task.Status.GetCondition(v1alpha1.ConditionSucceeded)
			}
		}
		flow.Status.Initialize(taskStatuses)
		r.Recorder.Eventf(&flow, corev1.EventTypeNormal, "Initialized", "Initialized workflow %s", flow.Name)

		// we already send event for initialized, so we update `beforeCondition` with current condition
		beforeCondition = flow.Status.GetCondition(v1alpha1.ConditionSucceeded)
	}

	err := r.doReconcile(ctx, &flow)
	if err != nil {
		logger.Error(err, "failed to reconcile")
		event.EmitError(r.Recorder, err, &flow)
	}

	afterCondition := flow.Status.GetCondition(v1alpha1.ConditionSucceeded)
	event.Emit(r.Recorder, beforeCondition, afterCondition, &flow)

	// update status if necessary
	if !equality.Semantic.DeepEqual(originStatus, flow.Status) {
		if err := r.UpdateStatus(ctx, &flow); err != nil {
			logger.Error(err, "failed to update status")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// UpdateStatus updates Workflow status, which retries on conflict.
func (r *WorkflowReconciler) UpdateStatus(ctx context.Context, flow *v1alpha1.Workflow) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var existing v1alpha1.Workflow
		if err := r.Get(ctx, client.ObjectKeyFromObject(flow), &existing); err != nil {
			return client.IgnoreNotFound(err)
		}
		// If there is no change, do not update
		if equality.Semantic.DeepEqual(existing.Status, flow.Status) {
			return nil
		}
		flow.Status.LastUpdatedAt = &metav1.Time{Time: time.Now()}
		flow.Status.ObservedGeneration = flow.Generation
		existing.Status = flow.Status
		return r.Status().Update(ctx, &existing)
	})
}

func (r *WorkflowReconciler) doReconcile(ctx context.Context, flow *v1alpha1.Workflow) error {
	logger := log.FromContext(ctx)

	if flow.IsDone() {
		return nil
	}

	// workflow can only be pending before it starts, we do nothing when it is pending
	if flow.IsPending() {
		flow.Status.Pending("workflow is pending")
		return nil
	}

	if flow.Status.IsCancelling() || flow.Status.IsPausing() {
		// update status and return
		resolvedTasks, err := r.resolveTasks(ctx, flow)
		if err != nil {
			flow.Status.Running(v1alpha1.ConditionReasonError.String(), err.Error())
			logger.Error(err, "failed to resolve tasks")
			return err
		}

		wfCtx := &workflow.WorkflowContext{
			SpecStatus: flow.Spec.Status,
			Workflow:   flow,
			Tasks:      resolvedTasks,
		}
		wfCtx.CalculateStatus()
		return nil
	}

	if flow.IsCancelled() {
		if err := r.cancelWorkflow(ctx, flow); err != nil {
			logger.Error(err, "failed to cancel workflow")
			flow.Status.Running(v1alpha1.ConditionReasonError.String(), err.Error())
			return err
		}
		return nil
	}

	// TODO: use webhook to ensure that the workflow can only be paused when it is running
	if flow.IsPaused() {
		flow.Status.Pause("workflow is paused")
		// TODO(common_release/rollout#14): pause tasks
		return nil
	}

	// build dag
	d, err := dag.Build(v1alpha1.WorkflowTaskList(flow.Spec.Tasks), v1alpha1.WorkflowTaskList(flow.Spec.Tasks).Deps())
	if err != nil {
		flow.Status.Fail(v1alpha1.ConditionReasonError.String(), err.Error())
		logger.Error(err, "failed to build dag")
		return err
	}

	// run next tasks
	resolvedTasks, err := r.resolveTasks(ctx, flow)
	if err != nil {
		flow.Status.Running(v1alpha1.ConditionReasonError.String(), err.Error())
		logger.Error(err, "failed to resolve tasks")
		return err
	}

	wfCtx := &workflow.WorkflowContext{
		DAG:        d,
		SpecStatus: flow.Spec.Status,
		Workflow:   flow,
		Tasks:      resolvedTasks,
	}

	err = wfCtx.RunTasks(ctx, r.Client)
	if err != nil {
		flow.Status.Running(v1alpha1.ConditionReasonError.String(), err.Error())
		logger.Error(err, "failed to run tasks")
		return err
	}

	// calculate status
	wfCtx.CalculateStatus()

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&v1alpha1.Workflow{}).
		Owns(&v1alpha1.Task{}).
		Complete(r)
}

// resolveTasks resolve tasks
func (r *WorkflowReconciler) resolveTasks(ctx context.Context, flow *v1alpha1.Workflow) ([]resolvetask.ResolvedTask, error) {
	resolvedTasks := make([]resolvetask.ResolvedTask, len(flow.Spec.Tasks))

	taskMap, err := r.getTaskMap(ctx, flow)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get task map")
	}

	// TODO(hengzhuo): consider to get task by name in workflow status
	for i, workflowTask := range flow.Spec.Tasks {
		generateName := fmt.Sprintf("%s-", workflowTask.Name)
		resolvedTasks[i] = resolvetask.ResolvedTask{
			Namespace:    flow.Namespace,
			GenerateName: generateName,
			WorkflowTask: &(flow.Spec.Tasks[i]),
			Task:         taskMap[generateName],
			Type:         workflowTask.TaskSpec.GetType(),
		}
	}
	return resolvedTasks, nil
}

// getTaskMap get tasks of workflow and generate a map of task generatedName -> task
func (r *WorkflowReconciler) getTaskMap(ctx context.Context, flow *v1alpha1.Workflow) (map[string]*v1alpha1.Task, error) {
	var taskList v1alpha1.TaskList
	if err := r.List(ctx, &taskList, client.InNamespace(flow.Namespace), client.MatchingLabels{api.LabelGeneratedBy: flow.Name}); err != nil {
		return nil, err
	}
	// filter tasks by ownerReference
	var tasks []*v1alpha1.Task
	for i := range taskList.Items {
		task := taskList.Items[i]
		if metav1.IsControlledBy(&task, flow) {
			tasks = append(tasks, &task)
		}
	}
	// generate map
	taskMap := make(map[string]*v1alpha1.Task, len(tasks))
	for i := range tasks {
		task := tasks[i]
		taskMap[task.GenerateName] = task
	}
	return taskMap, nil
}

func (r *WorkflowReconciler) cancelWorkflow(ctx context.Context, flow *v1alpha1.Workflow) error {
	logger := log.FromContext(ctx)
	if flow.Status.IsCancelling() {
		logger.Info("workflow is already in Cancelling status")
		return nil
	}

	resolvedTasks, err := r.resolveTasks(ctx, flow)
	if err != nil {
		return err
	}

	var taskErr error
	var wg sync.WaitGroup
	var taskUpdated bool
	for _, rt := range resolvedTasks {
		if rt.Task == nil || rt.IsCompleted() {
			continue
		}

		taskUpdated = true
		wg.Add(1)
		go func(task *v1alpha1.Task) {
			defer wg.Done()
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := r.Client.Get(ctx, types.NamespacedName{Namespace: task.Namespace, Name: task.Name}, task); err != nil {
					return err
				}
				task.Spec.Status = v1alpha1.TaskSpecStatusCancelled
				return r.Client.Update(ctx, task)
			}); err != nil {
				logger.Error(err, "failed to cancel task", "task", task.Name)
				taskErr = multierror.Append(taskErr, err)
			}
		}(rt.Task)
	}

	wg.Wait()

	if taskErr != nil {
		return taskErr
	}

	if taskUpdated {
		flow.Status.Cancelling("waiting for tasks to be cancelled")
	} else {
		flow.Status.Cancel("workflow cancelled")
	}
	return nil
}
