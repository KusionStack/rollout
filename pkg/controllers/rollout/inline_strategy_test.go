// Copyright 2025 The KusionStack Authors
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

func Test_constructRolloutRunFromInlineStrategy(t *testing.T) {
	tests := []struct {
		name             string
		obj              *rolloutv1alpha1.Rollout
		workloadWrappers []*workload.Info
		rolloutId        string
		wantRun          bool
		wantCanary       bool
		wantBatch        bool
	}{
		{
			name: "no inline strategy - return nil, false",
			obj: &rolloutv1alpha1.Rollout{
				Spec: rolloutv1alpha1.RolloutSpec{
					StrategyRef: "default-strategy",
					WorkloadRef: rolloutv1alpha1.WorkloadRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
			},
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId: "test-rollout-1",
			wantRun:   false,
		},
		{
			name: "only canary strategy - should return nil (batch strategy required)",
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
				},
			},
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantRun:    false, // Canary alone is not supported, need BatchStrategy
			wantCanary: false,
			wantBatch:  false,
		},
		{
			name: "only batch strategy",
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
								Breakpoint: true,
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
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantRun:    true,
			wantCanary: false,
			wantBatch:  true,
		},
		{
			name: "canary + batch strategy together",
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
								Replicas: intstr.FromString("5%"),
							},
						},
					},
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Breakpoint: true,
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
				},
			},
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
			},
			rolloutId:  "test-rollout-1",
			wantRun:    true,
			wantCanary: true,
			wantBatch:  true,
		},
		{
			name: "filter non-existent workloads",
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
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-b", // This workload doesn't exist
											Name:    "test-1",
										},
										Replicas: intstr.FromString("30%"),
									},
								},
							},
						},
					},
				},
			},
			workloadWrappers: []*workload.Info{
				newTestInfo("cluster-a", "test", "test-1"),
				// cluster-b/test-1 is not in workloads
			},
			rolloutId:  "test-rollout-1",
			wantRun:    true,
			wantCanary: false,
			wantBatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, gotInline := constructRolloutRunFromInlineStrategy(tt.obj, tt.workloadWrappers, tt.rolloutId)
			if gotInline != tt.wantRun {
				t.Errorf("constructRolloutRunFromInlineStrategy() gotInline = %v, want %v", gotInline, tt.wantRun)
				return
			}

			if !tt.wantRun {
				if run != nil {
					t.Errorf("constructRolloutRunFromInlineStrategy() expected nil run, got %v", run)
				}
				return
			}

			if run == nil {
				t.Errorf("constructRolloutRunFromInlineStrategy() expected non-nil run, got nil")
				return
			}

			// Verify basic fields
			if run.Name != tt.rolloutId {
				t.Errorf("run.Name = %v, want %v", run.Name, tt.rolloutId)
			}
			if run.Namespace != tt.obj.Namespace {
				t.Errorf("run.Namespace = %v, want %v", run.Namespace, tt.obj.Namespace)
			}

			// Verify Canary
			if tt.wantCanary {
				if run.Spec.Canary == nil {
					t.Errorf("run.Spec.Canary = nil, want non-nil")
				} else if len(run.Spec.Canary.Targets) == 0 {
					t.Errorf("run.Spec.Canary.Targets is empty")
				}
			}

			// Verify Batch
			if tt.wantBatch {
				if run.Spec.Batch == nil {
					t.Errorf("run.Spec.Batch = nil, want non-nil")
				} else if len(run.Spec.Batch.Batches) == 0 {
					t.Errorf("run.Spec.Batch.Batches is empty")
				}
			}

			// Verify owner reference
			if len(run.OwnerReferences) != 1 {
				t.Errorf("len(run.OwnerReferences) = %v, want 1", len(run.OwnerReferences))
			}
		})
	}
}

