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

package fake

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

var gvk = schema.GroupVersionKind{
	Group:   "rollout.kusionstack.io",
	Version: "v1alpha1",
	Kind:    "FakeWorkload",
}

var _ workload.Interface = &fakeWorkload{}

type fakeWorkload struct {
	workload.Info
}

func New(cluster, namespace, name string) workload.Interface {
	return &fakeWorkload{
		Info: workload.Info{
			Cluster:   cluster,
			Namespace: namespace,
			Name:      name,
			GVK:       gvk,
			Labels: map[string]string{
				"rollout.kusionstack.io/cluster": cluster,
			},
		},
	}
}

// GetInfo implements workload.Interface.
func (w *fakeWorkload) GetInfo() workload.Info {
	return w.Info
}

func (w *fakeWorkload) GetStatus() rolloutv1alpha1.RolloutWorkloadStatus {
	panic("unimplemented")
}

func (w *fakeWorkload) GetObj() client.Object {
	panic("unimplemented")
}

// CalculateAtLeastUpdatedAvailableReplicas implements workload.Interface.
func (*fakeWorkload) CalculateAtLeastUpdatedAvailableReplicas(failureThreshold *intstr.IntOrString) (int, error) {
	panic("unimplemented")
}

// CheckReady implements workload.Interface.
func (*fakeWorkload) CheckReady(expectUpdatedReplicas *int32) (bool, error) {
	panic("unimplemented")
}

// IsWaitingRollout implements workload.Interface.
func (*fakeWorkload) IsWaitingRollout() bool {
	panic("unimplemented")
}

// UpdateOnConflict implements workload.Interface.
func (*fakeWorkload) UpdateOnConflict(ctx context.Context, modifyFunc func(obj client.Object) error) error {
	panic("unimplemented")
}

// UpgradePartition implements workload.Interface.
func (*fakeWorkload) UpgradePartition(partition *intstr.IntOrString) (bool, error) {
	panic("unimplemented")
}
