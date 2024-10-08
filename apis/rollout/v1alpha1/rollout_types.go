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
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ro
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=".status.conditions[?(@.type=='Available')].status"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.rolloutID"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",format="date-time"

// Rollout is the Schema for the rollouts API
type Rollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RolloutSpec   `json:"spec,omitempty"`
	Status RolloutStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// RolloutList contains a list of Rollout
type RolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rollout `json:"items"`
}

// RolloutSpec defines the desired state of Rollout
type RolloutSpec struct {
	// Disabled means that rollout will not response for new event.
	// Default value is false.
	Disabled bool `json:"disabled,omitempty"`

	// HistoryLimit defines the maximum number of completed rolloutRun
	// history records to keep.
	// The HistoryLimit can start from 0 (no retained RolloutRun history).
	// When not set or set to math.MaxInt32, the Rollout will keep all RolloutRun history records.
	//
	// +kubebuilder:default=10
	HistoryLimit *int32 `json:"historyLimit,omitempty"`

	// TriggerPolicy defines when rollout will be triggered
	//
	// +kubebuilder:default=Auto
	TriggerPolicy RolloutTriggerPolicy `json:"triggerPolicy,omitempty"`

	// StrategyRef is the reference to the rollout strategy
	//
	// +kubebuilder:validation:Required
	StrategyRef string `json:"strategyRef,omitempty"`

	// WorkloadRef is a reference to a kind of workloads
	WorkloadRef WorkloadRef `json:"workloadRef,omitempty"`

	// TrafficTopologyRefs defines the networking traffic relationships between
	// workloads, backend services, and routes.
	TrafficTopologyRefs []string `json:"trafficTopologyRefs,omitempty"`
}

type RolloutTriggerPolicy string

const (
	// AutoTriggerPolicy specifies the rollout progress will be triggered when all related
	// workloads are waiting for rolling update, it is the default policy.
	AutoTriggerPolicy RolloutTriggerPolicy = "Auto"

	// ManualTriggerPolicy specifies the rollout will only triggered by manually.
	ManualTriggerPolicy RolloutTriggerPolicy = "Manual"
)

