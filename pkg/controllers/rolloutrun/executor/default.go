package executor

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
)

const (
	defaultRequeueAfter              int64 = 5
	defaultRunRolloutWebhooksTimeout       = time.Second * time.Duration(600)
)

// ExecutorContext context of rolloutRun
type ExecutorContext struct {
	rollout    *rolloutv1alpha1.Rollout
	rolloutRun *rolloutv1alpha1.RolloutRun
	newStatus  *rolloutv1alpha1.RolloutRunStatus
}

type Executor struct {
}

// Do execute the lifecycle for rollout run, and will return new status
func (r *Executor) Do(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	logger := logr.FromContext(ctx)

	// init
	newBatchStatus := executorContext.newStatus.BatchStatus
	if newBatchStatus == nil {
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
	}

	currentBatchIndex := executorContext.newStatus.BatchStatus.CurrentBatchIndex
	preCurrentBatchState := executorContext.newStatus.BatchStatus.CurrentBatchState
	logger.Info(fmt.Sprintf(
		"DefaultExecutor start to process, currentBatchIndex=%d", currentBatchIndex,
	))
	defer logger.Info(fmt.Sprintf(
		"DefaultExecutor process finished, currentBatchIndex=%d, status from(%s)->to(%s)",
		currentBatchIndex, preCurrentBatchState, executorContext.newStatus.BatchStatus.CurrentBatchState,
	))

	// if currentBatchError is not null, do nothing
	currentBatchError := executorContext.rolloutRun.Status.BatchStatus.CurrentBatchError
	if currentBatchError != nil {
		logger.Info(fmt.Sprintf(
			"DefaultExecutor will terminate when currentBatchError exist, error=%v", currentBatchError,
		))
		return false, ctrl.Result{}, nil
	}

	// lifecycle
	var (
		err    error
		done   bool
		result ctrl.Result
	)
	rolloutRun := executorContext.rolloutRun
	batchStatusRecord := newBatchStatus.Records[currentBatchIndex]
	switch newBatchStatus.CurrentBatchState {
	case rolloutv1alpha1.BatchStepStatePending:
		done, result = r.doInitialized(ctx, executorContext)
		if done {
			batchStatusRecord.State = rolloutv1alpha1.BatchStepStatePreBatchStepHook
			newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePreBatchStepHook
		}
	case rolloutv1alpha1.BatchStepStatePreBatchStepHook:
		done, result, err = r.doPreBatchHook(ctx, executorContext)
		if done {
			batchStatusRecord.State = rolloutv1alpha1.BatchStepStateRunning
			newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStateRunning
		}
	case rolloutv1alpha1.BatchStepStateRunning:
		done, result, err = r.doUpgrading(ctx, executorContext)
		if done {
			batchStatusRecord.State = rolloutv1alpha1.BatchStepStatePostBatchStepHook
			newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePostBatchStepHook
		}
	case rolloutv1alpha1.BatchStepStatePostBatchStepHook:
		done, result, err = r.doPostBatchHook(ctx, executorContext)
		if done {
			logger.Info(fmt.Sprintf(
				"DefaultExecutor will pause, currentBatchIndex=%d", currentBatchIndex,
			))
			pause := rolloutRun.Spec.Batch.Batches[newBatchStatus.CurrentBatchIndex].Pause
			if pause == nil || !*pause {
				batchStatusRecord.State = rolloutv1alpha1.BatchStepStatePaused
				newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePaused
			} else {
				if batchStatusRecord.FinishTime == nil {
					batchStatusRecord.FinishTime = &metav1.Time{Time: time.Now()}
				}
				batchStatusRecord.State = rolloutv1alpha1.BatchStepStateSucceeded
				newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStateSucceeded
			}
		}
	case rolloutv1alpha1.BatchStepStatePaused:
		done, result = r.doPaused(ctx, executorContext)
		if done {
			if batchStatusRecord.FinishTime == nil {
				batchStatusRecord.FinishTime = &metav1.Time{Time: time.Now()}
			}
			batchStatusRecord.State = rolloutv1alpha1.BatchStepStateSucceeded
			newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStateSucceeded
		}
	case rolloutv1alpha1.BatchStepStateSucceeded:
		done = r.doSucceeded(ctx, executorContext)
		if int(currentBatchIndex) < (len(executorContext.rolloutRun.Spec.Batch.Batches) - 1) {
			// move to next batch
			newBatchStatus.Context = map[string]string{}
			newBatchStatus.CurrentBatchIndex = currentBatchIndex + 1
			newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePending
		} else {
			// if all batch completed
			logger.Info(fmt.Sprintf(
				"DefaultExecutor complete since all bacthes done, currentBatchIndex=%d", currentBatchIndex,
			))
		}
	}

	batchStatusRecord.Targets = nil

	// default RequeueAfter
	if !done && err == nil && result.IsZero() {
		result = ctrl.Result{RequeueAfter: time.Duration(defaultRequeueAfter) * time.Second}
	}

	return isCompleted(executorContext), result, err
}

