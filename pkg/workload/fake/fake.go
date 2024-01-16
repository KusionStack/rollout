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

var GVK = schema.GroupVersionKind{
	Group:   "rollout.kusionstack.io",
	Version: "v1alpha1",
	Kind:    "FakeWorkload",
}

var _ workload.Interface = &fakeWorkload{}

type FakeWorkload interface {
	workload.Interface
	ChangeStatus(replicas, partition, ready int32) FakeWorkload
}

type fakeWorkload struct {
	workload.Info

	Replicas      int32
	Partition     int32
	ReadyReplicas int32
}

func New(cluster, namespace, name string) FakeWorkload {
	return &fakeWorkload{
		Info: workload.Info{
			Cluster:   cluster,
			Namespace: namespace,
			Name:      name,
			GVK:       GVK,
			Labels: map[string]string{
				"rollout.kusionstack.io/cluster": cluster,
			},
		},
		Replicas:      100,
		Partition:     0,
		ReadyReplicas: 0,
	}
}

// GetInfo implements workload.Interface.
func (w *fakeWorkload) GetInfo() workload.Info {
	return w.Info
}

func (w *fakeWorkload) GetStatus() rolloutv1alpha1.RolloutWorkloadStatus {
	return rolloutv1alpha1.RolloutWorkloadStatus{
		Cluster:            w.Info.Cluster,
		Name:               w.Info.Name,
		Generation:         1,
		ObservedGeneration: 1,
		RolloutReplicasSummary: rolloutv1alpha1.RolloutReplicasSummary{
			Replicas:                 w.Replicas,
			UpdatedReplicas:          w.ReadyReplicas,
			UpdatedReadyReplicas:     w.ReadyReplicas,
			UpdatedAvailableReplicas: w.ReadyReplicas,
		},
	}
}

// IsWaitingRollout implements workload.Interface.
func (w *fakeWorkload) IsWaitingRollout() bool {
	return w.Partition == 0
}

// UpdateOnConflict implements workload.Interface.
func (*fakeWorkload) UpdateOnConflict(ctx context.Context, modifyFunc func(obj client.Object) error) error {
	return nil
}

// UpgradePartition implements workload.Interface.
func (w *fakeWorkload) UpgradePartition(partition intstr.IntOrString) (bool, error) {
	partitionInt, _ := workload.CalculatePartitionReplicas(&w.Replicas, partition)
	if partitionInt > int(w.Replicas) {
		partitionInt = int(w.Replicas)
	}
	if partitionInt <= int(w.Partition) {
		// already updated
		return false, nil
	}
	w.Partition = int32(partitionInt)
	w.ReadyReplicas = int32(partitionInt)
	return true, nil
}

func (w *fakeWorkload) ChangeStatus(replicas, partition, ready int32) FakeWorkload {
	w.Replicas = replicas
	w.Partition = partition
	w.ReadyReplicas = ready
	return w
}
