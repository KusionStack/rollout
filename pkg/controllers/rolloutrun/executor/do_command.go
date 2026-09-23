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
			handleBatchStatusWhenSkipped(newStatus, len(rolloutRun.Spec.Batch.Batches), rolloutRun.Spec.Batch.Batches, ctx.Workloads)
		}
	case rolloutapis.AnnoManualCommandCancel:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
	case rolloutapis.AnnoManualCommandForceSkipCurrentBatch:
		handleBatchStatusWhenSkipped(newStatus, len(rolloutRun.Spec.Batch.Batches), rolloutRun.Spec.Batch.Batches, ctx.Workloads)
	}

	return ctrl.Result{Requeue: true}
}

func handleBatchStatusWhenSkipped(newStatus *rolloutv1alpha1.RolloutRunStatus, batchSize int, batches []rolloutv1alpha1.RolloutRunStep, workloads *workload.Set) {
	currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
	if newStatus.Error != nil {
		newStatus.Error = nil
	}

	newStatus.BatchStatus.Records[currentBatchIndex].State = StepSkipped

	// Record tolerations for each workload in the current batch
	recordRolloutRunTolerations(&newStatus.BatchStatus.Tolerations, batches, workloads, currentBatchIndex)

	if int(currentBatchIndex) >= (batchSize - 1) {
		// Last batch: advance to PostRollout phase (will transition to Succeeded)
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
		return
	}

	// Not the last batch: advance to the next batch
	newStatus.BatchStatus.CurrentBatchIndex = currentBatchIndex + 1
	newStatus.BatchStatus.CurrentBatchState = StepNone
}

// recordRolloutRunTolerations upserts tolerations for each workload in the current batch.
// For each target, it calculates the gap between expected and actual updated available replicas,
// and updates (or inserts) the toleration value into the tolerations slice.
func recordRolloutRunTolerations(tolerations *[]rolloutv1alpha1.RolloutRunTolerationTarget, batches []rolloutv1alpha1.RolloutRunStep, workloads *workload.Set, currentBatchIndex int32) {
	if workloads == nil || int(currentBatchIndex) >= len(batches) {
		return
	}
	currentBatch := batches[currentBatchIndex]
	for _, target := range currentBatch.Targets {
		info := workloads.Get(target.Cluster, target.Name)
		if info == nil {
			continue
		}
		status := info.APIStatus()
		currentBatchExpectedReplicas, _ := workload.CalculateUpdatedReplicas(&status.Replicas, target.Replicas)
		gap := currentBatchExpectedReplicas - status.UpdatedAvailableReplicas
		if gap < 0 {
			gap = 0
		}
		upsertToleration(tolerations, target.CrossClusterObjectNameReference, gap)
	}
}

// upsertToleration updates the toleration value for the given workload reference,
// or appends a new entry if the workload is not yet present.
func upsertToleration(tolerations *[]rolloutv1alpha1.RolloutRunTolerationTarget, ref rolloutv1alpha1.CrossClusterObjectNameReference, gap int32) {
	for i, t := range *tolerations {
		if t.CrossClusterObjectNameReference == ref {
			(*tolerations)[i].Toleration = gap
			return
		}
	}
	*tolerations = append(*tolerations, rolloutv1alpha1.RolloutRunTolerationTarget{
		CrossClusterObjectNameReference: ref,
		Toleration:                      gap,
	})
}
