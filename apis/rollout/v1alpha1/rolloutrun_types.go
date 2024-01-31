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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ror

type RolloutRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RolloutRunSpec   `json:"spec,omitempty"`
	Status RolloutRunStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// RolloutList contains a list of Rollout
type RolloutRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RolloutRun `json:"items"`
}

type RolloutRunSpec struct {
	// TargetType defines the GroupVersionKind of target resource
	TargetType ObjectTypeRef `json:"targetType,omitempty"`

	// Webhooks defines rollout webhook configuration
	Webhooks []RolloutWebhook `json:"webhooks,omitempty"`

	// Canary defines the canary strategy
	// +optional
	Canary *RolloutRunCanaryStrategy `json:"canary,omitempty"`

	// Batch Strategy
	// +optional
	Batch *RolloutRunBatchStrategy `json:"batch,omitempty"`
}

type RolloutRunBatchStrategy struct {
	// Batches define the order of phases to execute release in batch release
	Batches []RolloutRunStep `json:"batches,omitempty"`

	// Toleration is the toleration policy of the canary strategy
	// +optional
	Toleration *TolerationStrategy `json:"toleration,omitempty"`
}

type RolloutRunStep struct {
	// desired target replicas
	Targets []RolloutRunStepTarget `json:"targets"`

	// traffic strategy
	// +optional
	Traffic *TrafficStrategy `json:"traffic,omitempty"`

	// If set to true, the rollout will be paused before the step starts.
	// +optional
	Breakpoint bool `json:"breakpoint,omitempty"`

	// Properties contains additional information for step
	// +optional
	Properties map[string]string `json:"properties,omitempty"`
}

type RolloutRunCanaryStrategy struct {
	// desired target replicas
	Targets []RolloutRunStepTarget `json:"targets"`

	// traffic strategy
	// +optional
	Traffic *TrafficStrategy `json:"traffic,omitempty"`

	// Properties contains additional information for step
	// +optional
	Properties map[string]string `json:"properties,omitempty"`

	// PodTemplateMetadataPatch defines a patch for workload podTemplate metadata.
	// +optional
	PodTemplateMetadataPatch *PodTemplateMetadataPatch `json:"podTemplateMetadataPatch,omitempty"`
}

type RolloutRunStepTarget struct {
	CrossClusterObjectNameReference `json:",inline"`

	// Replicas is the replicas of the rollout task, which represents the number of pods to be upgraded
	Replicas intstr.IntOrString `json:"replicas"`
}

type RolloutRunStatus struct {
	// ObservedGeneration is the most recent generation observed for this Rollout. It corresponds to the
	// Rollout's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions is the list of conditions
	Conditions []Condition `json:"conditions,omitempty"`
	// Phase indecates the current phase of rollout
	Phase RolloutRunPhase `json:"phase,omitempty"`
	// The last time this status was updated.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
	// Error indicates the error info of progressing
	Error *CodeReasonMessage `json:"error,omitempty"`
	// CanaryStatus describes the state of the active canary release
	// +optional
	CanaryStatus *RolloutRunStepStatus `json:"canaryStatus,omitempty"`
	// BatchStatus describes the state of the active batch release
	// +optional
	BatchStatus *RolloutRunBatchStatus `json:"batchStatus,omitempty"`
	// TargetStatuses describes the referenced workloads status
	// +optional
	TargetStatuses []RolloutWorkloadStatus `json:"targetStatuses,omitempty"`
}

type RolloutRunBatchStatus struct {
	// RolloutBatchStatus contains status of current batch
	RolloutBatchStatus `json:",inline"`
	// Records contains all batches status details.
	Records []RolloutRunStepStatus `json:"records,omitempty"`
}

type RolloutRunPhase string

const (
	// RolloutRunPhaseInitial defines the initial phase of rolloutRun
	RolloutRunPhaseInitial RolloutRunPhase = "Initial"
	// RolloutRunPhasePreRollout defines the phase of rolloutRun before rollout
	RolloutRunPhasePreRollout RolloutRunPhase = "PreRollout"
	// RolloutRunPhasePausing defines the phase of rolloutRun pausing
	RolloutRunPhasePausing RolloutRunPhase = "Pausing"
	// RolloutRunPhasePaused defines the phase of rolloutRun paused
	RolloutRunPhasePaused RolloutRunPhase = "Paused"
	// RolloutRunPhaseProgressing defines the phase of rolloutRun progressing
	RolloutRunPhaseProgressing RolloutRunPhase = "Progressing"
	// RolloutRunPhasePostRollout defines the phase of rollout after progressing
	RolloutRunPhasePostRollout RolloutRunPhase = "PostRollout"
	// RolloutRunPhaseCanceling defines the phase of rolloutRun canceling
	RolloutRunPhaseCanceling RolloutRunPhase = "Canceling"
	// RolloutRunPhaseCanceled defines the phase of rolloutRun canceled
	RolloutRunPhaseCanceled RolloutRunPhase = "Canceled"
	// RolloutRunPhaseFailed defines the phase of rolloutRun succeeded
	RolloutRunPhaseSucceeded RolloutRunPhase = "Succeeded"
)

type RolloutRunStepStatus struct {
	// Index is the id of the batch
	Index *int32 `json:"index,omitempty"`
	// State is Rollout step state
	State RolloutStepState `json:"state,omitempty"`
	// StartTime is the time when the stage started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// FinishTime is the time when the stage finished
	// +optional
	FinishTime *metav1.Time `json:"finishTime,omitempty"`
	// WorkloadDetails contains release details for each workload
	// +optional
	Targets []RolloutWorkloadStatus `json:"targets,omitempty"`
	// Webhooks contains webhook status
	// +optional
	Webhooks []RolloutWebhookStatus `json:"webhooks,omitempty"`
}

type RolloutWebhookStatus struct {
	// Current webhook worker state
	State RolloutWebhookState `json:"state,omitempty"`
	// Webhook Type
	HookType HookType `json:"hookType,omitempty"`
	// Webhook Name
	Name string `json:"name,omitempty"`
	// Webhook result
	CodeReasonMessage `json:",inline"`
	// Failure count
	FailureCount int32 `json:"failureCount,omitempty"`
}

// RolloutWebhookState indicates current state of webhook webhook.
type RolloutWebhookState string

const (
	WebhookRunning   RolloutWebhookState = "Running"
	WebhookOnHold    RolloutWebhookState = "OnHold"
	WebhookCompleted RolloutWebhookState = "Completed"
)

func (r *RolloutRun) IsCompleted() bool {
	if r == nil {
		return false
	}
	return r.Status.Phase == RolloutRunPhaseSucceeded || r.Status.Phase == RolloutRunPhaseCanceled
}
