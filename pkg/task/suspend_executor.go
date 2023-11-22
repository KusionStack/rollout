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
	"time"

	"kusionstack.io/rollout/apis/workflow/v1alpha1"
)

type suspendExecutor struct {
	TaskExecutor
}

func (e *suspendExecutor) Run(ctx context.Context) (time.Duration, error) {
	e.Result.Reason = string(v1alpha1.WorkflowReasonPaused)
	e.Result.Status = TaskResultStatusRunning
	return 0, nil
}

func NewSuspendExecutor(task *v1alpha1.Task) (Executor, error) {
	return &suspendExecutor{TaskExecutor{Task: task}}, nil
}
