/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package executor

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"

	rorexecutor "kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
	"kusionstack.io/rollout/pkg/workload"
)

// TestHandleBatchStatusWhenSkipped exercises scalerun.handleBatchStatusWhenSkipped
// to cover manual-skip toleration recording (design scenario 5) plus the scale-down
// negative case (scenario 6). ScaleRun's toleration logic differs from RolloutRun
// in two key ways:
//   - gap uses info.Status.AvailableReplicas (NOT UpdatedAvailableReplicas)
//   - ScaleFrom/ScaleTo are sourced from Records[currentBatchIndex].Targets, with
//     fallback to info.Status.DesiredReplicas / target.Replicas when no entry yet.
//   - Toleration only applies for scale-up (ScaleFrom < ScaleTo); scale-down is
//     skipped entirely (no entry appended).
//   - Last batch advances Phase to PostRollout; non-last batch advances the
//     CurrentBatchIndex and resets CurrentBatchState to StepNone.
func TestHandleBatchStatusWhenSkipped(t *testing.T) {
	tests := []struct {
		name                      string
		batchIndex                int32
		batchSize                 int
		batches                   []rolloutv1alpha1.ScaleRunStep
		currentTargets            []rolloutv1alpha1.ScaleWorkloadStatus
		workloads                 *workload.Set
		expectedCurrentBatchIndex int32
		expectedCurrentBatchState rolloutv1alpha1.RolloutStepState
		expectedPhase             rolloutv1alpha1.RolloutRunPhase
		expectedRecordState       rolloutv1alpha1.RolloutStepState
		expectedToleration        *int32
		expectNoToleration        bool
	}{
		{
			// Scenario 5: manual skip on first batch (scale-up 10 -> 20).
			// available=15, so gap = 20 - 15 = 5, recorded as toleration.
			// Non-last batch: advance to index 1, state -> StepNone.
			name:       "skip advances to next batch (scale-up with toleration recorded)",
			batchIndex: 0,
			batchSize:  3,
			batches: []rolloutv1alpha1.ScaleRunStep{
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 20),
				}},
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 30),
				}},
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 40),
				}},
			},
			currentTargets: []rolloutv1alpha1.ScaleWorkloadStatus{
				{Cluster: "cluster-a", Name: "test-a", ScaleFrom: 10, ScaleTo: 20},
			},
			workloads:                 newTestScaleWorkloadSet("cluster-a", "test-a", 1, 10, 15),
			expectedCurrentBatchIndex: 1,
			expectedCurrentBatchState: rorexecutor.StepNone,
			expectedRecordState:       rorexecutor.StepSkipped,
			expectedToleration:        ptr.To[int32](5), // 20 - 15 = 5
		},
		{
			// Scenario 5 (middle): manual skip on middle batch (scale-up 20 -> 30).
			// available=22, so gap = 30 - 22 = 8, recorded as toleration.
			// Non-last batch: advance to index 2, state -> StepNone.
			name:       "skip advances to next batch from middle",
			batchIndex: 1,
			batchSize:  3,
			batches: []rolloutv1alpha1.ScaleRunStep{
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 20),
				}},
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 30),
				}},
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 40),
				}},
			},
			currentTargets: []rolloutv1alpha1.ScaleWorkloadStatus{
				{Cluster: "cluster-a", Name: "test-a", ScaleFrom: 20, ScaleTo: 30},
			},
			workloads:                 newTestScaleWorkloadSet("cluster-a", "test-a", 1, 20, 22),
			expectedCurrentBatchIndex: 2,
			expectedCurrentBatchState: rorexecutor.StepNone,
			expectedRecordState:       rorexecutor.StepSkipped,
			expectedToleration:        ptr.To[int32](8), // 30 - 22 = 8
		},
		{
			// Scenario 5 (last): manual skip on last batch (scale-up 30 -> 40).
			// available=33, so gap = 40 - 33 = 7, recorded as toleration.
			// Last batch: Phase -> PostRollout, CurrentBatchIndex unchanged.
			name:       "skip last batch transitions to PostRollout phase (scale-up)",
			batchIndex: 2,
			batchSize:  3,
			batches: []rolloutv1alpha1.ScaleRunStep{
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 20),
				}},
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 30),
				}},
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 40),
				}},
			},
			currentTargets: []rolloutv1alpha1.ScaleWorkloadStatus{
				{Cluster: "cluster-a", Name: "test-a", ScaleFrom: 30, ScaleTo: 40},
			},
			workloads:                 newTestScaleWorkloadSet("cluster-a", "test-a", 1, 30, 33),
			expectedCurrentBatchIndex: 2, // unchanged - last batch does not advance index
			expectedCurrentBatchState: rorexecutor.StepNone,
			expectedPhase:             rolloutv1alpha1.RolloutRunPhasePostRollout,
			expectedRecordState:       rorexecutor.StepSkipped,
			expectedToleration:        ptr.To[int32](7), // 40 - 33 = 7
		},
		{
			// Scenario 6: scale-down (ScaleFrom=10 >= ScaleTo=5).
			// recordScaleRunTolerations skips scale-down entirely, so no
			// toleration is appended. Still advances to next batch (index 1).
			name:       "scale-down does not record toleration",
			batchIndex: 0,
			batchSize:  2,
			batches: []rolloutv1alpha1.ScaleRunStep{
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 5),
				}},
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 10),
				}},
			},
			currentTargets: []rolloutv1alpha1.ScaleWorkloadStatus{
				{Cluster: "cluster-a", Name: "test-a", ScaleFrom: 10, ScaleTo: 5},
			},
			workloads:                 newTestScaleWorkloadSet("cluster-a", "test-a", 1, 10, 10),
			expectedCurrentBatchIndex: 1,
			expectedCurrentBatchState: rorexecutor.StepNone,
			expectedRecordState:       rorexecutor.StepSkipped,
			expectNoToleration:        true,
		},
		{
			// Fallback path: when Records[currentBatchIndex].Targets has no
			// matching entry, ScaleFrom falls back to info.Status.DesiredReplicas
			// and ScaleTo falls back to target.Replicas. Here DesiredReplicas=10
			// and target.Replicas=20 -> scale-up -> gap = 20 - 15 = 5.
			name:       "fallback to DesiredReplicas/Replicas when currentTargets empty",
			batchIndex: 0,
			batchSize:  2,
			batches: []rolloutv1alpha1.ScaleRunStep{
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 20),
				}},
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 30),
				}},
			},
			currentTargets:            nil, // no Targets recorded yet -> fallback path
			workloads:                 newTestScaleWorkloadSet("cluster-a", "test-a", 1, 10, 15),
			expectedCurrentBatchIndex: 1,
			expectedCurrentBatchState: rorexecutor.StepNone,
			expectedRecordState:       rorexecutor.StepSkipped,
			expectedToleration:        ptr.To[int32](5), // 20 - 15 = 5
		},
		{
			// Gap clamping: when available exceeds ScaleTo, gap is clamped to 0.
			// ScaleTo=20, Available=25 -> raw gap = -5 -> clamped to 0.
			name:       "gap clamped to zero when available exceeds ScaleTo",
			batchIndex: 0,
			batchSize:  2,
			batches: []rolloutv1alpha1.ScaleRunStep{
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 20),
				}},
				{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
					newScaleRunStepTarget("cluster-a", "test-a", 30),
				}},
			},
			currentTargets: []rolloutv1alpha1.ScaleWorkloadStatus{
				{Cluster: "cluster-a", Name: "test-a", ScaleFrom: 10, ScaleTo: 20},
			},
			workloads:                 newTestScaleWorkloadSet("cluster-a", "test-a", 1, 10, 25),
			expectedCurrentBatchIndex: 1,
			expectedCurrentBatchState: rorexecutor.StepNone,
			expectedRecordState:       rorexecutor.StepSkipped,
			expectedToleration:        ptr.To[int32](0), // 20 - 25 = -5 -> clamped to 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newStatus := &rolloutv1alpha1.ScaleRunStatus{
				Batches: &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: tt.batchIndex,
					},
					Records: make([]rolloutv1alpha1.ScaleRunStepStatus, tt.batchSize),
				},
			}
			// Inject the current targets for the active batch (used by
			// recordScaleRunTolerations to look up ScaleFrom/ScaleTo).
			newStatus.Batches.Records[tt.batchIndex].Targets = tt.currentTargets

			handleBatchStatusWhenSkipped(newStatus, tt.batchSize, tt.batches, tt.workloads)

			if newStatus.Batches.CurrentBatchIndex != tt.expectedCurrentBatchIndex {
				t.Errorf("CurrentBatchIndex = %d, want %d", newStatus.Batches.CurrentBatchIndex, tt.expectedCurrentBatchIndex)
			}
			if newStatus.Batches.CurrentBatchState != tt.expectedCurrentBatchState {
				t.Errorf("CurrentBatchState = %v, want %v", newStatus.Batches.CurrentBatchState, tt.expectedCurrentBatchState)
			}
			if tt.expectedPhase != "" && newStatus.Phase != tt.expectedPhase {
				t.Errorf("Phase = %v, want %v", newStatus.Phase, tt.expectedPhase)
			}
			if tt.expectedRecordState != "" && newStatus.Batches.Records[tt.batchIndex].State != tt.expectedRecordState {
				t.Errorf("Records[%d].State = %v, want %v", tt.batchIndex, newStatus.Batches.Records[tt.batchIndex].State, tt.expectedRecordState)
			}
			if tt.expectNoToleration {
				if len(newStatus.Batches.Tolerations) != 0 {
					t.Errorf("expected 0 toleration entries for scale-down, got %d: %v", len(newStatus.Batches.Tolerations), newStatus.Batches.Tolerations)
				}
			} else if tt.expectedToleration != nil {
				if len(newStatus.Batches.Tolerations) != 1 {
					t.Errorf("expected 1 toleration entry, got %d", len(newStatus.Batches.Tolerations))
				} else if newStatus.Batches.Tolerations[0].Toleration != *tt.expectedToleration {
					t.Errorf("Tolerations[0].Toleration = %d, want %d", newStatus.Batches.Tolerations[0].Toleration, *tt.expectedToleration)
				}
			}
		})
	}
}

