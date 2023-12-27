package executor

import (
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutapis "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

// doCommand
func (r *Executor) doCommand(executorContext *ExecutorContext) ctrl.Result {
	rolloutRun := executorContext.RolloutRun
	cmd := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]
	r.logger.Info("DefaultExecutor begin to doCommand", "command", cmd)

	newProgressingStatus := executorContext.NewStatus.BatchStatus

	batchError := newProgressingStatus.Error
	currentBatchIndex := newProgressingStatus.CurrentBatchIndex
	currentBatchState := newProgressingStatus.CurrentBatchState
	switch cmd {
	case rolloutapis.AnnoManualCommandResume:
		if newProgressingStatus.State == rolloutv1alpha1.RolloutProgressingStatePaused {
			newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStateRolling
		}
		if currentBatchState == BatchStatePaused {
			newProgressingStatus.CurrentBatchState = BatchStatePreBatchHook
			newProgressingStatus.Records[currentBatchIndex].State = newProgressingStatus.CurrentBatchState
		}
	case rolloutapis.AnnoManualCommandRetry:
		if batchError != nil {
			newProgressingStatus.Error = nil
		}
	case rolloutapis.AnnoManualCommandPause:
		newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStatePaused
	case rolloutapis.AnnoManualCommandCancel:
		newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStateCanceling
	case rolloutapis.AnnoManualCommandSkip:
		if batchError != nil {
			newProgressingStatus.Error = nil
			if int(currentBatchIndex) < (len(rolloutRun.Spec.Batch.Batches) - 1) {
				currentBatchIndex++
				newProgressingStatus.CurrentBatchIndex = currentBatchIndex
				newProgressingStatus.CurrentBatchState = BatchStateInitial
			} else {
				newProgressingStatus.State = rolloutv1alpha1.RolloutProgressingStatePostRollout
			}
		}
	default:
		return ctrl.Result{}
	}

	delete(rolloutRun.Annotations, rolloutapis.AnnoManualCommandKey)

	return ctrl.Result{Requeue: true}
}
