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
	defaultRequeueAfter int32 = 5
	defaultRunTimeout   int32 = 5
)

// ExecutorContext context of rolloutRun
type ExecutorContext struct {
	rollout    *rolloutv1alpha1.Rollout
	rolloutRun *rolloutv1alpha1.RolloutRun
	newStatus  *rolloutv1alpha1.RolloutRunStatus
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

	// init
	rolloutRun := executorContext.rolloutRun
	newBatchStatus := executorContext.newStatus.BatchStatus
	if newBatchStatus == nil {
		if len(rolloutRun.Spec.Batch.Batches) > 0 {
			recordSize := len(executorContext.rolloutRun.Spec.Batch.Batches)
			newBatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
				Context: map[string]string{},
				RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
					CurrentBatchIndex: 0,
					CurrentBatchState: rolloutv1alpha1.BatchStepStatePending,
				},
				Records: make([]rolloutv1alpha1.RolloutRunBatchStatusRecord, recordSize),
			}
			executorContext.newStatus.BatchStatus = newBatchStatus
		} else {
			executorContext.newStatus.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
		}
	}

	currentBatchIndex := executorContext.newStatus.BatchStatus.CurrentBatchIndex
	preCurrentBatchState := executorContext.newStatus.BatchStatus.CurrentBatchState
	logger.Info("DefaultExecutor start to process", "currentBatchIndex", currentBatchIndex)
	defer func() {
		logger.Info(
			"DefaultExecutor process finished", "currentBatchIndex", currentBatchIndex,
			"stateFrom", preCurrentBatchState, "stateTo", executorContext.newStatus.BatchStatus.CurrentBatchState,
		)
	}()

	// if currentBatchError is not null, do nothing
	currentBatchError := executorContext.rolloutRun.Status.BatchStatus.CurrentBatchError
	if currentBatchError != nil {
		logger.Info(
			"DefaultExecutor will terminate", "currentBatchError", currentBatchError,
		)
		return false, ctrl.Result{}, nil
	}

	// lifecycle
	var (
		err    error
		done   bool
		result ctrl.Result
	)
	if len(rolloutRun.Spec.Batch.Batches) > 0 {
		switch newBatchStatus.CurrentBatchState {
		case rolloutv1alpha1.BatchStepStatePending:
			done, result = r.doInitialized(executorContext)
		case rolloutv1alpha1.BatchStepStatePreBatchStepHook:
			done, result, err = r.doPreBatchHook(ctx, executorContext)
		case rolloutv1alpha1.BatchStepStateRunning:
			done, result, err = r.doUpgrading(ctx, executorContext)
		case rolloutv1alpha1.BatchStepStatePostBatchStepHook:
			done, result, err = r.doPostBatchHook(ctx, executorContext)
		case rolloutv1alpha1.BatchStepStatePaused:
			done, result = r.doPaused(executorContext)
		case rolloutv1alpha1.BatchStepStateSucceeded:
			done = r.doSucceeded(executorContext)
		}
	}

	// default RequeueAfter
	// todo add backoff machinery
	if !done && err == nil && result.IsZero() {
		result = ctrl.Result{RequeueAfter: time.Duration(defaultRequeueAfter) * time.Second}
	}

	return isCompleted(executorContext), result, err
}

// isCompleted detect if rolloutRun is completed
func isCompleted(executorContext *ExecutorContext) bool {
	if len(executorContext.rolloutRun.Spec.Batch.Batches) == 0 {
		return true
	}
	newBatchStatus := executorContext.newStatus.BatchStatus
	if newBatchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStateSucceeded {
		return false
	}
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	return int(currentBatchIndex) >= (len(executorContext.rolloutRun.Spec.Batch.Batches) - 1)
}

// doInitialized process Initialized state
func (r *Executor) doInitialized(executorContext *ExecutorContext) (bool, ctrl.Result) {
	logger := r.Logger

	newBatchStatus := executorContext.newStatus.BatchStatus

	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	logger.Info(
		"DefaultExecutor begin to doInitialized", "currentBatchIndex", currentBatchIndex,
	)

	if newBatchStatus.Records[currentBatchIndex].IsZero() {
		newBatchStatus.Records[currentBatchIndex] = rolloutv1alpha1.RolloutRunBatchStatusRecord{}
		newBatchStatus.Records[currentBatchIndex].StartTime = &metav1.Time{Time: time.Now()}
	}

	newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePreBatchStepHook
	newBatchStatus.Records[currentBatchIndex].State = rolloutv1alpha1.BatchStepStatePreBatchStepHook

	return true, ctrl.Result{Requeue: true}
}

// doPaused process paused state
func (r *Executor) doPaused(executorContext *ExecutorContext) (bool, ctrl.Result) {
	logger := r.Logger

	currentBatchIndex := executorContext.newStatus.BatchStatus.CurrentBatchIndex
	logger.Info(
		"DefaultExecutor begin to doPaused", "currentBatchIndex", currentBatchIndex,
	)

	return false, ctrl.Result{Requeue: false}
}

// doSucceeded process succeeded state
func (r *Executor) doSucceeded(executorContext *ExecutorContext) bool {
	logger := r.Logger

	newBatchStatus := executorContext.newStatus.BatchStatus

	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	logger.Info(
		"DefaultExecutor begin to doSucceeded", "currentBatchIndex", currentBatchIndex,
	)

	if int(currentBatchIndex) < (len(executorContext.rolloutRun.Spec.Batch.Batches) - 1) {
		// move to next batch
		newBatchStatus.Context = map[string]string{}
		newBatchStatus.CurrentBatchIndex = currentBatchIndex + 1
		newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePending
	} else {
		// if all batch completed
		logger.Info("DefaultExecutor complete since all batches done")
	}

	return true
}
