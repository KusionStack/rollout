package executor

import (
	rolloutapis "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// doCommand
func (r *Executor) doCommand(ctx *ExecutorContext) ctrl.Result {
	rolloutRun := ctx.RolloutRun
	cmd := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]
	logger := ctx.WithLogger(r.logger)
	logger.Info("processing manual command", "command", cmd)

	newStatus := ctx.NewStatus
	batchError := newStatus.Error
	switch cmd {
	case rolloutapis.AnnoManualCommandPause:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePausing
	case rolloutapis.AnnoManualCommandResume, rolloutapis.AnnoManualCommandContinue: // nolint
		if newStatus.Phase == rolloutv1alpha1.RolloutRunPhasePaused {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
		}
	case rolloutapis.AnnoManualCommandRetry:
		if batchError != nil {
			newStatus.Error = nil
		}
	case rolloutapis.AnnoManualCommandSkip:
		if batchError != nil {
			handleBatchStatusWhenSkipped(newStatus, len(rolloutRun.Spec.Batch.Batches))
		}
	case rolloutapis.AnnoManualCommandCancel:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
	case rolloutapis.AnnoManualCommandForceSkipCurrentBatch:
		handleBatchStatusWhenSkipped(newStatus, len(rolloutRun.Spec.Batch.Batches))
	}

	return ctrl.Result{Requeue: true}
}

func handleBatchStatusWhenSkipped(newStatus *rolloutv1alpha1.RolloutRunStatus, batchSize int) {
	currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
	if newStatus.Error != nil {
		newStatus.Error = nil
	}

	// only skip when current batch is not the last batch
	if int(currentBatchIndex) < (batchSize - 1) {
		newStatus.BatchStatus.Records[currentBatchIndex].State = StepSkipped
		newStatus.BatchStatus.CurrentBatchIndex = currentBatchIndex + 1
		newStatus.BatchStatus.CurrentBatchState = StepNone
	}
}
