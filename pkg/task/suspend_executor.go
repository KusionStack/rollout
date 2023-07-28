package task

import (
	"context"
	"github.com/KusionStack/rollout/api/v1alpha1"
	"time"
)

type suspendExecutor struct {
	TaskExecutor
}

func (e *suspendExecutor) Run(ctx context.Context) (time.Duration, error) {
	e.Result.Reason = string(v1alpha1.ConditionReasonPaused)
	e.Result.Status = TaskResultStatusRunning
	return 0, nil
}

func NewSuspendExecutor(task *v1alpha1.Task) (Executor, error) {
	return &suspendExecutor{TaskExecutor{Task: task}}, nil
}
