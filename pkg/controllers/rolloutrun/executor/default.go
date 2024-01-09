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

// Do execute the lifecycle for rollout run, and will return new status
func (r *Executor) Do(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	// init BatchStatus
	newStatus := executorContext.NewStatus
	rolloutRun := executorContext.RolloutRun
	if len(newStatus.Phase) == 0 || newStatus.BatchStatus == nil {
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseInitial

		context := make(map[string]string)
		if len(rolloutRun.Spec.Batch.Batches) == 0 {
			newStatus.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{Context: context}
		} else {
			recordSize := len(rolloutRun.Spec.Batch.Batches)
			records := make([]rolloutv1alpha1.RolloutRunBatchStatusRecord, recordSize)
			for idx := range records {
				records[idx] = rolloutv1alpha1.RolloutRunBatchStatusRecord{State: BatchStateInitial}
			}
			newStatus.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{Records: records, Context: context}
		}
	}

	prePhase := newStatus.Phase
	newBatchStatus := newStatus.BatchStatus
	r.logger.Info("DefaultExecutor start to process")
	defer func() {
		r.logger.Info("DefaultExecutor process finished", "phaseFrom", prePhase, "phaseTo", newStatus.Phase)
	}()

	// if command exist, do command
	if _, exist := utils.GetMapValue(rolloutRun.Annotations, rolloutapis.AnnoManualCommandKey); exist {
		return false, r.doCommand(executorContext), nil
	}

	if !rolloutRun.DeletionTimestamp.IsZero() {
		r.logger.Info("DefaultExecutor will cancel since DeletionTimestamp is not nil")
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
		return false, ctrl.Result{Requeue: true}, nil
	}

	// if paused, do nothing
	if newStatus.Phase == rolloutv1alpha1.RolloutRunPhasePaused {
		r.logger.Info("DefaultExecutor will terminate since paused")
		return false, ctrl.Result{}, nil
	}

	// if batchError exist, do nothing
	progressingError := newBatchStatus.Error
	if progressingError != nil {
		r.logger.Info("DefaultExecutor will terminate since err exist", "batchError", progressingError)
		return false, ctrl.Result{}, nil
	}

	return r.lifecycle(ctx, executorContext)
}

// lifecycle
func (r *Executor) lifecycle(ctx context.Context, executorContext *ExecutorContext) (done bool, result ctrl.Result, err error) {
	newStatus := executorContext.NewStatus
	switch newStatus.Phase {
	case rolloutv1alpha1.RolloutRunPhaseInitial:
		result = ctrl.Result{Requeue: true}
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePreRollout
	case rolloutv1alpha1.RolloutRunPhasePaused:
		result = ctrl.Result{}
	case rolloutv1alpha1.RolloutRunPhaseSucceeded:
		done = true
		result = ctrl.Result{}
	case rolloutv1alpha1.RolloutRunPhaseCanceling:
		result = ctrl.Result{}
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceled
	case rolloutv1alpha1.RolloutRunPhasePreRollout:
		result = ctrl.Result{Requeue: true}
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseRolling
	case rolloutv1alpha1.RolloutRunPhaseRolling:
		result, err = r.doBatch(ctx, executorContext)
	case rolloutv1alpha1.RolloutRunPhasePostRollout:
		done = true
		result = ctrl.Result{}
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseSucceeded
	}
	return done, result, err
}

// doBatch process batch one-by-one
func (r *Executor) doBatch(ctx context.Context, executorContext *ExecutorContext) (ctrl.Result, error) {
	// todo resize records when rolloutRun.spec changed
	// https://github.com/KusionStack/rollout/issues/27

	// init BatchStatus
	newStatus := executorContext.NewStatus
	newBatchStatus := newStatus.BatchStatus
	if len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) == 0 {
		newBatchStatus.RolloutBatchStatus = rolloutv1alpha1.RolloutBatchStatus{CurrentBatchState: BatchStateInitial}
	}

	currentBatchStatus := newBatchStatus.RolloutBatchStatus
	currentBatchIndex := currentBatchStatus.CurrentBatchIndex
	preCurrentBatchState := currentBatchStatus.CurrentBatchState
	r.logger.Info("DefaultExecutor start to doBatch", "currentBatchIndex", currentBatchIndex)
	defer func() {
		r.logger.Info(
			"DefaultExecutor doBatch finished", "currentBatchIndex", currentBatchIndex,
			"stateFrom", preCurrentBatchState, "stateTo", newBatchStatus.RolloutBatchStatus.CurrentBatchState,
		)
	}()

	var (
		err    error
		result ctrl.Result
	)
	rolloutRun := executorContext.RolloutRun
	if len(rolloutRun.Spec.Batch.Batches) == 0 {
		result = ctrl.Result{Requeue: true}
		r.logger.Info("DefaultExecutor doBatch fast done since batches empty")
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
	}

	switch currentBatchStatus.CurrentBatchState {
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
		if currentBatchStatus.CurrentBatchState == BatchStateSucceeded &&
			int(currentBatchStatus.CurrentBatchIndex) >= (len(rolloutRun.Spec.Batch.Batches)-1) {
			result = ctrl.Result{Requeue: true}
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
		}
	}
	return result, err
}

// doBatchInitial process Initialized sta--feature-gates=UseDefaultExecutor=true
func (r *Executor) doBatchInitial(executorContext *ExecutorContext) ctrl.Result {
	newBatchStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	r.logger.Info(
		"DefaultExecutor begin to doBatchInitial", "currentBatchIndex", currentBatchIndex,
	)

	if executorContext.RolloutRun.Spec.Batch.Batches[currentBatchIndex].Breakpoint {
		r.logger.Info("DefaultExecutor will pause since breakpoint exist")
		newBatchStatus.CurrentBatchState = BatchStatePaused
		newBatchStatus.Records[currentBatchIndex].State = newBatchStatus.CurrentBatchState
	} else {
		newBatchStatus.RolloutBatchStatus.CurrentBatchState = BatchStatePreBatchHook
		if newBatchStatus.Records[currentBatchIndex].StartTime == nil {
			newBatchStatus.Records[currentBatchIndex].StartTime = &metav1.Time{Time: time.Now()}
		}
		newBatchStatus.Records[currentBatchIndex].State = newBatchStatus.CurrentBatchState
	}

	return ctrl.Result{Requeue: true}
}

// doBatchSucceeded process succeeded state
func (r *Executor) doBatchSucceeded(executorContext *ExecutorContext) (result ctrl.Result) {
	newBatchStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	r.logger.Info(
		"DefaultExecutor begin to doBatchSucceeded", "currentBatchIndex", currentBatchIndex,
	)

	if int(currentBatchIndex+1) >= len(executorContext.RolloutRun.Spec.Batch.Batches) {
		result = ctrl.Result{}
		r.logger.Info("DefaultExecutor doBatchSucceeded completed since all batches done")
	} else {
		r.logger.Info("DefaultExecutor doBatchSucceeded move to next batch")
		result = ctrl.Result{Requeue: true}
		newBatchStatus.Context = map[string]string{}
		newBatchStatus.RolloutBatchStatus = rolloutv1alpha1.RolloutBatchStatus{
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
