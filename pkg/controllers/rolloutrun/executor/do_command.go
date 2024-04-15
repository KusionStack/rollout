package executor

import (
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutapis "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

// doCommand
func (r *Executor) doCommand(ctx *ExecutorContext) ctrl.Result {
	rolloutRun := ctx.RolloutRun
	cmd := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]
	logger := ctx.loggerWithContext(r.logger)
	logger.Info("processing manual command", "command", cmd)

	newStatus := ctx.NewStatus
	newBatchStatus := ctx.NewStatus.BatchStatus

	batchError := newStatus.Error
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	switch cmd {
	case rolloutapis.AnnoManualCommandResume, rolloutapis.AnnoManualCommandContinue: // nolint
		if newStatus.Phase == rolloutv1alpha1.RolloutRunPhasePaused {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
		}
	case rolloutapis.AnnoManualCommandRetry:
		if batchError != nil {
			newStatus.Error = nil
		}
	case rolloutapis.AnnoManualCommandPause:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePausing
	case rolloutapis.AnnoManualCommandCancel:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
	case rolloutapis.AnnoManualCommandSkip:
		if batchError != nil {
			newStatus.Error = nil
			if int(currentBatchIndex) < (len(rolloutRun.Spec.Batch.Batches) - 1) {
				currentBatchIndex++
				newBatchStatus.CurrentBatchIndex = currentBatchIndex
				newBatchStatus.CurrentBatchState = StepNone
			} else {
				newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
			}
		}
	}

	return ctrl.Result{Requeue: true}
}
