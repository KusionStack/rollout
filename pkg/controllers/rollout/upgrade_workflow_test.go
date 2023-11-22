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
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/fake"
)

func Test_getPausedBatches(t *testing.T) {
	type args struct {
		count      int32
		pausePoint []int32
		strategy   rolloutv1alpha1.PauseModeType
	}
	tests := []struct {
		name string
		args args
		want sets.Int32
	}{
		{
			name: "count == 0",
			args: args{
				count:      0,
				pausePoint: []int32{},
				strategy:   rolloutv1alpha1.PauseModeTypeFirstBatch,
			},
			want: sets.NewInt32(),
		},
		{
			name: "PauseModeTypeFirstBatch",
			args: args{
				count:      10,
				pausePoint: []int32{},
				strategy:   rolloutv1alpha1.PauseModeTypeFirstBatch,
			},
			want: sets.NewInt32(1),
		},
		{
			name: "PauseModeTypeFirstBatch with paused points",
			args: args{
				count:      10,
				pausePoint: []int32{1, 3, 5, 7, 9, 10, 11},
				strategy:   rolloutv1alpha1.PauseModeTypeFirstBatch,
			},
			want: sets.NewInt32(1, 3, 5, 7, 9),
		},
		{
			name: "PauseModeTypeEachBatch",
			args: args{
				count:      10,
				pausePoint: []int32{1, 3, 5, 7, 9, 10, 11},
				strategy:   rolloutv1alpha1.PauseModeTypeEachBatch,
			},
			want: sets.NewInt32(1, 2, 3, 4, 5, 6, 7, 8, 9),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPausedBatches(tt.args.count, tt.args.pausePoint, tt.args.strategy)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_convertFastBatch(t *testing.T) {
	tests := []struct {
		name      string
		fastBatch *rolloutv1alpha1.FastBatch
		want      []rolloutv1alpha1.RolloutStep
	}{
		{
			name: "1 batch with beta",
			fastBatch: &rolloutv1alpha1.FastBatch{
				Beta: &rolloutv1alpha1.FastBetaBatch{
					Replicas: intOrStringPtr(intstr.FromInt(1)),
				},
				Count: 1,
			},
			want: []rolloutv1alpha1.RolloutStep{
				{
					// beta batch
					Replicas: intstr.FromInt(1),
					Pause:    pointer.Bool(true),
				},
				{
					Replicas: intstr.FromString("100%"),
				},
			},
		},
		{
			name: "3 batches with beta and PauseModeTypeFirstBatch",
			fastBatch: &rolloutv1alpha1.FastBatch{
				Beta: &rolloutv1alpha1.FastBetaBatch{
					Replicas: intOrStringPtr(intstr.FromInt(1)),
				},
				Count:     3,
				PauseMode: rolloutv1alpha1.PauseModeTypeFirstBatch,
			},
			want: []rolloutv1alpha1.RolloutStep{
				{
					// beta batch
					Replicas: intstr.FromInt(1),
					Pause:    pointer.Bool(true),
				},
				{
					// batch index 1
					Replicas: intstr.FromString("33%"),
					Pause:    pointer.Bool(true),
				},
				{
					// batch index 2
					Replicas: intstr.FromString("66%"),
				},
				{
					// batch index 3
					Replicas: intstr.FromString("100%"),
				},
			},
		},
		{
			name: "3 batches with beta and PauseModeTypeEachBatch",
			fastBatch: &rolloutv1alpha1.FastBatch{
				Beta: &rolloutv1alpha1.FastBetaBatch{
					Replicas: intOrStringPtr(intstr.FromInt(1)),
				},
				Count:     3,
				PauseMode: rolloutv1alpha1.PauseModeTypeEachBatch,
			},
			want: []rolloutv1alpha1.RolloutStep{
				{
					// beta batch
					Replicas: intstr.FromInt(1),
					Pause:    pointer.Bool(true),
				},
				{
					// batch index 1
					Replicas: intstr.FromString("33%"),
					Pause:    pointer.Bool(true),
				},
				{
					// batch index 2
					Replicas: intstr.FromString("66%"),
					Pause:    pointer.Bool(true),
				},
				{
					// batch index 3
					Replicas: intstr.FromString("100%"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertFastBatch(tt.fastBatch)
			assert.Equal(t, tt.want, got)
		})
	}
}

func intOrStringPtr(is intstr.IntOrString) *intstr.IntOrString {
	return &is
}

func Test_constructRolloutRunBatches(t *testing.T) {
	tests := []struct {
		name             string
		strategy         *rolloutv1alpha1.RolloutStrategy
		workloadWrappers []workload.Interface
		want             []rolloutv1alpha1.RolloutRunStep
	}{
		{
			name: "fast batch",
			strategy: &rolloutv1alpha1.RolloutStrategy{
				Batch: &rolloutv1alpha1.BatchStrategy{
					FastBatch: &rolloutv1alpha1.FastBatch{
						Beta: &rolloutv1alpha1.FastBetaBatch{
							Replicas: intOrStringPtr(intstr.FromInt(1)),
						},
						Count:     2,
						PauseMode: rolloutv1alpha1.PauseModeTypeFirstBatch,
					},
				},
			},
			workloadWrappers: []workload.Interface{
				fake.New("cluster-a", "test", "test-1"),
				fake.New("cluster-b", "test", "test-1"),
			},
			want: []rolloutv1alpha1.RolloutRunStep{
				{
					// beta step
					Pause: pointer.Bool(true),
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-a",
								Name:    "test-1",
							},
							Replicas: intstr.FromInt(1),
						},
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-b",
								Name:    "test-1",
							},
							Replicas: intstr.FromInt(1),
						},
					},
				},
				{
					// 1 step
					Pause: pointer.Bool(true),
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-a",
								Name:    "test-1",
							},
							Replicas: intstr.FromString("50%"),
						},
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-b",
								Name:    "test-1",
							},
							Replicas: intstr.FromString("50%"),
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

		{
			name: "customize batch",
			strategy: &rolloutv1alpha1.RolloutStrategy{
				Batch: &rolloutv1alpha1.BatchStrategy{
					Batches: []rolloutv1alpha1.RolloutStep{
						{
							Pause:    pointer.Bool(true),
							Replicas: intstr.FromString("10%"),
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
							Pause:    pointer.Bool(true),
							Replicas: intstr.FromString("50%"),
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
			workloadWrappers: []workload.Interface{
				fake.New("cluster-a", "test", "test-1"),
				fake.New("cluster-b", "test", "test-1"),
			},
			want: []rolloutv1alpha1.RolloutRunStep{
				{
					// beta step
					Pause: pointer.Bool(true),
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
					Pause: pointer.Bool(true),
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-a",
								Name:    "test-1",
							},
							Replicas: intstr.FromString("50%"),
						},
						{
							CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
								Cluster: "cluster-b",
								Name:    "test-1",
							},
							Replicas: intstr.FromString("50%"),
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