// WorkloadRef is a reference to a workload
type WorkloadRef struct {
	// APIVersion is the group/version for the resource being referenced.
	// If APIVersion is not specified, the specified Kind must be in the core API group.
	// For any other third-party types, APIVersion is required.
	// +optional
	APIVersion string `json:"apiVersion"`
	// Kind is the type of resource being referenced
	//
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`
	// Match indicates how to match workloads. only one workload should be matches in one cluster
	Match ResourceMatch `json:"match"`
}

// RolloutStatus defines the observed state of Rollout
type RolloutStatus struct {
	// ObservedGeneration is the most recent generation observed for this Rollout. It corresponds to the
	// Rollout's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions is the list of conditions
	Conditions []Condition `json:"conditions,omitempty"`
	// Phase indicates the current phase of rollout
	Phase RolloutPhase `json:"phase,omitempty"`
	// The last time this status was updated.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
	// RolloutID is reference to rolloutRun name.
	RolloutID string `json:"rolloutID,omitempty"`
}

// RolloutPhase indicates the current rollout phase
type RolloutPhase string

const (
	// RolloutPhaseInitialized indicates the rollout is ready and waiting for next trigger
	RolloutPhaseInitialized RolloutPhase = "Initialized"
	// RolloutPhaseTerminating indicates the rollout is disabled
	RolloutPhaseDisabled RolloutPhase = "Disabled"
	// RolloutPhaseProgressing indicates the rollout is progressing
	RolloutPhaseProgressing RolloutPhase = "Progressing"
	// RolloutPhaseTerminating indicates the rollout is terminating
	RolloutPhaseTerminating RolloutPhase = "Terminating"
)

const (
	// rollout condition types

	// Available means all the dependents of this Rollout are available.
	RolloutConditionAvailable ConditionType = "Available"
	// RolloutConditionProgressing means the rollout is progressing
	RolloutConditionProgressing ConditionType = "Progressing"
	// RolloutConditionCompleted means the rollout is Terminating
	RolloutConditionTerminating ConditionType = "Terminating"
	// RolloutConditionTrigger means the rollout is triggered.
	RolloutConditionTrigger ConditionType = "Trigger"

	// rollout condition reasons

	// RolloutReasonTerminatingCompleted means the termination of rollout is Completed.
	RolloutReasonTerminatingCompleted = "Completed"
	// RolloutReasonProgressingRunning means the rollout is not triggered.
	RolloutReasonProgressingUnTriggered = "UnTriggered"
	// RolloutReasonProgressingRunning means the rollout is running.
	RolloutReasonProgressingRunning = "Running"
	// RolloutReasonProgressingCompleted means the rollout is completed.
	RolloutReasonProgressingCompleted = "Completed"
	// RolloutReasonProgressingCanceled means the rollout is completed.
	RolloutReasonProgressingCanceled = "Canceled"
	// RolloutReasonProgressingError means the rollout is completed.
	RolloutReasonProgressingError = "Error"
)

// RolloutBatchStatus defines the status of batch release.
type RolloutBatchStatus struct {
	// CurrentBatchIndex defines the current batch index of batch release progress.
	CurrentBatchIndex int32 `json:"currentBatchIndex"`
	// CurrentBatchState indicates the current batch state.
	CurrentBatchState RolloutStepState `json:"currentBatchState,omitempty"`
}

type RolloutReplicasSummary struct {
	// Replicas is the desired number of pods targeted by workload
	Replicas int32 `json:"replicas"`
	// UpdatedReplicas is the number of pods targeted by workload that have the updated template spec.
	UpdatedReplicas int32 `json:"updatedReplicas"`
	// UpdatedReadyReplicas is the number of ready pods targeted by workload that have the updated template spec.
	UpdatedReadyReplicas int32 `json:"updatedReadyReplicas"`
	// UpdatedAvailableReplicas is the number of service available pods targeted by workload that have the updated template spec.
	UpdatedAvailableReplicas int32 `json:"updatedAvailableReplicas"`
}

type RolloutWorkloadStatus struct {
	// summary of replicas
	RolloutReplicasSummary `json:",inline,omitempty"`

	// Name is the workload name
	Name string `json:"name,omitempty"`
	// Cluster defines which cluster the workload is in.
	Cluster string `json:"cluster,omitempty"`
	// Generation is the found in workload metadata.
	Generation int64 `json:"generation,omitempty"`
	// ObservedGeneration is the most recent generation observed for this workload.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// StableRevision is the old stable revision used to generate pods.
	StableRevision string `json:"stableRevision,omitempty"`
	// UpdatedRevision is the updated template revision used to generate pods.
	UpdatedRevision string `json:"updatedRevision,omitempty"`
}

type RolloutStepState string

const (
	// RolloutStepNone indicates that the step is not started.
	RolloutStepNone RolloutStepState = ""

	// RolloutStepPending indicates that the step is pending.
	RolloutStepPending RolloutStepState = "Pending"

	// RolloutStepPreCanaryStepHook indicates that the step is in the pre-canary hook.
	RolloutStepPreCanaryStepHook RolloutStepState = RolloutStepState(PreCanaryStepHook)

	// RolloutStepPreBatchStepHook indicates that the step is in the pre-batch hook.
	RolloutStepPreBatchStepHook RolloutStepState = RolloutStepState(PreBatchStepHook)

	// RolloutStepRunning indicates that the step is running.
	RolloutStepRunning RolloutStepState = "Running"

	// RolloutStepPostCanaryStepHook indicates that the step is in the post-canary hook.
	RolloutStepPostCanaryStepHook RolloutStepState = RolloutStepState(PostCanaryStepHook)

	// RolloutStepPostBatchStepHook indicates that the step is in the post-batch hook.
	RolloutStepPostBatchStepHook RolloutStepState = RolloutStepState(PostBatchStepHook)

	// RolloutStepSucceeded indicates that the step is completed.
	RolloutStepSucceeded RolloutStepState = "Succeeded"

	// RolloutStepResourceRecycling indicates that the step is recycling resources.
	// In Canary strategy, it occurs after the user confirms (Paused).
	// In Batch strategy, it occurs before the PreBatchStepHook.
	RolloutStepResourceRecycling RolloutStepState = "ResourceRecycling"
)