func Test_validateAndCopyBatchStrategy(t *testing.T) {
	tests := []struct {
		name        string
		batch       *rolloutv1alpha1.RolloutRunBatchStrategy
		workloadMap map[string]*workload.Info
		want        *rolloutv1alpha1.RolloutRunBatchStrategy
	}{
		{
			name:        "nil batch",
			batch:       nil,
			workloadMap: map[string]*workload.Info{},
			want:        nil,
		},
		{
			name:  "empty batches",
			batch: &rolloutv1alpha1.RolloutRunBatchStrategy{},
			workloadMap: map[string]*workload.Info{
				"cluster-a/test-1": newTestInfo("cluster-a", "test", "test-1"),
			},
			want: &rolloutv1alpha1.RolloutRunBatchStrategy{
				Batches: []rolloutv1alpha1.RolloutRunStep{},
			},
		},
		{
			name: "batch with targets - all exist",
			batch: &rolloutv1alpha1.RolloutRunBatchStrategy{
				Batches: []rolloutv1alpha1.RolloutRunStep{
					{
						Breakpoint: true,
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Cluster: "cluster-a",
									Name:    "test-1",
								},
								Replicas: intstr.FromString("20%"),
							},
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Cluster: "cluster-b",
									Name:    "test-1",
								},
								Replicas:             intstr.FromString("30%"),
								ReplicaSlidingWindow: ptr.To(intstr.FromString("10%")),
							},
						},
					},
				},
			},
			workloadMap: map[string]*workload.Info{
				"cluster-a/test-1": newTestInfo("cluster-a", "test", "test-1"),
				"cluster-b/test-1": newTestInfo("cluster-b", "test", "test-1"),
			},
			want: &rolloutv1alpha1.RolloutRunBatchStrategy{
				Batches: []rolloutv1alpha1.RolloutRunStep{
					{
						Breakpoint: true,
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Cluster: "cluster-a",
									Name:    "test-1",
								},
								Replicas: intstr.FromString("20%"),
							},
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Cluster: "cluster-b",
									Name:    "test-1",
								},
								Replicas:             intstr.FromString("30%"),
								ReplicaSlidingWindow: ptr.To(intstr.FromString("10%")),
							},
						},
					},
				},
			},
		},
		{
			name: "batch with targets - filter non-existent",
			batch: &rolloutv1alpha1.RolloutRunBatchStrategy{
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
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Cluster: "cluster-b", // Doesn't exist
									Name:    "test-1",
								},
								Replicas: intstr.FromString("30%"),
							},
						},
					},
				},
			},
			workloadMap: map[string]*workload.Info{
				"cluster-a/test-1": newTestInfo("cluster-a", "test", "test-1"),
				// cluster-b/test-1 is missing
			},
			want: &rolloutv1alpha1.RolloutRunBatchStrategy{
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
		{
			name: "batch with toleration preserved",
			batch: &rolloutv1alpha1.RolloutRunBatchStrategy{
				Toleration: &rolloutv1alpha1.TolerationStrategy{
					WorkloadFailureThreshold: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
				Batches: []rolloutv1alpha1.RolloutRunStep{
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Cluster: "cluster-a",
									Name:    "test-1",
								},
								Replicas: intstr.FromString("50%"),
							},
						},
					},
				},
			},
			workloadMap: map[string]*workload.Info{
				"cluster-a/test-1": newTestInfo("cluster-a", "test", "test-1"),
			},
			want: &rolloutv1alpha1.RolloutRunBatchStrategy{
				Toleration: &rolloutv1alpha1.TolerationStrategy{
					WorkloadFailureThreshold: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
				Batches: []rolloutv1alpha1.RolloutRunStep{
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Cluster: "cluster-a",
									Name:    "test-1",
								},
								Replicas: intstr.FromString("50%"),
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateAndCopyBatchStrategy(tt.batch, tt.workloadMap)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validateAndCopyBatchStrategy() = %v, want %v", spew.Sdump(got), spew.Sdump(tt.want))
			}
		})
	}
}

func Test_validateAndCopyCanaryStrategy(t *testing.T) {
	tests := []struct {
		name        string
		canary      *rolloutv1alpha1.RolloutRunCanaryStrategy
		workloadMap map[string]*workload.Info
		want        *rolloutv1alpha1.RolloutRunCanaryStrategy
	}{
		{
			name:        "nil canary",
			canary:      nil,
			workloadMap: map[string]*workload.Info{},
			want:        nil,
		},
		{
			name: "canary with all targets existing",
			canary: &rolloutv1alpha1.RolloutRunCanaryStrategy{
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
			workloadMap: map[string]*workload.Info{
				"cluster-a/test-1": newTestInfo("cluster-a", "test", "test-1"),
			},
			want: &rolloutv1alpha1.RolloutRunCanaryStrategy{
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
		},
		{
			name: "canary with filtered targets",
			canary: &rolloutv1alpha1.RolloutRunCanaryStrategy{
				Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Cluster: "cluster-a",
							Name:    "test-1",
						},
						Replicas: intstr.FromString("10%"),
					},
					{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Cluster: "cluster-b", // Doesn't exist
							Name:    "test-1",
						},
						Replicas: intstr.FromString("20%"),
					},
				},
			},
			workloadMap: map[string]*workload.Info{
				"cluster-a/test-1": newTestInfo("cluster-a", "test", "test-1"),
				// cluster-b/test-1 is missing
			},
			want: &rolloutv1alpha1.RolloutRunCanaryStrategy{
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
		},
		{
			name: "canary with all non-existent targets",
			canary: &rolloutv1alpha1.RolloutRunCanaryStrategy{
				Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Cluster: "cluster-b",
							Name:    "test-1",
						},
						Replicas: intstr.FromString("10%"),
					},
				},
			},
			workloadMap: map[string]*workload.Info{
				"cluster-a/test-1": newTestInfo("cluster-a", "test", "test-1"),
				// cluster-b/test-1 is missing
			},
			want: &rolloutv1alpha1.RolloutRunCanaryStrategy{
				Targets: []rolloutv1alpha1.RolloutRunStepTarget{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateAndCopyCanaryStrategy(tt.canary, tt.workloadMap)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validateAndCopyCanaryStrategy() = %v, want %v", spew.Sdump(got), spew.Sdump(tt.want))
			}
		})
	}
}

func Test_buildWorkloadMap(t *testing.T) {
	workloads := []*workload.Info{
		newTestInfo("cluster-a", "test", "test-1"),
		newTestInfo("cluster-b", "test", "test-2"),
	}

	m := buildWorkloadMap(workloads)

	if len(m) != 2 {
		t.Errorf("len(m) = %v, want 2", len(m))
	}

	if _, ok := m["cluster-a/test-1"]; !ok {
		t.Errorf("m['cluster-a/test-1'] not found")
	}

	if _, ok := m["cluster-b/test-2"]; !ok {
		t.Errorf("m['cluster-b/test-2'] not found")
	}

	if _, ok := m["cluster-a/test-2"]; ok {
		t.Errorf("m['cluster-a/test-2'] should not exist")
	}
}

func Test_workloadKey(t *testing.T) {
	tests := []struct {
		cluster string
		name    string
		want    string
	}{
		{"cluster-a", "test-1", "cluster-a/test-1"},
		{"cluster-b", "test-2", "cluster-b/test-2"},
		{"cluster-a", "", "cluster-a/"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := workloadKey(tt.cluster, tt.name)
			if got != tt.want {
				t.Errorf("workloadKey() = %v, want %v", got, tt.want)
			}
		})
	}
}
