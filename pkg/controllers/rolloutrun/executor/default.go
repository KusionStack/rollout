package executor

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutapis "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
)

const (
	defaultRunTimeout   int32 = 5
	defaultRequeueAfter       = time.Duration(5) * time.Second
)

// ExecutorContext context of rolloutRun
type ExecutorContext struct {
	Rollout    *rolloutv1alpha1.Rollout
	RolloutRun *rolloutv1alpha1.RolloutRun
	NewStatus  *rolloutv1alpha1.RolloutRunStatus
}

type Executor struct {
	logger logr.Logger
}

func NewDefaultExecutor(logger logr.Logger) *Executor {
	return &Executor{logger: logger}
}

// Do execute the progressing lifecycle for rollout run, and will return new status
func (r *Executor) Do(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	// init ProgressingStatus
	newProgressingStatus := executorContext.NewStatus.BatchStatus
	if newProgressingStatus == nil {
		newProgressingStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
			Context: make(map[string]string),
			State:   rolloutv1alpha1.RolloutProgressingStateInitial,
		}
		executorContext.NewStatus.BatchStatus = newProgressingStatus
	}

	preState := newProgressingStatus.State
	r.logger.Info("DefaultExecutor start to process")
	defer func() {
		r.logger.Info("DefaultExecutor process finished", "stateFrom", preState, "stateTo", newProgressingStatus.State)
	}()

	// if command exist, do command
	rolloutRun := executorContext.RolloutRun
	if _, exist := utils.GetMapValue(rolloutRun.Annotations, rolloutapis.AnnoManualCommandKey); exist {
		return false, r.doCommand(executorContext), nil
	}

	// if paused, do nothing
	if newProgressingStatus.State == rolloutv1alpha1.RolloutProgressingStatePaused {
		r.logger.Info("DefaultExecutor will terminate since paused")
		return false, ctrl.Result{}, nil
	}

	// if batchError exist, do nothing
	progressingError := newProgressingStatus.Error
	if progressingError != nil {
		r.logger.Info("DefaultExecutor will terminate since err exist", "batchError", progressingError)
		return false, ctrl.Result{}, nil
	}

	return r.lifecycle(ctx, executorContext)
}

// lifecycle process progressing flow
func (r *Executor) lifecycle(ctx context.Context, executorContext *ExecutorContext) (done bool, result ctrl.Result, err error) {
	newProgressingStatus := executorContext.NewStatus.BatchStatus
	switch newProgressingStatus.State {
	case rolloutv1alpha1.RolloutProgressingStateInitial:
		result = ctrl.Result{Requeue: true}
		newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStatePreRollout
	case rolloutv1alpha1.RolloutProgressingStatePaused:
		result = ctrl.Result{}
	case rolloutv1alpha1.RolloutProgressingStateCompleted:
		result = ctrl.Result{}
	case rolloutv1alpha1.RolloutProgressingStateCanceling:
		result = ctrl.Result{}
		newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStateCompleted
	case rolloutv1alpha1.RolloutProgressingStatePreRollout:
		result = ctrl.Result{Requeue: true}
		newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStateRolling
	case rolloutv1alpha1.RolloutProgressingStateRolling:
		result, err = r.doBatch(ctx, executorContext)
	case rolloutv1alpha1.RolloutProgressingStatePostRollout:
		done = true
		result = ctrl.Result{}
		newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStateCompleted
	}
	return done, result, err
}

