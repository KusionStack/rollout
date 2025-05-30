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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
)

// Accessor defines the functions to access the workload.
// The following interfaces are optional:
// - CanaryReleaseControl
// - BatchReleaseControl
// - PodControl
type Accessor interface {
	// GroupVersionKind returns the GroupVersionKind of the workload
	GroupVersionKind() schema.GroupVersionKind
	// DependentWorkloadGVKs returns the dependent workloadds' GroupVersionKinds
	DependentWorkloadGVKs() []schema.GroupVersionKind
	// NewObject returns a new instance of the workload type
	NewObject() client.Object
	// NewObjectList returns a new instance of the workload list type
	NewObjectList() client.ObjectList
	// Watchable indicates whether this workload type can be watched from the API server.
	Watchable() bool
	// GetInfo returns a info represent workload
	GetInfo(cluster string, obj client.Object) (*Info, error)
}

// BatchReleaseControl defines the control functions for workload batch release
type BatchReleaseControl interface {
	// BatchPreCheck checks object before batch release.
	BatchPreCheck(obj client.Object) error
	// ApplyPartition use expectedUpdated replicas to calculate partition and apply it to the workload.
	ApplyPartition(obj client.Object, expectedUpdatedReplicas int32) error
}

// CanaryReleaseControl defines the control functions for workload canary release
type CanaryReleaseControl interface {
	// CanaryPreCheck checks object before canary release.
	CanaryPreCheck(obj client.Object) error
	// Scale scales the workload replicas.
	Scale(obj client.Object, replicas int32) error
	// ApplyCanaryPatch applies canary to the workload.
	ApplyCanaryPatch(canary client.Object, podTemplatePatch *v1alpha1.MetadataPatch) error
}

type PodControl interface {
	// IsUpdatedPod checks if the pod revision is updated of the workload
	IsUpdatedPod(reader client.Reader, obj client.Object, pod *corev1.Pod) (bool, error)
	// GetPodSelector gets the pod selector of the workload
	GetPodSelector(obj client.Object) (labels.Selector, error)
}
