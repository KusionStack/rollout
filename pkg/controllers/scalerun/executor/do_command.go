package executor

import (
	rolloutapis "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	rorexecutor "kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
)

// doCommand
func (r *Executor) doCommand(ctx *ExecutorContext) ctrl.Result {
	scaleRun := ctx.ScaleRun
	cmd := scaleRun.Annotations[rolloutapis.AnnoManualCommandKey]
	logger := ctx.WithLogger(r.logger)
	logger.Info("processing manual command", "command", cmd)

	newStatus := ctx.NewStatus
	newBatchStatus := ctx.NewStatus.Batches

	batchError := newStatus.Error
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
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
			newStatus.Error = nil

			if int(currentBatchIndex) < (len(scaleRun.Spec.Batch.Batches) - 1) {
				currentBatchIndex++
				newBatchStatus.CurrentBatchIndex = currentBatchIndex
				newBatchStatus.CurrentBatchState = rorexecutor.StepNone
			} else {
				newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
			}
		}
	case rolloutapis.AnnoManualCommandCancel:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
	case rolloutapis.AnnoManualCommandForceSkipCurrentBatch:
		if batchError != nil {
			newStatus.Error = nil
		}
		if int(currentBatchIndex) < (len(scaleRun.Spec.Batch.Batches) - 1) {
			currentBatchIndex++
			newBatchStatus.CurrentBatchIndex = currentBatchIndex
			newBatchStatus.CurrentBatchState = rorexecutor.StepNone
		} else {
			newBatchStatus.CurrentBatchState = rorexecutor.StepPostBatchStepHook
		}
	}

	return ctrl.Result{Requeue: true}
}
