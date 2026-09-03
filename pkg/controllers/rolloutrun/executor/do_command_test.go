package executor

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
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
		expectedCurrentBatchIndex int32
		expectedCurrentBatchState rolloutv1alpha1.RolloutStepState
		expectedPhase             rolloutv1alpha1.RolloutRunPhase
		expectedRecordState       rolloutv1alpha1.RolloutStepState
		expectedToleration        *int32
	}{
		{
			name:       "skip advances to next batch",
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
			workloads:                 newTestWorkloadSet("cluster-a", "test-a", 1, 100, 25),
			expectedCurrentBatchIndex: 1,
			expectedCurrentBatchState: rolloutv1alpha1.RolloutStepNone,
			expectedRecordState:       rolloutv1alpha1.RolloutStepSkipped,
			expectedToleration:        ptr.To[int32](5), // 30 - 25 = 5
		},
		{
			name:       "skip advances to next batch from middle",
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
			workloads:                 newTestWorkloadSet("cluster-a", "test-a", 1, 100, 52),
			expectedCurrentBatchIndex: 2,
			expectedCurrentBatchState: rolloutv1alpha1.RolloutStepNone,
			expectedRecordState:       rolloutv1alpha1.RolloutStepSkipped,
			expectedToleration:        ptr.To[int32](8), // 60 - 52 = 8
		},
		{
			name:       "skip last batch transitions to PostRollout phase",
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
			workloads:                 newTestWorkloadSet("cluster-a", "test-a", 1, 100, 93),
			expectedCurrentBatchIndex: 2, // unchanged - last batch does not advance index
			expectedCurrentBatchState: rolloutv1alpha1.RolloutStepNone,
			expectedPhase:             rolloutv1alpha1.RolloutRunPhasePostRollout,
			expectedRecordState:       rolloutv1alpha1.RolloutStepSkipped,
			expectedToleration:        ptr.To[int32](7), // 100 - 93 = 7
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

			handleBatchStatusWhenSkipped(newStatus, tt.batchSize, tt.batches, tt.workloads)

			if newStatus.BatchStatus.CurrentBatchIndex != tt.expectedCurrentBatchIndex {
				t.Errorf("CurrentBatchIndex = %d, want %d", newStatus.BatchStatus.CurrentBatchIndex, tt.expectedCurrentBatchIndex)
			}
			if newStatus.BatchStatus.CurrentBatchState != tt.expectedCurrentBatchState {
				t.Errorf("CurrentBatchState = %v, want %v", newStatus.BatchStatus.CurrentBatchState, tt.expectedCurrentBatchState)
			}
			if tt.expectedPhase != "" && newStatus.Phase != tt.expectedPhase {
				t.Errorf("Phase = %v, want %v", newStatus.Phase, tt.expectedPhase)
			}
			if tt.expectedRecordState != "" && newStatus.BatchStatus.Records[tt.batchIndex].State != tt.expectedRecordState {
				t.Errorf("Records[%d].State = %v, want %v", tt.batchIndex, newStatus.BatchStatus.Records[tt.batchIndex].State, tt.expectedRecordState)
			}
			if tt.expectedToleration != nil {
				if len(newStatus.BatchStatus.Tolerations) != 1 {
					t.Errorf("expected 1 toleration entry, got %d", len(newStatus.BatchStatus.Tolerations))
				} else if newStatus.BatchStatus.Tolerations[0].Toleration != *tt.expectedToleration {
					t.Errorf("Tolerations[0].Toleration = %d, want %d", newStatus.BatchStatus.Tolerations[0].Toleration, *tt.expectedToleration)
				}
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
