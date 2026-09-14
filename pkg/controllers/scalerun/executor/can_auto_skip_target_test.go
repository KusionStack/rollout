package executor

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"

	rorexecutor "kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
	"kusionstack.io/rollout/pkg/workload"
)

// TestCanAutoSkipTargetScaleRun directly exercises scalerun.batchExecutor.canAutoSkipTarget
// to cover scale-up/scale-down and transient-state guards (problem #4, design scenarios 6 & 7):
//   - Scale-down (ScaleFrom >= ScaleTo) returns false (scenario 6: skip toleration entirely)
//   - Scale-up with Generation mismatch returns false
//   - Scale-up with gap <= 0 returns false
//   - Scale-up with gap > FailureThreshold returns false
//   - Scale-up with InitialDelay not elapsed returns false
//   - Scale-up with gap within threshold and delay elapsed returns true (scenario 7: auto-skip)
//   - Scale-up with nil Toleration / nil FailureThreshold / nil InitialDelaySeconds
func TestCanAutoSkipTargetScaleRun(t *testing.T) {
	e := &batchExecutor{}

	now := ptr.To(metav1.Now())
	pastTime := ptr.To(metav1.Time{Time: time.Now().Add(-10 * time.Minute)})

	type args struct {
		item       rolloutv1alpha1.ScaleRunStepTarget
		info       *workload.Info
		scaledFrom int32
		scaledTo   int32
		newStatus  *rolloutv1alpha1.ScaleRunStatus
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "scale-down (ScaleFrom >= ScaleTo) returns false (scenario 6: toleration skipped)",
			args: args{
				item:       scaleTolerationTarget(5, ptr.To[int32](0)),
				info:       newInfoBuilder().generation(1).observedGen(1).available(8).desired(10).build(),
				scaledFrom: 10,
				scaledTo:   5,
				newStatus:  newScaleRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "scale-up with nil Toleration returns false",
			args: args{
				item:       rolloutv1alpha1.ScaleRunStepTarget{Toleration: nil},
				info:       newInfoBuilder().generation(1).observedGen(1).available(15).desired(20).build(),
				scaledFrom: 10,
				scaledTo:   20,
				newStatus:  newScaleRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "scale-up with nil FailureThreshold returns false",
			args: args{
				item: rolloutv1alpha1.ScaleRunStepTarget{
					Toleration: &rolloutv1alpha1.RolloutStepTargetToleration{InitialDelaySeconds: ptr.To[int32](0)},
				},
				info:       newInfoBuilder().generation(1).observedGen(1).available(15).desired(20).build(),
				scaledFrom: 10,
				scaledTo:   20,
				newStatus:  newScaleRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "scale-up with Generation mismatch returns false",
			args: args{
				item:       scaleTolerationTarget(5, ptr.To[int32](0)),
				info:       newInfoBuilder().generation(2).observedGen(1).available(15).desired(20).build(),
				scaledFrom: 10,
				scaledTo:   20,
				newStatus:  newScaleRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "scale-up with gap <= 0 returns false (already satisfied)",
			args: args{
				item:       scaleTolerationTarget(5, ptr.To[int32](0)),
				info:       newInfoBuilder().generation(1).observedGen(1).available(20).desired(20).build(),
				scaledFrom: 10,
				scaledTo:   20,
				newStatus:  newScaleRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "scale-up with gap > FailureThreshold returns false",
			args: args{
				item:       scaleTolerationTarget(2, ptr.To[int32](0)),
				info:       newInfoBuilder().generation(1).observedGen(1).available(15).desired(20).build(),
				scaledFrom: 10,
				scaledTo:   20,
				newStatus:  newScaleRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "scale-up with InitialDelay not yet elapsed returns false",
			args: args{
				item:       scaleTolerationTarget(5, ptr.To[int32](300)),
				info:       newInfoBuilder().generation(1).observedGen(1).available(15).desired(20).build(),
				scaledFrom: 10,
				scaledTo:   20,
				newStatus:  newScaleRunStatusWithStart(0, now),
			},
			want: false,
		},
		{
			name: "scale-up with gap within threshold and delay elapsed returns true (scenario 7: auto-skip)",
			args: args{
				item:       scaleTolerationTarget(5, ptr.To[int32](300)),
				info:       newInfoBuilder().generation(1).observedGen(1).available(15).desired(20).build(),
				scaledFrom: 10,
				scaledTo:   20,
				newStatus:  newScaleRunStatusWithStart(0, pastTime),
			},
			want: true,
		},
		{
			name: "scale-up with InitialDelaySeconds nil treats delay as already elapsed",
			args: args{
				item:       scaleTolerationTarget(5, nil),
				info:       newInfoBuilder().generation(1).observedGen(1).available(18).desired(20).build(),
				scaledFrom: 10,
				scaledTo:   20,
				newStatus:  newScaleRunStatusWithStart(0, now),
			},
			want: true,
		},
		{
			name: "scale-up with equal ScaleFrom and ScaleTo returns false (degenerate, not scale-up)",
			args: args{
				item:       scaleTolerationTarget(5, ptr.To[int32](0)),
				info:       newInfoBuilder().generation(1).observedGen(1).available(10).desired(10).build(),
				scaledFrom: 10,
				scaledTo:   10,
				newStatus:  newScaleRunStatusWithStart(0, now),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.canAutoSkipTarget(tt.args.item, tt.args.info, tt.args.scaledFrom, tt.args.scaledTo, tt.args.newStatus)
			if got != tt.want {
				t.Errorf("canAutoSkipTarget() = %v, want %v", got, tt.want)
			}
		})
	}
}

func scaleTolerationTarget(failureThreshold int32, initialDelay *int32) rolloutv1alpha1.ScaleRunStepTarget {
	tol := &rolloutv1alpha1.RolloutStepTargetToleration{
		FailureThreshold: ptr.To[int32](failureThreshold),
	}
	if initialDelay != nil {
		tol.InitialDelaySeconds = ptr.To[int32](*initialDelay)
	}
	return rolloutv1alpha1.ScaleRunStepTarget{Toleration: tol}
}

func newScaleRunStatusWithStart(currentBatchIndex int32, startTime *metav1.Time) *rolloutv1alpha1.ScaleRunStatus {
	records := []rolloutv1alpha1.ScaleRunStepStatus{
		{Index: ptr.To[int32](0), State: rorexecutor.StepRunning, StartTime: startTime},
		{Index: ptr.To[int32](1), State: rorexecutor.StepNone},
		{Index: ptr.To[int32](2), State: rorexecutor.StepNone},
	}
	return &rolloutv1alpha1.ScaleRunStatus{
		Batches: &rolloutv1alpha1.ScaleRunBatchStatus{
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

func (b *infoBuilder) available(n int32) *infoBuilder {
	b.info.Status.AvailableReplicas = n
	return b
}

func (b *infoBuilder) desired(n int32) *infoBuilder {
	b.info.Status.DesiredReplicas = n
	return b
}

func (b *infoBuilder) build() *workload.Info {
	return b.info
}
