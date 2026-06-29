package executor

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"

	"kusionstack.io/rollout/pkg/workload"
)

func TestHandleBatchStatusWhenSkipped(t *testing.T) {
	tests := []struct {
		name                      string
		batchIndex                int32
		batchSize                 int
		batches                   []rolloutv1alpha1.RolloutRunStep
		workloads                 *workload.Set
		existingToleration        []rolloutv1alpha1.RolloutRunTolerationTarget
		expectedToleration        []rolloutv1alpha1.RolloutRunTolerationTarget
		expectedCurrentBatchIndex int32
		expectedCurrentBatchState rolloutv1alpha1.RolloutStepState
	}{
		{
			name:       "skip records toleration for workload with deficit",
			batchIndex: 0,
			batchSize:  3,
			batches: []rolloutv1alpha1.RolloutRunStep{
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(30)),
				}},
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(60)),
				}},
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(100)),
				}},
			},
			workloads:          newTestWorkloadSet("cluster-a", "test-a", 1, 100, 25),
			existingToleration: nil,
			expectedToleration: []rolloutv1alpha1.RolloutRunTolerationTarget{
				{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-a"}, Toleration: 5}, // 30 - 25 = 5
			},
			expectedCurrentBatchIndex: 1,
			expectedCurrentBatchState: rolloutv1alpha1.RolloutStepNone,
		},
		{
			name:       "skip accumulates toleration when existing toleration present",
			batchIndex: 1,
			batchSize:  3,
			batches: []rolloutv1alpha1.RolloutRunStep{
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(30)),
				}},
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(60)),
				}},
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(100)),
				}},
			},
			workloads: newTestWorkloadSet("cluster-a", "test-a", 1, 100, 52),
			existingToleration: []rolloutv1alpha1.RolloutRunTolerationTarget{
				{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-a"}, Toleration: 5},
			},
			expectedToleration: []rolloutv1alpha1.RolloutRunTolerationTarget{
				{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-a"}, Toleration: 13}, // 5 + (60-52=8) = 13
			},
			expectedCurrentBatchIndex: 2,
			expectedCurrentBatchState: rolloutv1alpha1.RolloutStepNone,
		},
		{
			name:       "skip does not advance when last batch",
			batchIndex: 2,
			batchSize:  3,
			batches: []rolloutv1alpha1.RolloutRunStep{
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(30)),
				}},
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(60)),
				}},
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(100)),
				}},
			},
			workloads: newTestWorkloadSet("cluster-a", "test-a", 1, 100, 93),
			existingToleration: []rolloutv1alpha1.RolloutRunTolerationTarget{
				{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-a"}, Toleration: 5},
			},
			expectedToleration: []rolloutv1alpha1.RolloutRunTolerationTarget{
				{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-a"}, Toleration: 5},
			},
			expectedCurrentBatchIndex: 2, // unchanged
			expectedCurrentBatchState: rolloutv1alpha1.RolloutStepNone,
		},
		{
			name:       "skip with no deficit does not add toleration",
			batchIndex: 0,
			batchSize:  2,
			batches: []rolloutv1alpha1.RolloutRunStep{
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(30)),
				}},
				{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					newRunStepTarget("cluster-a", "test-a", intstr.FromInt(60)),
				}},
			},
			workloads:                 newTestWorkloadSet("cluster-a", "test-a", 1, 100, 35),
			existingToleration:        nil,
			expectedToleration:        nil, // no gap, no toleration added
			expectedCurrentBatchIndex: 1,
			expectedCurrentBatchState: rolloutv1alpha1.RolloutStepNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newStatus := &rolloutv1alpha1.RolloutRunStatus{
				BatchStatus: &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: tt.batchIndex,
					},
					Records: make([]rolloutv1alpha1.RolloutRunStepStatus, tt.batchSize),
				},
			}

			batchStrategy := &rolloutv1alpha1.RolloutRunBatchStrategy{
				Batches:     tt.batches,
				Tolerations: tt.existingToleration,
			}

			handleBatchStatusWhenSkipped(newStatus, tt.batchSize, tt.batches, tt.workloads, batchStrategy)

			if tt.expectedToleration == nil {
				if batchStrategy.Tolerations != nil {
					t.Errorf("expected nil Tolerations, got %v", batchStrategy.Tolerations)
				}
			} else {
				if len(batchStrategy.Tolerations) != len(tt.expectedToleration) {
					t.Errorf("expected %d Tolerations entries, got %d", len(tt.expectedToleration), len(batchStrategy.Tolerations))
				}
				for i, expected := range tt.expectedToleration {
					actual := batchStrategy.Tolerations[i]
					if actual.Cluster != expected.Cluster || actual.Name != expected.Name || actual.Toleration != expected.Toleration {
						t.Errorf("Tolerations[%d] = %+v, want %+v", i, actual, expected)
					}
				}
			}

			if newStatus.BatchStatus.CurrentBatchIndex != tt.expectedCurrentBatchIndex {
				t.Errorf("CurrentBatchIndex = %d, want %d", newStatus.BatchStatus.CurrentBatchIndex, tt.expectedCurrentBatchIndex)
			}
			if newStatus.BatchStatus.CurrentBatchState != tt.expectedCurrentBatchState {
				t.Errorf("CurrentBatchState = %v, want %v", newStatus.BatchStatus.CurrentBatchState, tt.expectedCurrentBatchState)
			}
		})
	}
}

func TestFindSkipToleration(t *testing.T) {
	tolerations := []rolloutv1alpha1.RolloutRunTolerationTarget{
		{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-a"}, Toleration: 5},
		{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-b", Name: "test-b"}, Toleration: 8},
	}

	tests := []struct {
		name        string
		tolerations []rolloutv1alpha1.RolloutRunTolerationTarget
		ref         rolloutv1alpha1.CrossClusterObjectNameReference
		expected    int32
	}{
		{
			name:        "found workload",
			tolerations: tolerations,
			ref:         rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-a"},
			expected:    5,
		},
		{
			name:        "not found workload",
			tolerations: tolerations,
			ref:         rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-c", Name: "test-c"},
			expected:    0,
		},
		{
			name:        "nil tolerations",
			tolerations: nil,
			ref:         rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-a"},
			expected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findSkipToleration(tt.tolerations, tt.ref)
			if result != tt.expected {
				t.Errorf("findSkipToleration() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// newTestWorkloadSet creates a workload.Set with a single workload for testing
func newTestWorkloadSet(cluster, name string, generation int64, desiredReplicas, updatedAvailableReplicas int32) *workload.Set {
	return workload.NewSet(&workload.Info{
		ClusterName: cluster,
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			Generation: generation,
		},
		Status: workload.InfoStatus{
			ObservedGeneration:       generation,
			DesiredReplicas:          desiredReplicas,
			UpdatedAvailableReplicas: updatedAvailableReplicas,
		},
	})
}
