/**
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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=ros

// RolloutStrategy is the Schema for the rolloutstrategies API
type RolloutStrategy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Batch is the batch strategy for upgrade and operation
	// +optional
	Batch *BatchStrategy `json:"batch,omitempty"`

	// Webhooks defines
	Webhooks []RolloutWebhook `json:"webhooks,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// RolloutStrategyList contains a list of RolloutStrategy
type RolloutStrategyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RolloutStrategy `json:"items"`
}

// BatchStrategy defines the batch strategy
type BatchStrategy struct {
	// Batches define the order of phases to execute release in canary release
	Batches []RolloutStep `json:"batches,omitempty"`
	// Toleration is the toleration policy of the canary strategy
	// +optional
	Toleration *TolerationStrategy `json:"toleration,omitempty"`
}

// TolerationStrategy defines the toleration strategy
type TolerationStrategy struct {
	// WorkloadFailureThreshold indicates how many failed pods can be tolerated in all upgraded pods of one workload.
	// The default value is 0, which means no failed pods can be tolerated.
	// This is a workload level threshold.
	// +optional
	WorkloadFailureThreshold *intstr.IntOrString `json:"workloadTotalFailureThreshold,omitempty"`

	// FailureThreshold indicates how many failed pods can be tolerated before marking the rollout task as success
	// If not set, the default value is 0, which means no failed pods can be tolerated
	// This is a task level threshold.
	// +optional
	TaskFailureThreshold *intstr.IntOrString `json:"taskFailureThreshold,omitempty"`

	// Number of seconds after the toleration check has started before the task are initiated.
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
}

// Custom release step
type RolloutStep struct {
	// Replicas is the replicas of the rollout task, which represents the number of pods to be upgraded
	Replicas intstr.IntOrString `json:"replicas"`

	// traffic strategy
	Traffic *TrafficStrategy `json:"traffic,omitempty"`

	// Match defines condition used for matching resource cross clusterset
	Match *ResourceMatch `json:"matchTargets,omitempty"`

	// If set to true, the rollout will be paused before the step starts.
	Breakpoint bool `json:"breakpoint,omitempty"`

	// Properties contains additional information for step
	Properties map[string]string `json:"properties,omitempty"`
}
