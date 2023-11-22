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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// WorkloadReleaseTask is the task for workload release
type WorkloadReleaseTask struct {
	// Workload is the workload of the task
	Workload Workload `json:"workload"`

	// Partition is the partition of the workload release
	// +optional
	Partition intstr.IntOrString `json:"partition,omitempty"`

	// ReadyCondition is the condition to check if the task is successful
	// +optional
	ReadyCondition *ReadyCondition `json:"readyCondition,omitempty"`
}

// Workload is the workload of the task
type Workload struct {
	// APIVersion is the group/version for the resource being referenced.
	// If APIVersion is not specified, the specified Kind must be in the core API group.
	// For any other third-party types, APIVersion is required.
	// +optional
	APIVersion string `json:"apiVersion"`
	// Kind is the type of resource being referenced
	Kind string `json:"kind"`

	// Name is the name of the workload
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the workload
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Cluster is the cluster of the task
	// +optional
	Cluster string `json:"cluster,omitempty"`
}

// ReadyCondition is the condition to check if the task is successful
type ReadyCondition struct {
	// AtLeastWaitingSeconds is the seconds to wait at least
	// +optional
	AtLeastWaitingSeconds int32 `json:"atLeastWaitingSeconds,omitempty"`

	// FailureThreshold indicates how many failed pods can be tolerated before marking the task as success in the workload scope
	// +optional
	FailureThreshold intstr.IntOrString `json:"failureThreshold,omitempty"`

	// TaskFailureThreshold indicates how many failed pods can be tolerated before marking the task as success in the task scope
	// +optional
	TaskFailureThreshold intstr.IntOrString `json:"taskFailureThreshold,omitempty"`
}

// WorkloadReleaseTaskStatus defines the status of the workload release task
type WorkloadReleaseTaskStatus struct {
	// Name is the name of the workload
	Name string `json:"name,omitempty"`

	// ObservedGenerationUpdated indicates if the observedGeneration is updated to the latest generation
	ObservedGenerationUpdated bool `json:"observedGenerationUpdated"`

	// Replicas is the number of replicas
	Replicas int32 `json:"replicas"`

	// UpdatedReplicas is the number of updated replicas
	UpdatedReplicas int32 `json:"updatedReplicas"`

	// UpdatedReadyReplicas is the number of updated ready replicas
	UpdatedReadyReplicas int32 `json:"updatedReadyReplicas"`

	// UpdatedAvailableReplicas is the number of updated available replicas
	UpdatedAvailableReplicas int32 `json:"updatedAvailableReplicas"`
}
