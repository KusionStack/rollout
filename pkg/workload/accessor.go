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

package workload

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
)

// Accessor defines the functions to access the workload.
type Accessor interface {
	GroupVersionKind() schema.GroupVersionKind
	// NewObject returns a new instance of the workload type
	NewObject() client.Object
	// NewObjectList returns a new instance of the workload list type
	NewObjectList() client.ObjectList
	// Watchable indicates whether this workload type can be watched from the API server.
	Watchable() bool
	// GetInfo returns a info represent workload
	GetInfo(cluster string, obj client.Object) (*Info, error)
	// ReleaseControl returns the release control for the workload
	ReleaseControl() ReleaseControl
}

// ReleaseControl defines the control functions for workload release
type ReleaseControl interface {
	// BatchPreCheck checks object before batch release.
	BatchPreCheck(obj client.Object) error
	// ApplyPartition applies partition to the workload
	ApplyPartition(obj client.Object, partition intstr.IntOrString) error
	// CanaryPreCheck checks object before canary release.
	CanaryPreCheck(obj client.Object) error
	// Scale scales the workload replicas.
	Scale(obj client.Object, replicas int32) error
	// ApplyCanaryPatch applies canary to the workload.
	ApplyCanaryPatch(canary client.Object, podTemplatePatch *v1alpha1.MetadataPatch) error
}
