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

package v1alpha1

import "time"

// SuspendTask is a task that suspends the workflow
type SuspendTask struct {
	// Approved indicates whether the task is approved to continue
	// +optional
	Approved *bool `json:"approved,omitempty"`

	// Duration is the duration to suspend the workflow
	// +optional
	Duration *time.Duration `json:"duration,omitempty"`
}
