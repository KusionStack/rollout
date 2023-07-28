package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/KusionStack/rollout/api"
	"github.com/KusionStack/rollout/api/v1alpha1"
	"github.com/KusionStack/rollout/pkg/dag"
	resolvetask "github.com/KusionStack/rollout/pkg/task"
)

// WorkflowContext is the context for a workflow. It helps simplify the implementation for scheduling the next tasks.
type WorkflowContext struct {
	DAG *dag.DAG

	SpecStatus v1alpha1.WorkflowSpecStatus

	Workflow *v1alpha1.Workflow

	Tasks []resolvetask.ResolvedTask
}

// WorkflowTaskCount is the count of the workflow tasks
type WorkflowTaskCount struct {
	Skipped   int
	Cancelled int
	Succeeded int
	Failed    int
	Running   int
	Pending   int
	Paused    int
}

func (wc *WorkflowContext) DAGRunnableTasks() []resolvetask.ResolvedTask {
	var tasks []resolvetask.ResolvedTask

	// If the workflow is cancelled, we should not schedule any tasks.
	if wc.IsCancelled() {
		return tasks
	}

	candidates := wc.DAG.GetCandidateTasks(wc.completedTaskNames())
	for _, task := range wc.Tasks {
		if _, ok := candidates[task.WorkflowTask.Name]; ok && task.Task == nil {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func (wc *WorkflowContext) IsCancelled() bool {
	return wc.SpecStatus == v1alpha1.WorkflowSpecStatusCancelled
}

// completedTaskNames returns the names of the completed tasks
func (wc *WorkflowContext) completedTaskNames() []string {
	var completedTaskNames []string
	for _, rt := range wc.Tasks {
		if rt.Task != nil && rt.Task.Status.IsCompleted() {
			completedTaskNames = append(completedTaskNames, rt.WorkflowTask.Name)
		}
	}
	return completedTaskNames
}

func (wc *WorkflowContext) CalculateStatus() {
	wc.calculateCondition()

	wc.Workflow.Status.ObservedGeneration = wc.Workflow.Generation
	wc.Workflow.Status.Tasks = wc.calculateTaskStatus()
}

func (wc *WorkflowContext) calculateCondition() {
	// todo: consider timeout cases

	// completed tasks is the sum of succeeded, failed, cancelled
	// incomplete tasks is the sum of running, pending
	// the rest is skipped tasks
	count := wc.getWorkflowTaskCount()
	completedTasks := count.Succeeded + count.Failed + count.Cancelled + count.Skipped
	incompleteTasks := count.Running + count.Pending + count.Paused
	message := fmt.Sprintf("Tasks Completed: %d (Failed: %d, Cancelled: %d), Tasks Incomplete: %d (Running: %d, Pending: %d, Paused: %d), Skipped: %d",
		completedTasks, count.Failed, count.Cancelled, incompleteTasks, count.Running, count.Pending, count.Paused, count.Skipped)

	// cancelled status
	if wc.IsCancelled() {
		if count.Running+count.Paused > 0 {
			wc.Workflow.Status.Cancelling(message)
		} else {
			wc.Workflow.Status.Cancel(message)
		}
		return
	}

	// completed status
	if incompleteTasks == 0 {
		if count.Failed > 0 {
			wc.Workflow.Status.Fail(v1alpha1.ConditionReasonFailed.String(), message)
		} else {
			wc.Workflow.Status.Succeed(message)
		}
		return
	}

	// paused status
	if count.Paused > 0 && count.Running == 0 {
		wc.Workflow.Status.Pause(message)
		return
	}

	// pausing status

	wc.Workflow.Status.Running(v1alpha1.ConditionReasonRunning.String(), message)
}

func (wc *WorkflowContext) getWorkflowTaskCount() WorkflowTaskCount {
	var count WorkflowTaskCount
	for _, task := range wc.Tasks {
		switch {
		case task.IsSucceeded():
			count.Succeeded++
		case task.IsCancelled():
			count.Cancelled++
		case task.IsFailed():
			count.Failed++
		case task.IsSkipped():
			count.Skipped++
		case task.IsRunning():
			count.Running++
		case task.IsPaused():
			count.Paused++
		default:
			count.Pending++
		}
	}
	return count
}

func (wc *WorkflowContext) calculateTaskStatus() []v1alpha1.WorkflowTaskStatus {
	taskStatuses := make([]v1alpha1.WorkflowTaskStatus, len(wc.Tasks))
	for i, task := range wc.Tasks {
		taskStatuses[i] = v1alpha1.WorkflowTaskStatus{
			Name:             task.WorkflowTask.Name,
			DisplayName:      task.WorkflowTask.DisplayName,
			Labels:           task.WorkflowTask.Labels,
			SuccessCondition: nil,
		}
		if task.Task != nil {
			taskStatuses[i].SuccessCondition = task.Task.Status.GetCondition(v1alpha1.ConditionSucceeded)
			taskStatuses[i].StartedAt = task.Task.Status.StartedAt
			taskStatuses[i].FinishedAt = task.Task.Status.FinishedAt
			taskStatuses[i].WorkloadRelease = task.Task.Status.WorkloadRelease
		}
	}
	return taskStatuses
}

// RunTasks runs the tasks in the workflow context
func (wc *WorkflowContext) RunTasks(ctx context.Context, client client.Client) error {
	// resume suspended tasks if needed
	if err := wc.ResumeSuspendedTasks(ctx, client); err != nil {
		return err
	}

	taskMap := make(map[string]*v1alpha1.Task)
	runnableTasks := wc.DAGRunnableTasks()
	for _, rt := range runnableTasks {
		task, err := wc.CreateTask(ctx, client, rt)
		if err != nil {
			return err
		}
		taskMap[task.GenerateName] = task
	}
	for i := range wc.Tasks {
		rt := &wc.Tasks[i]
		if task, ok := taskMap[rt.GenerateName]; ok {
			rt.Task = task
		}
	}
	return nil
}

func (wc *WorkflowContext) CreateTask(ctx context.Context, c client.Client, rt resolvetask.ResolvedTask) (*v1alpha1.Task, error) {
	resource := &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: rt.GenerateName,
			Namespace:    rt.Namespace,
			Labels: map[string]string{
				api.LabelGeneratedBy: wc.Workflow.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: wc.Workflow.APIVersion,
					Kind:       wc.Workflow.Kind,
					Name:       wc.Workflow.Name,
					UID:        wc.Workflow.UID,
					Controller: boolPtr(true),
				},
			},
		},
		Spec: rt.WorkflowTask.TaskSpec,
	}
	err := c.Create(ctx, resource)
	if err != nil {
		return nil, err
	}
	return resource, nil
}

// ResumeSuspendedTasks resumes the suspended tasks in the workflow context
func (wc *WorkflowContext) ResumeSuspendedTasks(ctx context.Context, c client.Client) error {
	taskStr, ok := wc.Workflow.Annotations[api.AnnoWorkflowResumeSuspendTasks]
	if !ok {
		return nil
	}
	var taskNames []string
	if err := json.Unmarshal([]byte(taskStr), &taskNames); err != nil {
		return err
	}
	taskSet := sets.NewString(taskNames...)
	for _, task := range wc.Tasks {
		if task.Type == v1alpha1.TaskTypeSuspend && taskSet.Has(task.WorkflowTask.Name) && task.Task != nil {
			resource := task.Task
			if resource.Status.IsPaused() {
				resource.Status.Succeed("Resumed")
				if err := c.Status().Update(ctx, resource); err != nil {
					return err
				}
				// TODO: add event
			}
		}
	}
	return nil
}

// convert bool to pointer
func boolPtr(b bool) *bool {
	return &b
}
