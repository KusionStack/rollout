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
		ClusterName: cluster,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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

func Test_constructRolloutRun(t *testing.T) {
	tests := []struct {
		name             string
		obj              *rolloutv1alpha1.Rollout
		strategy         *rolloutv1alpha1.RolloutStrategy
		workloadWrappers []*workload.Info
		rolloutId        string
		wantInline       bool // true if should use inline strategy
	}{
		{
			name: "inline batch strategy takes precedence over StrategyRef",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "test-rollout",
				},
				Spec: rolloutv1alpha1.RolloutSpec{
					WorkloadRef: rolloutv1alpha1.WorkloadRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-a",
											Name:    "test-1",
										},
										Replicas: intstr.FromString("20%"),
									},
								},
							},
						},
					},
				},
			},
			strategy: &rolloutv1alpha1.RolloutStrategy{
				Batch: &rolloutv1alpha1.BatchStrategy{
					Batches: []rolloutv1alpha1.RolloutStep{
						{
							Replicas: intstr.FromString("50%"),
							Match: &rolloutv1alpha1.ResourceMatch{
								Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
									{Cluster: "cluster-a", Name: "test-1"},
								},
							},
						},
					},
				},
			},
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantInline: true,
		},
		{
			name: "inline canary strategy takes precedence over StrategyRef",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "test-rollout",
				},
				Spec: rolloutv1alpha1.RolloutSpec{
					WorkloadRef: rolloutv1alpha1.WorkloadRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					CanaryStrategy: &rolloutv1alpha1.RolloutRunCanaryStrategy{
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
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
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
						},
					},
				},
			},
			strategy: &rolloutv1alpha1.RolloutStrategy{
				Canary: &rolloutv1alpha1.CanaryStrategy{
					Replicas: intstr.FromString("20%"),
					Match: &rolloutv1alpha1.ResourceMatch{
						Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
							{Cluster: "cluster-a", Name: "test-1"},
						},
					},
				},
				Batch: &rolloutv1alpha1.BatchStrategy{
					Batches: []rolloutv1alpha1.RolloutStep{
						{
							Replicas: intstr.FromString("100%"),
							Match: &rolloutv1alpha1.ResourceMatch{
								Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
									{Cluster: "cluster-a", Name: "test-1"},
								},
							},
						},
					},
				},
			},
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantInline: true,
		},
		{
			name: "fallback to StrategyRef when no inline strategy",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "test-rollout",
				},
				Spec: rolloutv1alpha1.RolloutSpec{
					WorkloadRef: rolloutv1alpha1.WorkloadRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
			},
			strategy: &rolloutv1alpha1.RolloutStrategy{
				Batch: &rolloutv1alpha1.BatchStrategy{
					Batches: []rolloutv1alpha1.RolloutStep{
						{
							Replicas: intstr.FromString("50%"),
							Match: &rolloutv1alpha1.ResourceMatch{
								Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
									{Cluster: "cluster-a", Name: "test-1"},
								},
							},
						},
					},
				},
			},
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantInline: false,
		},
		{
			name: "inline batch with canary together",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "test-rollout",
				},
				Spec: rolloutv1alpha1.RolloutSpec{
					WorkloadRef: rolloutv1alpha1.WorkloadRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-a",
											Name:    "test-1",
										},
										Replicas: intstr.FromString("25%"),
									},
								},
							},
						},
					},
					CanaryStrategy: &rolloutv1alpha1.RolloutRunCanaryStrategy{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Cluster: "cluster-a",
									Name:    "test-1",
								},
								Replicas: intstr.FromString("5%"),
							},
						},
					},
				},
			},
			strategy: &rolloutv1alpha1.RolloutStrategy{
				Batch: &rolloutv1alpha1.BatchStrategy{
					Batches: []rolloutv1alpha1.RolloutStep{
						{
							Replicas: intstr.FromString("50%"),
							Match: &rolloutv1alpha1.ResourceMatch{
								Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
									{Cluster: "cluster-a", Name: "test-1"},
								},
							},
						},
					},
				},
			},
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantInline: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := constructRolloutRun(tt.obj, tt.strategy, tt.workloadWrappers, tt.rolloutId)

			if run == nil {
				t.Errorf("constructRolloutRun() returned nil")
				return
			}

			// Verify basic fields
			if run.Name != tt.rolloutId {
				t.Errorf("run.Name = %v, want %v", run.Name, tt.rolloutId)
			}
			if run.Namespace != tt.obj.Namespace {
				t.Errorf("run.Namespace = %v, want %v", run.Namespace, tt.obj.Namespace)
			}

			if tt.wantInline {
				// Verify that inline strategy was used (BatchSpec should be set based on inline config)
				if tt.obj.Spec.BatchStrategy != nil {
					if run.Spec.Batch == nil {
						t.Errorf("run.Spec.Batch is nil, expected inline batch strategy to be used")
						return
					}
					// Verify batch content matches inline config
					if len(run.Spec.Batch.Batches) != len(tt.obj.Spec.BatchStrategy.Batches) {
						t.Errorf("run.Spec.Batch.Batches length = %d, want %d",
							len(run.Spec.Batch.Batches), len(tt.obj.Spec.BatchStrategy.Batches))
					}
				}

				if tt.obj.Spec.CanaryStrategy != nil {
					if run.Spec.Canary == nil {
						t.Errorf("run.Spec.Canary is nil, expected inline canary strategy to be used")
						return
					}
					// Verify canary content matches inline config
					if len(run.Spec.Canary.Targets) != len(tt.obj.Spec.CanaryStrategy.Targets) {
						t.Errorf("run.Spec.Canary.Targets length = %d, want %d",
							len(run.Spec.Canary.Targets), len(tt.obj.Spec.CanaryStrategy.Targets))
					}
				}
			} else {
				// Verify that StrategyRef was used
				if run.Spec.Batch == nil {
					t.Errorf("run.Spec.Batch is nil, expected StrategyRef batch to be used")
					return
				}
				// Verify batch content matches strategy config
				if len(run.Spec.Batch.Batches) != len(tt.strategy.Batch.Batches) {
					t.Errorf("run.Spec.Batch.Batches length = %d, want %d",
						len(run.Spec.Batch.Batches), len(tt.strategy.Batch.Batches))
				}
			}

			// Verify owner reference exists
			if len(run.OwnerReferences) != 1 {
				t.Errorf("len(run.OwnerReferences) = %d, want 1", len(run.OwnerReferences))
			} else {
				owner := run.OwnerReferences[0]
				if owner.Kind != "Rollout" || owner.Name != tt.obj.Name {
					t.Errorf("ownerReference = {Kind:%s, Name:%s}, want {Kind:Rollout, Name:%s}",
						owner.Kind, owner.Name, tt.obj.Name)
				}
			}
		})
	}
}
