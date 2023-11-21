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

	// TriggerPolicy defines when rollout will be triggered
	TriggerPolicy RolloutTriggerPolicy `json:"triggerPolicy,omitempty"`

	// StrategyRef is the reference to the rollout strategy
	StrategyRef string `json:"strategyRef,omitempty"`

	// WorkloadRef is a reference to a kind of workloads
	WorkloadRef WorkloadRef `json:"workloadRef,omitempty"`
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
	Kind string `json:"kind"`
	// Match indicates how to match workloads. only one workload should be matches in one cluster
	Match ResourceMatch `json:"match"`
}

type ResourceMatch struct {
	// Selector is a label query over a set of resources, in this case resource
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
	// Names is a list of workload name
	Names []CrossClusterObjectNameReference `json:"names,omitempty"`
}

const (
	MatchAllCluster = ""
)

// CrossClusterObjectNameReference contains cluster and name reference to a k8s object
type CrossClusterObjectNameReference struct {
	// Cluster indicates the name of cluster
	Cluster string `json:"cluster,omitempty"`
	// Name is the resource name
	Name string `json:"name,omitempty"`
}

func (r CrossClusterObjectNameReference) Matches(cluster, name string) bool {
	if r.Name != name {
		// object name is not matched
		return false
	}
	if r.Cluster == MatchAllCluster || cluster == MatchAllCluster {
		// match all clusters
		return true
	}
	return r.Cluster == cluster
}
