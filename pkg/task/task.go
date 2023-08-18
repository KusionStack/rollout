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

package task

import "github.com/KusionStack/rollout/api/v1alpha1"

type ResolvedTask struct {
	Namespace string
	//Name          string
	GenerateName string
	WorkflowTask *v1alpha1.WorkflowTask
	Task         *v1alpha1.Task
	Type         v1alpha1.TaskType
}

// IsSucceeded returns true if the task is successful
func (rt *ResolvedTask) IsSucceeded() bool {
	return rt.Task != nil && rt.Task.Status.IsSucceeded()
}

// IsFailed returns true if the task is failed
func (rt *ResolvedTask) IsFailed() bool {
	return rt.Task != nil && rt.Task.Status.IsFailed()
}

// IsRunning returns true if the task is running
func (rt *ResolvedTask) IsRunning() bool {
	return rt.Task != nil && rt.Task.Status.IsRunning()
}

// IsPaused returns true if the task is paused
func (rt *ResolvedTask) IsPaused() bool {
	return rt.Task != nil && rt.Task.Status.IsPaused()
}

// IsCancelled returns true if the task is cancelled
func (rt *ResolvedTask) IsCancelled() bool {
	return rt.Task != nil && rt.Task.Status.IsCancelled()
}

// IsSkipped returns true if the task is skipped
func (rt *ResolvedTask) IsSkipped() bool {
	return rt.Task != nil && rt.Task.Status.IsSkipped()
}

// IsCompleted returns true if the task is completed
func (rt *ResolvedTask) IsCompleted() bool {
	return rt.Task != nil && rt.Task.Status.IsCompleted()
}