// isCompleted detect if rolloutRun is completed
func isCompleted(executorContext *ExecutorContext) bool {
	newBatchStatus := executorContext.newStatus.BatchStatus
	if newBatchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStateSucceeded {
		return false
	}
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	return int(currentBatchIndex) >= (len(executorContext.rolloutRun.Spec.Batch.Batches) - 1)
}

// doInitialized process Initialized state
func (r *Executor) doInitialized(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result) {
	logger := logr.FromContext(ctx)

	newBatchStatus := executorContext.newStatus.BatchStatus

	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	logger.Info(fmt.Sprintf(
		"DefaultExecutor begin to doInitialized, currentBatchIndex=%d", currentBatchIndex,
	))

	if reflect.ValueOf(newBatchStatus.Records[currentBatchIndex]).IsZero() {
		newBatchStatus.Records[currentBatchIndex] = rolloutv1alpha1.RolloutRunBatchStatusRecord{}
		newBatchStatus.Records[currentBatchIndex].StartTime = &metav1.Time{Time: time.Now()}
	}

	return true, ctrl.Result{Requeue: true}
}

// doPreBatchHook process PreBatchHook state
func (r *Executor) doPreBatchHook(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	logger := logr.FromContext(ctx)

	currentBatchIndex := executorContext.newStatus.BatchStatus.CurrentBatchIndex
	logger.Info(fmt.Sprintf(
		"DefaultExecutor begin to doPreBatchHook, currentBatchIndex=%d", currentBatchIndex,
	))

	var (
		done   bool
		err    error
		result ctrl.Result
	)
	task := func() {
		done, result, err = r.runRolloutWebhooks(ctx, rolloutv1alpha1.HookTypePreBatchStep, executorContext)
	}
	if timeoutErr := utils.RunWithTimeout(ctx, defaultRunRolloutWebhooksTimeout, task); timeoutErr != nil {
		if err == nil {
			err = timeoutErr
		}
		logger.Error(timeoutErr, "DefaultExecutor doPreBatchHook error since timeout")
	}
	return done, result, err
}

// todo add analysis logic
// doPostBatchHook process doPostBatchHook state
func (r *Executor) doPostBatchHook(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	logger := logr.FromContext(ctx)

	currentBatchIndex := executorContext.newStatus.BatchStatus.CurrentBatchIndex
	logger.Info(fmt.Sprintf(
		"DefaultExecutor begin to doPostBatchHook, currentBatchIndex=%d", currentBatchIndex,
	))

	var (
		done   bool
		err    error
		result ctrl.Result
	)
	task := func() {
		done, result, err = r.runRolloutWebhooks(ctx, rolloutv1alpha1.HookTypePostBatchStep, executorContext)
	}
	if timeoutErr := utils.RunWithTimeout(ctx, defaultRunRolloutWebhooksTimeout, task); timeoutErr != nil {
		if err == nil {
			err = timeoutErr
		}
		logger.Error(timeoutErr, "DefaultExecutor doPostBatchHook error since timeout")
	}
	return done, result, err
}

// doPaused process paused state
func (r *Executor) doPaused(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result) {
	logger := logr.FromContext(ctx)

	currentBatchIndex := executorContext.newStatus.BatchStatus.CurrentBatchIndex
	logger.Info(fmt.Sprintf(
		"DefaultExecutor begin to doPaused, currentBatchIndex=%d", currentBatchIndex,
	))

	return false, ctrl.Result{Requeue: false}
}

// doSucceeded process succeeded state
func (r *Executor) doSucceeded(ctx context.Context, executorContext *ExecutorContext) bool {
	logger := logr.FromContext(ctx)
	logger.Info(fmt.Sprintf("doSucceeded (%s)", executorContext.rolloutRun.Name))

	currentBatchIndex := executorContext.newStatus.BatchStatus.CurrentBatchIndex
	logger.Info(fmt.Sprintf(
		"DefaultExecutor begin to doSucceeded, currentBatchIndex=%d", currentBatchIndex,
	))

	return true
}
