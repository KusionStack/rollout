// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceMatch struct {
	// Selector is a label query over a set of resources, in this case resource
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
	// Names is a list of workload name
	Names []CrossClusterObjectNameReference `json:"names,omitempty"`
}

type ObjectTypeRef struct {
	// APIVersion is the group/version for the resource being referenced.
	// If APIVersion is not specified, the specified Kind must be in the core API group.
	// For any other third-party types, APIVersion is required.
	// +optional
	APIVersion string `json:"apiVersion"`
	// Kind is the type of resource being referenced
	Kind string `json:"kind"`
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

type CrossClusterObjectReference struct {
	ObjectTypeRef                   `json:",inline"`
	CrossClusterObjectNameReference `json:",inline"`
}

type CodeReasonMessage struct {
	// Code is a globally unique identifier
	Code string `json:"code,omitempty"`
	// A human-readable short word
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human-readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}
