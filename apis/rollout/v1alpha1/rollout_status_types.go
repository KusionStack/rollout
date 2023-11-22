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

// RolloutStatus defines the observed state of Rollout
type RolloutStatus struct {
	// ObservedGeneration is the most recent generation observed for this Rollout. It corresponds to the
	// Rollout's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions is the list of conditions
	Conditions []Condition `json:"conditions,omitempty"`
	// Phase indicates the current phase of rollout
	Phase RolloutPhase `json:"phase,omitempty"`
	// RolloutID is reference to workflow name now
	RolloutID string `json:"rolloutID,omitempty"`
	// BatchStatus describes the state of the active batch release
	BatchStatus *BatchStatus `json:"batchStatus,omitempty"`
	// The last time this status was updated.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
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
	// RolloutRunPhaseCompleted indicates the rolloutRun is finished
	RolloutRunPhaseCompleted RolloutPhase = "Completed"
)

const (
	// rollout condition types

	// RolloutConditionProgressing means the rollout is progressing
	RolloutConditionProgressing ConditionType = "Progressing"
	// RolloutConditionCompleted means the rollout is Terminating
	RolloutConditionTerminating ConditionType = "Terminating"
	// RolloutConditionTrigger means the rollout is
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

// BatchStatus defines the status of batch release.
type BatchStatus struct {
	// CurrentBatchIndex defines the current batch index of batch release progress.
	CurrentBatchIndex int32 `json:"currentBatchIndex"`
	// CurrentBatchIndex indicates the current batch state.
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
	// PodTemplateHash is used to distinguish different version of pod
	PodTemplateHash string `json:"podTemplateHash,omitempty"`
}

type RolloutStepState string

const (
	// RolloutStepStatePending means the step is pending.
	RolloutStepStatePending RolloutStepState = "Pending"
	// RolloutStepStateRunning means the step is running.
	RolloutStepStateRunning RolloutStepState = "Running"
	// RolloutStepStateSucceeded means the step is completed.
	RolloutStepStateSucceeded RolloutStepState = "Succeeded"
	// RolloutStepStatePaused means the step is paused.
	RolloutStepStatePaused RolloutStepState = "Paused"
	// RolloutStepStateCanceled means the step is canceled.
	RolloutStepStateCanceled RolloutStepState = "Canceled"
	// RolloutStepStateError means the step is error.
	RolloutStepStateError RolloutStepState = "Error"
)
