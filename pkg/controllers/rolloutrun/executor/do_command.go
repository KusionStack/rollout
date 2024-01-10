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

	newStatus := executorContext.NewStatus
	newBatchStatus := executorContext.NewStatus.BatchStatus

	batchError := newBatchStatus.Error
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	currentBatchState := newBatchStatus.CurrentBatchState
	switch cmd {
	case rolloutapis.AnnoManualCommandResume:
		if newStatus.Phase == rolloutv1alpha1.RolloutRunPhasePaused {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
		}
		if currentBatchState == BatchStatePaused {
			newBatchStatus.CurrentBatchState = BatchStatePreBatchHook
			newBatchStatus.Records[currentBatchIndex].State = newBatchStatus.CurrentBatchState
		}
	case rolloutapis.AnnoManualCommandRetry:
		if batchError != nil {
			newBatchStatus.Error = nil
		}
	case rolloutapis.AnnoManualCommandPause:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePausing
	case rolloutapis.AnnoManualCommandCancel:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
	case rolloutapis.AnnoManualCommandSkip:
		if batchError != nil {
			newBatchStatus.Error = nil
			if int(currentBatchIndex) < (len(rolloutRun.Spec.Batch.Batches) - 1) {
				currentBatchIndex++
				newBatchStatus.CurrentBatchIndex = currentBatchIndex
				newBatchStatus.CurrentBatchState = BatchStateInitial
			} else {
				newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
			}
		}
	default:
		return ctrl.Result{}
	}

	delete(rolloutRun.Annotations, rolloutapis.AnnoManualCommandKey)

	return ctrl.Result{Requeue: true}
}