// doBatch process batch one-by-one
func (r *Executor) doBatch(ctx context.Context, executorContext *ExecutorContext) (ctrl.Result, error) {
	// init BatchStatus
	newProgressingStatus := executorContext.NewStatus.BatchStatus
	if len(newProgressingStatus.RolloutBatchStatus.CurrentBatchState) == 0 {
		rolloutRun := executorContext.RolloutRun
		if len(rolloutRun.Spec.Batch.Batches) > 0 {
			recordSize := len(executorContext.RolloutRun.Spec.Batch.Batches)
			records := make([]rolloutv1alpha1.RolloutRunBatchStatusRecord, recordSize)
			newProgressingStatus.Records = records
			newBatchStatus := rolloutv1alpha1.RolloutBatchStatus{
				CurrentBatchIndex: 0, CurrentBatchState: BatchStateInitial,
			}
			newProgressingStatus.RolloutBatchStatus = newBatchStatus
		} else {
			newProgressingStatus.RolloutBatchStatus = rolloutv1alpha1.RolloutBatchStatus{}
		}
	}

	currentBatchIndex := newProgressingStatus.RolloutBatchStatus.CurrentBatchIndex
	preCurrentBatchState := newProgressingStatus.RolloutBatchStatus.CurrentBatchState
	r.logger.Info("DefaultExecutor start to doBatch", "currentBatchIndex", currentBatchIndex)
	defer func() {
		r.logger.Info(
			"DefaultExecutor doBatch finished", "currentBatchIndex", currentBatchIndex,
			"stateFrom", preCurrentBatchState, "stateTo", newProgressingStatus.RolloutBatchStatus.CurrentBatchState,
		)
	}()

	var (
		err    error
		result ctrl.Result
	)
	rolloutRun := executorContext.RolloutRun
	newBatchStatus := newProgressingStatus.RolloutBatchStatus
	if len(rolloutRun.Spec.Batch.Batches) == 0 {
		result = ctrl.Result{Requeue: true}
		r.logger.Info("DefaultExecutor doBatch fast done since batches empty")
		newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStatePostRollout
	} else {
		switch newBatchStatus.CurrentBatchState {
		case BatchStateInitial:
			result = r.doBatchInitial(executorContext)
		case BatchStatePaused:
			result = r.doBatchPaused(executorContext)
		case BatchStatePreBatchHook:
			result, err = r.doBatchPreBatchHook(ctx, executorContext)
		case BatchStateUpgrading:
			result, err = r.doBatchUpgrading(ctx, executorContext)
		case BatchStatePostBatchHook:
			result, err = r.doBatchPostBatchHook(ctx, executorContext)
		case BatchStateSucceeded:
			result = r.doBatchSucceeded(executorContext)
			if newBatchStatus.CurrentBatchState == BatchStateSucceeded &&
				int(newBatchStatus.CurrentBatchIndex) >= (len(rolloutRun.Spec.Batch.Batches)-1) {
				result = ctrl.Result{Requeue: true}
				newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStatePostRollout
			}
		}
	}
	return result, err
}

// doBatchInitial process Initialized sta--feature-gates=UseDefaultExecutor=truete
func (r *Executor) doBatchInitial(executorContext *ExecutorContext) ctrl.Result {
	newProgressingStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newProgressingStatus.CurrentBatchIndex
	r.logger.Info(
		"DefaultExecutor begin to doBatchInitial", "currentBatchIndex", currentBatchIndex,
	)

	if executorContext.RolloutRun.Spec.Batch.Batches[currentBatchIndex].Breakpoint {
		r.logger.Info("DefaultExecutor will pause since breakpoint exist")
		newProgressingStatus.CurrentBatchState = BatchStatePaused
		newProgressingStatus.Records[currentBatchIndex].State = newProgressingStatus.CurrentBatchState
	} else {
		newProgressingStatus.RolloutBatchStatus.CurrentBatchState = BatchStatePreBatchHook
		if newProgressingStatus.Records[currentBatchIndex].StartTime == nil {
			newProgressingStatus.Records[currentBatchIndex].StartTime = &metav1.Time{Time: time.Now()}
		}
		newProgressingStatus.Records[currentBatchIndex].State = newProgressingStatus.CurrentBatchState
	}

	return ctrl.Result{Requeue: true}
}

// doBatchSucceeded process succeeded state
func (r *Executor) doBatchSucceeded(executorContext *ExecutorContext) (result ctrl.Result) {
	newProgressingStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newProgressingStatus.CurrentBatchIndex
	r.logger.Info(
		"DefaultExecutor begin to doBatchSucceeded", "currentBatchIndex", currentBatchIndex,
	)

	if int(currentBatchIndex+1) >= len(executorContext.RolloutRun.Spec.Batch.Batches) {
		result = ctrl.Result{}
		r.logger.Info("DefaultExecutor doBatchSucceeded completed since all batches done")
	} else {
		r.logger.Info("DefaultExecutor doBatchSucceeded move to next batch")
		result = ctrl.Result{Requeue: true}
		newProgressingStatus.Context = map[string]string{}
		newProgressingStatus.RolloutBatchStatus = rolloutv1alpha1.RolloutBatchStatus{
			CurrentBatchIndex: currentBatchIndex + 1, CurrentBatchState: BatchStateInitial,
		}
	}

	return result
}

// doBatchPaused process paused state
func (r *Executor) doBatchPaused(executorContext *ExecutorContext) ctrl.Result { //nolint:unparam
	currentBatchIndex := executorContext.NewStatus.BatchStatus.CurrentBatchIndex
	r.logger.Info(
		"DefaultExecutor begin to doBatchPaused", "currentBatchIndex", currentBatchIndex,
	)

	return ctrl.Result{}
}
