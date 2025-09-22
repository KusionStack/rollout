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
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/utils/accessor"
)

// Accessor defines the functions to access the workload.
// The following interfaces are optional:
// - CanaryReleaseControl
// - BatchReleaseControl
// - PodControl
type Accessor interface {
	accessor.ObjectAccessor
	// DependentWorkloadGVKs returns the dependent workloadds' GroupVersionKinds
	DependentWorkloadGVKs() []schema.GroupVersionKind
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
	ScaleControl
	// CanaryPreCheck checks object before canary release.
	CanaryPreCheck(obj client.Object) error
	// ApplyCanaryPatch applies canary to the workload.
	ApplyCanaryPatch(canary client.Object, podTemplatePatch *v1alpha1.MetadataPatch) error
}

type ReplicaObjectControl interface {
	// RepliceType returns the type of replica object
	ReplicaType() schema.GroupVersionKind
	// RecognizeRevision checks if the replica object revision is crrent or updated of the workload
	RecognizeRevision(ctx context.Context, reader client.Reader, workload, object client.Object) (current, updated bool, err error)
	// GetReplicObjects gets the pod selector of the workload
	GetReplicObjects(ctx context.Context, reader client.Reader, workload client.Object) ([]client.Object, error)
}

// ScaleControl defines the control functions for workload scale
type ScaleControl interface {
	// Scale use replicas to update replicas of the workload.
	Scale(obj client.Object, replicas int32) error
}
