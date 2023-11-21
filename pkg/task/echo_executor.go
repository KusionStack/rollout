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
		d := time.Until(e.Task.Status.StartedAt.Add(time.Duration(paramRunningSeconds) * time.Second))
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
	logger.Info(message)

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
		case "Canceled":
			e.Result.Status = TaskResultStatusFailed
			e.Result.Reason = "Canceled"
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
