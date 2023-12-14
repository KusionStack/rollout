package executor

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
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
	Logger logr.Logger
}

func NewDefaultExecutor(logger logr.Logger) *Executor {
	return &Executor{Logger: logger}
}

// Do execute the lifecycle for rollout run, and will return new status
func (r *Executor) Do(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	logger := r.Logger

	// init BatchStatus
	rolloutRun := executorContext.RolloutRun
	newBatchStatus := executorContext.NewStatus.BatchStatus
	if newBatchStatus == nil {
		r.doInitBatchStatus(executorContext)
		newBatchStatus = executorContext.NewStatus.BatchStatus
	}

	currentBatchIndex := executorContext.NewStatus.BatchStatus.CurrentBatchIndex
	preCurrentBatchState := executorContext.NewStatus.BatchStatus.CurrentBatchState
	logger.Info("DefaultExecutor start to process", "currentBatchIndex", currentBatchIndex)
	defer func() {
		logger.Info(
			"DefaultExecutor process finished", "currentBatchIndex", currentBatchIndex,
			"stateFrom", preCurrentBatchState, "stateTo", executorContext.NewStatus.BatchStatus.CurrentBatchState,
		)
	}()

	if len(rolloutRun.Spec.Batch.Batches) == 0 {
		return true, ctrl.Result{}, nil
	}

	// if currentBatchError is not null, do nothing
	currentBatchError := newBatchStatus.CurrentBatchError
	if currentBatchError != nil {
		logger.Info("DefaultExecutor will terminate", "currentBatchError", currentBatchError)
		return false, ctrl.Result{}, nil
	}

	// lifecycle
	var (
		err    error
		result ctrl.Result
	)
	switch newBatchStatus.CurrentBatchState {
	case rolloutv1alpha1.BatchStepStatePending:
		result = r.doInitialized(executorContext)
	case rolloutv1alpha1.BatchStepStatePreBatchStepHook:
		result, err = r.doPreBatchHook(ctx, executorContext)
	case rolloutv1alpha1.BatchStepStateRunning:
		result, err = r.doUpgrading(ctx, executorContext)
	case rolloutv1alpha1.BatchStepStatePostBatchStepHook:
		result, err = r.doPostBatchHook(ctx, executorContext)
	case rolloutv1alpha1.BatchStepStatePaused:
		r.doPaused(executorContext)
	case rolloutv1alpha1.BatchStepStateSucceeded:
		result = r.doSucceeded(executorContext)
		if newBatchStatus.CurrentBatchState == rolloutv1alpha1.BatchStepStateSucceeded &&
			int(newBatchStatus.CurrentBatchIndex) >= (len(rolloutRun.Spec.Batch.Batches)-1) {
			return true, result, nil
		}
	}

	return false, result, err
}

// doInitBatchStatus init BatchStatus
func (r *Executor) doInitBatchStatus(executorContext *ExecutorContext) {
	rolloutRun := executorContext.RolloutRun
	if len(rolloutRun.Spec.Batch.Batches) > 0 {
		var records []rolloutv1alpha1.RolloutRunBatchStatusRecord
		recordSize := len(executorContext.RolloutRun.Spec.Batch.Batches)
		for i := 0; i < recordSize; i++ {
			records = append(
				records, rolloutv1alpha1.RolloutRunBatchStatusRecord{},
			)
		}
		newBatchStatus := &rolloutv1alpha1.RolloutRunBatchStatus{
			Records: records,
			Context: map[string]string{},
			RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
				CurrentBatchIndex: 0,
				CurrentBatchState: rolloutv1alpha1.BatchStepStatePending,
			},
		}
		executorContext.NewStatus.BatchStatus = newBatchStatus
	} else {
		executorContext.NewStatus.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
	}
}

// doInitialized process Initialized state
func (r *Executor) doInitialized(executorContext *ExecutorContext) ctrl.Result {
	logger := r.Logger

	newBatchStatus := executorContext.NewStatus.BatchStatus

	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	logger.Info(
		"DefaultExecutor begin to doInitialized", "currentBatchIndex", currentBatchIndex,
	)

	if executorContext.RolloutRun.Spec.Batch.Batches[currentBatchIndex].Breakpoint {
		logger.Info("DefaultExecutor will pause")
		newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePaused
		newBatchStatus.Records[currentBatchIndex].State = rolloutv1alpha1.BatchStepStatePaused
	} else {
		newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePreBatchStepHook
		if newBatchStatus.Records[currentBatchIndex].StartTime == nil {
			newBatchStatus.Records[currentBatchIndex].StartTime = &metav1.Time{Time: time.Now()}
		}
		newBatchStatus.Records[currentBatchIndex].State = rolloutv1alpha1.BatchStepStatePreBatchStepHook
	}

	return ctrl.Result{Requeue: true}
}

// doPaused process paused state
func (r *Executor) doPaused(executorContext *ExecutorContext) {
	logger := r.Logger

	currentBatchIndex := executorContext.NewStatus.BatchStatus.CurrentBatchIndex
	logger.Info(
		"DefaultExecutor begin to doPaused", "currentBatchIndex", currentBatchIndex,
	)
}

// doSucceeded process succeeded state
func (r *Executor) doSucceeded(executorContext *ExecutorContext) ctrl.Result {
	logger := r.Logger

	newBatchStatus := executorContext.NewStatus.BatchStatus

	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	logger.Info(
		"DefaultExecutor begin to doSucceeded", "currentBatchIndex", currentBatchIndex,
	)

	if int(currentBatchIndex) >= (len(executorContext.RolloutRun.Spec.Batch.Batches) - 1) {
		// if all batch completed
		logger.Info("DefaultExecutor complete since all batches done")
		return ctrl.Result{}
	} else {
		// move to next batch
		newBatchStatus.Context = map[string]string{}
		newBatchStatus.CurrentBatchIndex = currentBatchIndex + 1
		newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePending
		return ctrl.Result{Requeue: true}
	}
}
