package task

import (
	"context"
	"errors"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/KusionStack/rollout/api/v1alpha1"
)

type TaskExecutor struct {
	Task   *v1alpha1.Task
	Result TaskResult
}

type TaskResult struct {
	Status  TaskResultStatusType
	Reason  string
	Message string
}

// TaskExecutor is the interface for task executor
type Executor interface {
	Run(ctx context.Context) (time.Duration, error)
	CalculateStatus()
}

// NewTaskExecutor creates a new task executor
func NewTaskExecutor(client client.Client, task *v1alpha1.Task) (Executor, error) {
	var executor Executor
	var err error
	switch task.Spec.GetType() {
	case v1alpha1.TaskTypeEcho:
		executor = &EchoExecutor{TaskExecutor{Task: task}}
	case v1alpha1.TaskTypeWorkloadRelease:
		executor, err = NewWorkloadReleaseExecutor(client, task)
	case v1alpha1.TaskTypeSuspend:
		executor, err = NewSuspendExecutor(task)
	default:
		return nil, errors.New("unknown task type")
	}

	return executor, err
}

// TaskResultStatusType is the type of task result status
type TaskResultStatusType string

const (
	// TaskResultStatusSucceeded is the status of succeeded task
	TaskResultStatusSucceeded TaskResultStatusType = "Succeeded"
	// TaskResultStatusFailed is the status of failed task
	TaskResultStatusFailed TaskResultStatusType = "Failed"
	// TaskResultStatusRunning is the status of running task
	TaskResultStatusRunning TaskResultStatusType = "Running"
)

// CalculateStatus calculates the status of the task
func (e *TaskExecutor) CalculateStatus() {
	switch e.Result.Status {
	case TaskResultStatusSucceeded:
		e.Task.Status.Succeed(e.Result.Message)
	case TaskResultStatusFailed:
		e.Task.Status.Fail(e.Result.Reason, e.Result.Message)
	case TaskResultStatusRunning:
		e.Task.Status.Running(e.Result.Reason, e.Result.Message)
	}
}
