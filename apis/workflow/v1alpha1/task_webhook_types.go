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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

type MapParams map[string]string

// WebhookTask is a task that performs webhook, mainly used for canary scenarios
type WebhookTask struct {
	// HookType is the type of the check point
	HookType rolloutv1alpha1.HookType `json:"hookType"`

	// Name indicates the provider name.
	Name string `json:"name"`

	// Action is the operation that would be executed.
	Action string `json:"action"`

	// AllBatchParams contains all batches parameters from rollout strategy.
	AllBatchParams map[string]MapParams `json:"allBatchParams"`
}

// WebhookTaskStatus contains the result of webhook checks.
type WebhookTaskStatus struct {
	// HookType indicates the type of webhook
	HookType rolloutv1alpha1.HookType `json:"hookType"`

	// Provider indicates the provider name.
	Provider string `json:"provider"`

	// Passed indicates whether the webhook check has passed or not.
	Passed bool `json:"passed"`

	// Status is the webhook check task status.
	Status string `json:"status"`

	// Verdict is the decision of webhook check if it is not passed.
	Verdict string `json:"verdict"`

	// StartTimestamp is the time when the task is started
	// +optional
	StartTimestamp *metav1.Time `json:"startTimestamp,omitempty"`

	// EndTimestamp is the time when the task is finished
	// +optional
	EndTimestamp *metav1.Time `json:"endTimestamp,omitempty"`
}
