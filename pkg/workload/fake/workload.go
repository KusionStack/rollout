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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/workload"
)

var GVK = schema.GroupVersionKind{
	Group:   "rollout.kusionstack.io",
	Version: "v1alpha1",
	Kind:    "FakeWorkload",
}

var _ workload.Interface = &workloadImpl{}

type FakeWorkload interface {
	workload.Interface
	ChangeStatus(replicas, partition, ready int32) FakeWorkload
}

type workloadImpl struct {
	workload.Info
	Partition int32
}

func New(cluster, namespace, name string) FakeWorkload {
	w := &workloadImpl{
		Info: workload.Info{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"rollout.kusionstack.io/cluster": cluster,
				},
				ClusterName: cluster,
				Generation:  1,
			},
			GroupVersionKind: GVK,
			Status: workload.Status{
				ObservedGeneration: 1,
			},
		},
	}
	w.ChangeStatus(100, 0, 0)
	return w
}

// GetInfo implements workload.Interface.
func (w *workloadImpl) GetInfo() workload.Info {
	return w.Info
}

// IsWaitingRollout implements workload.Interface.
func (w *workloadImpl) IsWaitingRollout() bool {
	return w.Partition == 0
}

// UpdateOnConflict implements workload.Interface.
func (*workloadImpl) UpdateOnConflict(ctx context.Context, modifyFunc func(obj client.Object) error) error {
	return nil
}

func (w *workloadImpl) ChangeStatus(replicas, partition, ready int32) FakeWorkload {
	w.Status.Replicas = replicas
	w.Partition = partition
	w.Status.UpdatedReplicas = ready
	w.Status.UpdatedReadyReplicas = ready
	w.Status.UpdatedAvailableReplicas = ready
	return w
}

func (w *workloadImpl) SetObject(_ client.Object) {
}