// TestUpsertToleration_ScaleRun directly exercises scalerun.upsertToleration
// (shared with RolloutRun but living in the scalerun package namespace via
// re-declaration is not the case here; this function is package-local).
// It verifies:
//   - append when ref not present
//   - overwrite when ref already exists (no duplicate entries)
//   - multiple distinct refs each get their own entry
func TestUpsertToleration_ScaleRun(t *testing.T) {
	refA := rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-a"}
	refB := rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-b", Name: "test-b"}

	t.Run("appends new entry when ref absent", func(t *testing.T) {
		tolerations := []rolloutv1alpha1.RolloutRunTolerationTarget{}
		upsertToleration(&tolerations, refA, 5)
		if len(tolerations) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(tolerations))
		}
		if tolerations[0].Toleration != 5 {
			t.Errorf("Toleration = %d, want 5", tolerations[0].Toleration)
		}
		if tolerations[0].CrossClusterObjectNameReference != refA {
			t.Errorf("ref mismatch: got %+v, want %+v", tolerations[0].CrossClusterObjectNameReference, refA)
		}
	})

	t.Run("overwrites existing entry when ref present", func(t *testing.T) {
		tolerations := []rolloutv1alpha1.RolloutRunTolerationTarget{
			{CrossClusterObjectNameReference: refA, Toleration: 5},
		}
		upsertToleration(&tolerations, refA, 9)
		if len(tolerations) != 1 {
			t.Fatalf("expected 1 entry (no duplicates), got %d", len(tolerations))
		}
		if tolerations[0].Toleration != 9 {
			t.Errorf("Toleration = %d, want 9 (overwritten)", tolerations[0].Toleration)
		}
	})

	t.Run("multiple distinct refs each get their own entry", func(t *testing.T) {
		tolerations := []rolloutv1alpha1.RolloutRunTolerationTarget{
			{CrossClusterObjectNameReference: refA, Toleration: 5},
		}
		upsertToleration(&tolerations, refB, 3)
		if len(tolerations) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(tolerations))
		}
		// Update A again to ensure B is not disturbed and A is overwritten
		upsertToleration(&tolerations, refA, 7)
		if len(tolerations) != 2 {
			t.Fatalf("expected 2 entries (no dup), got %d", len(tolerations))
		}
		got := map[string]int32{}
		for _, tr := range tolerations {
			got[tr.Cluster+"/"+tr.Name] = tr.Toleration
		}
		if got["cluster-a/test-a"] != 7 {
			t.Errorf("cluster-a/test-a Toleration = %d, want 7", got["cluster-a/test-a"])
		}
		if got["cluster-b/test-b"] != 3 {
			t.Errorf("cluster-b/test-b Toleration = %d, want 3", got["cluster-b/test-b"])
		}
	})
}

// newTestScaleWorkloadSet creates a workload.Set with a single workload for ScaleRun tests.
// ScaleRun uses info.Status.AvailableReplicas (NOT UpdatedAvailableReplicas like RolloutRun),
// so the helper populates AvailableReplicas rather than UpdatedAvailableReplicas.
func newTestScaleWorkloadSet(cluster, name string, generation int64, desiredReplicas, availableReplicas int32) *workload.Set {
	return workload.NewSet(&workload.Info{
		ClusterName: cluster,
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			Generation: generation,
		},
		Status: workload.InfoStatus{
			ObservedGeneration: generation,
			DesiredReplicas:    desiredReplicas,
			AvailableReplicas:  availableReplicas,
		},
	})
}
