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

func Test_constructRolloutRun_V2Path(t *testing.T) {
	tests := []struct {
		name       string
		obj        *rolloutv1alpha1.Rollout
		strategy   *rolloutv1alpha1.RolloutStrategy
		workloads  []*workload.Info
		rolloutId  string
		wantV2     bool // true if V2 strategy path expected
		wantCanary bool
		wantBatch  bool
	}{
		{
			name: "V2 batch strategy",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "test-rollout",
				},
				Spec: rolloutv1alpha1.RolloutSpec{
					StrategyRef: "test-strategy",
					WorkloadRef: rolloutv1alpha1.WorkloadRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
			},
			strategy: &rolloutv1alpha1.RolloutStrategy{
				BatchV2: &rolloutv1alpha1.BatchStrategyV2{
					Batches: []rolloutv1alpha1.RolloutBatchStrategyStep{
						{
							Targets: []rolloutv1alpha1.RolloutStrategyTargets{
								{Replicas: intstr.FromString("30%")},
							},
						},
						{
							Breakpoint: true,
							Targets: []rolloutv1alpha1.RolloutStrategyTargets{
								{Replicas: intstr.FromString("100%")},
							},
						},
					},
				},
			},
			workloads: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantV2:     true,
			wantCanary: false,
			wantBatch:  true,
		},
		{
			name: "V2 batch + canary strategy",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "test-rollout",
				},
				Spec: rolloutv1alpha1.RolloutSpec{
					StrategyRef: "test-strategy",
					WorkloadRef: rolloutv1alpha1.WorkloadRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
			},
			strategy: &rolloutv1alpha1.RolloutStrategy{
				CanaryV2: &rolloutv1alpha1.CanaryStrategyV2{
					Targets: []rolloutv1alpha1.RolloutStrategyTargets{
						{Replicas: intstr.FromString("5%")},
					},
				},
				BatchV2: &rolloutv1alpha1.BatchStrategyV2{
					Batches: []rolloutv1alpha1.RolloutBatchStrategyStep{
						{
							Targets: []rolloutv1alpha1.RolloutStrategyTargets{
								{Replicas: intstr.FromString("100%")},
							},
						},
					},
				},
			},
			workloads: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantV2:     true,
			wantCanary: true,
			wantBatch:  true,
		},
		{
			name: "V1 batch strategy",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "test-rollout",
				},
				Spec: rolloutv1alpha1.RolloutSpec{
					StrategyRef: "test-strategy",
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
						},
					},
				},
			},
			workloads: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantV2:     false,
			wantCanary: false,
			wantBatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := constructRolloutRun(tt.obj, tt.strategy, tt.workloads, tt.rolloutId)

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

			// Verify canary
			if tt.wantCanary {
				if run.Spec.Canary == nil {
					t.Errorf("run.Spec.Canary is nil, expected canary strategy")
				}
			}

			// Verify batch
			if tt.wantBatch {
				if run.Spec.Batch == nil {
					t.Errorf("run.Spec.Batch is nil, expected batch strategy")
				}
			}
		})
	}
}

func Test_constructRolloutRunCanaryV2(t *testing.T) {
	workloads := []*workload.Info{
		newTestInfo("cluster-a", "test", "test-1"),
		newTestInfo("cluster-b", "test", "test-2"),
	}

	tests := []struct {
		name     string
		strategy *rolloutv1alpha1.CanaryStrategyV2
		wantNil  bool
		wantLen  int
	}{
		{
			name:     "nil strategy",
			strategy: nil,
			wantNil:  true,
		},
		{
			name: "single target without match (match all)",
			strategy: &rolloutv1alpha1.CanaryStrategyV2{
				Targets: []rolloutv1alpha1.RolloutStrategyTargets{
					{Replicas: intstr.FromString("10%")},
				},
			},
			wantNil: false,
			wantLen: 2, // both workloads matched
		},
		{
			name: "target with match by name",
			strategy: &rolloutv1alpha1.CanaryStrategyV2{
				Targets: []rolloutv1alpha1.RolloutStrategyTargets{
					{
						Replicas: intstr.FromString("10%"),
						Match: &rolloutv1alpha1.ResourceMatch{
							Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
								{Cluster: "cluster-a", Name: "test-1"},
							},
						},
					},
				},
			},
			wantNil: false,
			wantLen: 1, // only cluster-a matched
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructRolloutRunCanaryV2(tt.strategy, workloads)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if len(got.Targets) != tt.wantLen {
				t.Errorf("got %d targets, want %d", len(got.Targets), tt.wantLen)
			}
		})
	}
}

func Test_constructRolloutRunBatchesV2(t *testing.T) {
	workloads := []*workload.Info{
		newTestInfo("cluster-a", "test", "test-1"),
		newTestInfo("cluster-b", "test", "test-2"),
	}

	tests := []struct {
		name     string
		strategy *rolloutv1alpha1.BatchStrategyV2
		wantNil  bool
		wantLen  int
	}{
		{
			name:     "nil strategy",
			strategy: nil,
			wantNil:  true,
		},
		{
			name: "two batches with targets",
			strategy: &rolloutv1alpha1.BatchStrategyV2{
				Batches: []rolloutv1alpha1.RolloutBatchStrategyStep{
					{
						Targets: []rolloutv1alpha1.RolloutStrategyTargets{
							{Replicas: intstr.FromString("30%")},
						},
					},
					{
						Breakpoint: true,
						Targets: []rolloutv1alpha1.RolloutStrategyTargets{
							{Replicas: intstr.FromString("100%")},
						},
					},
				},
			},
			wantNil: false,
			wantLen: 2,
		},
		{
			name: "batch with match filtering",
			strategy: &rolloutv1alpha1.BatchStrategyV2{
				Batches: []rolloutv1alpha1.RolloutBatchStrategyStep{
					{
						Targets: []rolloutv1alpha1.RolloutStrategyTargets{
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
			},
			wantNil: false,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructRolloutRunBatchesV2(tt.strategy, workloads)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d batches, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func Test_resolveRolloutTargets(t *testing.T) {
	workloads := []*workload.Info{
		newTestInfo("cluster-a", "test", "test-1"),
		newTestInfo("cluster-b", "test", "test-2"),
	}

	tests := []struct {
		name    string
		targets []rolloutv1alpha1.RolloutStrategyTargets
		wantLen int
	}{
		{
			name:    "empty targets",
			targets: nil,
			wantLen: 0,
		},
		{
			name: "match all workloads",
			targets: []rolloutv1alpha1.RolloutStrategyTargets{
				{Replicas: intstr.FromString("50%")},
			},
			wantLen: 2,
		},
		{
			name: "match specific cluster",
			targets: []rolloutv1alpha1.RolloutStrategyTargets{
				{
					Replicas: intstr.FromString("50%"),
					Match: &rolloutv1alpha1.ResourceMatch{
						Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
							{Cluster: "cluster-a", Name: "test-1"},
						},
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple targets filter to different workloads",
			targets: []rolloutv1alpha1.RolloutStrategyTargets{
				{
					Replicas: intstr.FromString("30%"),
					Match: &rolloutv1alpha1.ResourceMatch{
						Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
							{Cluster: "cluster-a", Name: "test-1"},
						},
					},
				},
				{
					Replicas: intstr.FromString("70%"),
					Match: &rolloutv1alpha1.ResourceMatch{
						Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
							{Cluster: "cluster-b", Name: "test-2"},
						},
					},
				},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveRolloutTargets(tt.targets, workloads)
			if len(got) != tt.wantLen {
				t.Errorf("got %d targets, want %d", len(got), tt.wantLen)
			}
		})
	}
}
