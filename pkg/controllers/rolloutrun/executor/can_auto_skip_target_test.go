package executor

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"

	"kusionstack.io/rollout/pkg/workload"
)

// TestCanAutoSkipTargetRolloutRun directly exercises batchExecutor.canAutoSkipTarget
// to cover the transient-state guards (problem #4) that are difficult to trigger
// through the full Reconcile loop:
//   - Generation mismatch
//   - TerminatingReplicas != 0
//   - Last-batch strict check (ObservedReplicas > DesiredReplicas)
//   - gap <= 0 while not ready
//   - Gap > FailureThreshold
//   - InitialDelay not yet elapsed
//   - nil Toleration / nil FailureThreshold
//   - Healthy deficit with delay elapsed -> skippable (auto-skip scenario 1)
func TestCanAutoSkipTargetRolloutRun(t *testing.T) {
	e := &batchExecutor{}

	now := ptr.To(metav1.Now())
	pastTime := ptr.To(metav1.Time{Time: time.Now().Add(-10 * time.Minute)})

	type args struct {
		item                         rolloutv1alpha1.RolloutRunStepTarget
		info                         *workload.Info
		currentBatchExpectedReplicas int32
		isLastBatch                  bool
		newStatus                    *rolloutv1alpha1.RolloutRunStatus
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "nil Toleration returns false",
			args: args{
				item:                         rolloutv1alpha1.RolloutRunStepTarget{Toleration: nil},
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(5).desired(10).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  false,
				newStatus:                    newRolloutRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "nil FailureThreshold returns false",
			args: args{
				item: rolloutv1alpha1.RolloutRunStepTarget{
					Toleration: &rolloutv1alpha1.RolloutStepTargetToleration{InitialDelaySeconds: ptr.To[int32](0)},
				},
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(5).desired(10).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  false,
				newStatus:                    newRolloutRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "Generation mismatch returns false (transient state, gap unreliable)",
			args: args{
				item:                         tolerationTarget(2, ptr.To[int32](0)),
				info:                         newInfoBuilder().generation(2).observedGen(1).updatedAvailable(5).desired(10).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  false,
				newStatus:                    newRolloutRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "TerminatingReplicas != 0 returns false (non-last batch, still converging)",
			args: args{
				item:                         tolerationTarget(5, ptr.To[int32](0)),
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(5).desired(10).terminating(3).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  false,
				newStatus:                    newRolloutRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "Last batch strict check: ObservedReplicas > DesiredReplicas returns false",
			args: args{
				item:                         tolerationTarget(5, ptr.To[int32](0)),
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(5).desired(10).observed(11).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  true,
				newStatus:                    newRolloutRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "gap <= 0 with healthy replicas returns false (already satisfied, no deficit to tolerate)",
			args: args{
				item:                         tolerationTarget(5, ptr.To[int32](0)),
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(10).desired(10).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  false,
				newStatus:                    newRolloutRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "gap > FailureThreshold returns false (scenario 2: keep waiting)",
			args: args{
				item:                         tolerationTarget(2, ptr.To[int32](0)),
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(7).desired(10).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  false,
				newStatus:                    newRolloutRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "gap == FailureThreshold and InitialDelay not yet elapsed returns false",
			args: args{
				item:                         tolerationTarget(5, ptr.To[int32](300)),
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(5).desired(10).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  false,
				newStatus:                    newRolloutRunStatusWithStart(0, now), // started now, 300s not elapsed
			},
			want: false,
		},
		{
			name: "gap == FailureThreshold and InitialDelay elapsed returns true (scenario 1: auto-skip middle batch)",
			args: args{
				item:                         tolerationTarget(5, ptr.To[int32](300)),
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(5).desired(10).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  false,
				newStatus:                    newRolloutRunStatusWithStart(0, pastTime), // started 10 min ago, 300s elapsed
			},
			want: true,
		},
		{
			name: "gap within threshold on last batch, no strict check violation, delay elapsed -> auto-skip last batch",
			args: args{
				item:                         tolerationTarget(5, ptr.To[int32](300)),
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(6).desired(10).observed(10).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  true,
				newStatus:                    newRolloutRunStatusWithStart(0, pastTime),
			},
			want: true,
		},
		{
			name: "last batch with TerminatingReplicas != 0 returns false (strict check blocks auto-skip)",
			args: args{
				item:                         tolerationTarget(5, ptr.To[int32](0)),
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(6).desired(10).observed(10).terminating(1).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  true,
				newStatus:                    newRolloutRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "InitialDelaySeconds nil treats delay as already elapsed",
			args: args{
				item:                         tolerationTarget(5, nil),
				info:                         newInfoBuilder().generation(1).observedGen(1).updatedAvailable(5).desired(10).build(),
				currentBatchExpectedReplicas: 10,
				isLastBatch:                  false,
				newStatus:                    newRolloutRunStatusWithStart(0, now),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.canAutoSkipTarget(tt.args.item, tt.args.info, tt.args.currentBatchExpectedReplicas, tt.args.isLastBatch, tt.args.newStatus)
			if got != tt.want {
				t.Errorf("canAutoSkipTarget() = %v, want %v", got, tt.want)
			}
		})
	}
}

// tolerationTarget builds a RolloutRunStepTarget with a Toleration.
// failureThreshold is required; initialDelay may be nil to indicate "no delay".
func tolerationTarget(failureThreshold int32, initialDelay *int32) rolloutv1alpha1.RolloutRunStepTarget {
	tol := &rolloutv1alpha1.RolloutStepTargetToleration{
		FailureThreshold: ptr.To[int32](failureThreshold),
	}
	if initialDelay != nil {
		tol.InitialDelaySeconds = ptr.To[int32](*initialDelay)
	}
	return rolloutv1alpha1.RolloutRunStepTarget{Toleration: tol}
}

func newRolloutRunStatusWithStart(currentBatchIndex int32, startTime *metav1.Time) *rolloutv1alpha1.RolloutRunStatus {
	records := []rolloutv1alpha1.RolloutRunStepStatus{
		{Index: ptr.To[int32](0), State: StepRunning, StartTime: startTime},
		{Index: ptr.To[int32](1), State: StepNone},
		{Index: ptr.To[int32](2), State: StepNone},
	}
	return &rolloutv1alpha1.RolloutRunStatus{
		BatchStatus: &rolloutv1alpha1.RolloutRunBatchStatus{
			RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
				CurrentBatchIndex: currentBatchIndex,
			},
			Records: records,
		},
	}
}

// infoBuilder is a fluent builder for workload.Info to keep test cases concise.
type infoBuilder struct {
	info *workload.Info
}

func newInfoBuilder() *infoBuilder {
	return &infoBuilder{info: &workload.Info{}}
}

func (b *infoBuilder) generation(g int64) *infoBuilder {
	b.info.Generation = g
	return b
}

func (b *infoBuilder) observedGen(g int64) *infoBuilder {
	b.info.Status.ObservedGeneration = g
	return b
}

func (b *infoBuilder) updatedAvailable(n int32) *infoBuilder {
	b.info.Status.UpdatedAvailableReplicas = n
	return b
}

func (b *infoBuilder) desired(n int32) *infoBuilder {
	b.info.Status.DesiredReplicas = n
	return b
}

func (b *infoBuilder) observed(n int32) *infoBuilder {
	b.info.Status.ObservedReplicas = n
	return b
}

func (b *infoBuilder) terminating(n int32) *infoBuilder {
	b.info.Status.TerminatingReplicas = n
	return b
}

func (b *infoBuilder) build() *workload.Info {
	return b.info
}
