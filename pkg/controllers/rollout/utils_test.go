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

package rollout

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

func newTestInfo(cluster, namespace, name string) *workload.Info {
	return &workload.Info{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			ClusterName: cluster,
		},
	}
}

func Test_constructRolloutRunBatches(t *testing.T) {
	tests := []struct {
		name             string
		strategy         *rolloutv1alpha1.RolloutStrategy
		workloadWrappers []*workload.Info
		want             []rolloutv1alpha1.RolloutRunStep
	}{
		{
			name: "customize batch",
			strategy: &rolloutv1alpha1.RolloutStrategy{
				Batch: &rolloutv1alpha1.BatchStrategy{
					Batches: []rolloutv1alpha1.RolloutStep{
						{
							Breakpoint: true,
							Replicas:   intstr.FromString("10%"),
							Match: &rolloutv1alpha1.ResourceMatch{
								Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
									{
										Cluster: "cluster-a",
										Name:    "test-1",
									},
								},
							},
						},
						{
							Breakpoint:           true,
							Replicas:             intstr.FromString("50%"),
							ReplicaSlidingWindow: ptr.To(intstr.FromString("10%")),
							Match: &rolloutv1alpha1.ResourceMatch{
								Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
									{
										Cluster: "cluster-a",
										Name:    "test-1",
									},
									{
										Cluster: "cluster-b",
										Name:    "test-1",
									},
								},
							},
						},
						{
							Replicas: intstr.FromString("100%"),
							Match: &rolloutv1alpha1.ResourceMatch{
								Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
									{
										Cluster: "cluster-a",
										Name:    "test-1",
									},
								},
							},
						},
						{
							Replicas: intstr.FromString("100%"),
							Match: &rolloutv1alpha1.ResourceMatch{
								Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
									{
										Cluster: "cluster-b",
										Name:    "test-1",
									},
								},
							},
						},
					},
				},
			},
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
				newTestInfo("cluster-b", "test", "test-1"),
			},
			want: []rolloutv1alpha1.RolloutRunStep{
				{
					// beta step
					Breakpoint: true,
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-a",
								Name:    "test-1",
							},
							Replicas: intstr.FromString("10%"),
						},
					},
				},
				{
					// 1 step
					Breakpoint: true,
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-a",
								Name:    "test-1",
							},
							Replicas:             intstr.FromString("50%"),
							ReplicaSlidingWindow: ptr.To(intstr.FromString("10%")),
						},
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-b",
								Name:    "test-1",
							},
							Replicas:             intstr.FromString("50%"),
							ReplicaSlidingWindow: ptr.To(intstr.FromString("10%")),
						},
					},
				},
				{
					// 2 step
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-a",
								Name:    "test-1",
							},
							Replicas: intstr.FromString("100%"),
						},
					},
				},
				{
					// 2 step
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-b",
								Name:    "test-1",
							},
							Replicas: intstr.FromString("100%"),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := constructRolloutRunBatches(tt.strategy.Batch, tt.workloadWrappers); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("constructRolloutRunBatches() = %v, want %v", spew.Sdump(got), spew.Sdump(tt.want))
			}
		})
	}
}
