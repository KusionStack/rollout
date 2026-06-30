package executor

import (
	rolloutapis "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	"kusionstack.io/rollout/pkg/workload"
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
			handleBatchStatusWhenSkipped(newStatus, len(rolloutRun.Spec.Batch.Batches), rolloutRun.Spec.Batch.Batches, ctx.Workloads, rolloutRun.Spec.Batch)
		}
	case rolloutapis.AnnoManualCommandCancel:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
	case rolloutapis.AnnoManualCommandForceSkipCurrentBatch:
		handleBatchStatusWhenSkipped(newStatus, len(rolloutRun.Spec.Batch.Batches), rolloutRun.Spec.Batch.Batches, ctx.Workloads, rolloutRun.Spec.Batch)
	}

	return ctrl.Result{Requeue: true}
}

func handleBatchStatusWhenSkipped(newStatus *rolloutv1alpha1.RolloutRunStatus, batchSize int, batches []rolloutv1alpha1.RolloutRunStep, workloads *workload.Set, batchStrategy *rolloutv1alpha1.RolloutRunBatchStrategy) {
	currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
	if newStatus.Error != nil {
		newStatus.Error = nil
	}

	// only skip when current batch is not the last batch
	if int(currentBatchIndex) < (batchSize - 1) {
		newStatus.BatchStatus.Records[currentBatchIndex].State = StepSkipped
		newStatus.BatchStatus.CurrentBatchIndex = currentBatchIndex + 1
		newStatus.BatchStatus.CurrentBatchState = StepNone

		// Calculate and accumulate skip toleration for each workload in current batch
		if workloads != nil && batchStrategy != nil {
			currentBatch := batches[currentBatchIndex]
			for _, target := range currentBatch.Targets {
				info := workloads.Get(target.Cluster, target.Name)
				if info == nil {
					continue
				}
				status := info.APIStatus()
				currentBatchExpectedReplicas, _ := workload.CalculateUpdatedReplicas(&status.Replicas, target.Replicas)
				gap := currentBatchExpectedReplicas - status.UpdatedAvailableReplicas
				if gap <= 0 {
					continue
				}

				// Accumulate toleration into the batch strategy spec
				updateSkipToleration(batchStrategy, target.CrossClusterObjectNameReference, gap)
			}
		}
	}
}

func updateSkipToleration(batchStrategy *rolloutv1alpha1.RolloutRunBatchStrategy, ref rolloutv1alpha1.CrossClusterObjectNameReference, gap int32) {
	for i := range batchStrategy.Tolerations {
		if batchStrategy.Tolerations[i].CrossClusterObjectNameReference == ref {
			batchStrategy.Tolerations[i].Toleration = gap
			return
		}
	}
	batchStrategy.Tolerations = append(batchStrategy.Tolerations, rolloutv1alpha1.RolloutRunTolerationTarget{
		CrossClusterObjectNameReference: ref,
		Toleration:                      gap,
	})
}
