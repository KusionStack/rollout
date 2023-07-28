package task

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	paramNameRunningSeconds = "runningSeconds"
	paramNameSuccess        = "success"
	paramNameExpectStatus   = "expectStatus"
)

var (
	paramRunningSeconds int
	paramSuccess        = true
	paramExpectStatus   = ""
)

// EchoExecutor is the executor for echo task
type EchoExecutor struct {
	TaskExecutor
}

func (e *EchoExecutor) Run(ctx context.Context) (time.Duration, error) {
	logger := log.FromContext(ctx)

	params := e.TaskExecutor.Task.Spec.Params
	for _, param := range params {
		switch param.Name {
		case paramNameRunningSeconds:
			paramRunningSeconds = param.Value.IntValue
		case paramNameSuccess:
			paramSuccess = param.Value.BoolValue
		case paramNameExpectStatus:
			paramExpectStatus = param.Value.StringValue
		}
	}

	if paramRunningSeconds > 0 {
		d := e.Task.Status.StartedAt.Add(time.Duration(paramRunningSeconds) * time.Second).Sub(time.Now())
		if d > 0 {
			logger.Info("Echo task is running", "requeueDuration", d)
			e.Result.Status = TaskResultStatusRunning
			e.Result.Reason = "Running"
			e.Result.Message = "Echo task is running"
			return d, nil
		}
	}

	// do echo
	message := fmt.Sprintf("Echo message: %q\n", e.Task.Spec.Echo.Message)
	fmt.Printf(message)

	if paramExpectStatus != "" {
		e.Result.Reason = "Expected"
		e.Result.Message = message
		switch paramExpectStatus {
		case string(TaskResultStatusSucceeded):
			e.Result.Status = TaskResultStatusSucceeded
		case string(TaskResultStatusFailed):
			e.Result.Status = TaskResultStatusFailed
		case string(TaskResultStatusRunning):
			e.Result.Status = TaskResultStatusRunning
		case "Paused":
			e.Result.Status = TaskResultStatusRunning
			e.Result.Reason = "Paused"
		case "Pending":
			e.Result.Status = TaskResultStatusRunning
			e.Result.Reason = "Pending"
		case "Cancelled":
			e.Result.Status = TaskResultStatusFailed
			e.Result.Reason = "Cancelled"
		default:
			e.Result.Status = TaskResultStatusFailed
			e.Result.Reason = "UnknownStatus"
		}

		return 0, nil
	}

	if paramSuccess {
		e.Result.Status = TaskResultStatusSucceeded
		e.Result.Reason = "Succeeded"
		e.Result.Message = message
	} else {
		e.Result.Status = TaskResultStatusFailed
		e.Result.Reason = "Failed"
		e.Result.Message = message
	}

	return 0, nil
}
