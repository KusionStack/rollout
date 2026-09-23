package executor

import (
	rolloutapis "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	rorexecutor "kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
	"kusionstack.io/rollout/pkg/workload"
)

// doCommand
func (r *Executor) doCommand(ctx *ExecutorContext) ctrl.Result {
	scaleRun := ctx.ScaleRun
	cmd := scaleRun.Annotations[rolloutapis.AnnoManualCommandKey]
	logger := ctx.WithLogger(r.logger)
	logger.Info("processing manual command", "command", cmd)

	newStatus := ctx.NewStatus

	switch cmd {
	case rolloutapis.AnnoManualCommandPause:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePausing
	case rolloutapis.AnnoManualCommandResume, rolloutapis.AnnoManualCommandContinue: // nolint
		if newStatus.Phase == rolloutv1alpha1.RolloutRunPhasePaused {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
		}
	case rolloutapis.AnnoManualCommandRetry:
		if newStatus.Error != nil {
			newStatus.Error = nil
		}
	case rolloutapis.AnnoManualCommandSkip:
		if newStatus.Error != nil {
			handleBatchStatusWhenSkipped(newStatus, len(scaleRun.Spec.Batch.Batches), scaleRun.Spec.Batch.Batches, ctx.Workloads)
		}
	case rolloutapis.AnnoManualCommandCancel:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
	case rolloutapis.AnnoManualCommandForceSkipCurrentBatch:
		handleBatchStatusWhenSkipped(newStatus, len(scaleRun.Spec.Batch.Batches), scaleRun.Spec.Batch.Batches, ctx.Workloads)
	}

	return ctrl.Result{Requeue: true}
}

// handleBatchStatusWhenSkipped advances the batch state when the current batch is manually skipped.
// - Marks the current batch record as StepSkipped.
// - Records tolerations for each workload in the current batch (scale-up scenarios only).
// - On the last batch, transitions the phase to PostRollout (which will then become Succeeded).
// - On non-last batch, advances CurrentBatchIndex and resets CurrentBatchState.
func handleBatchStatusWhenSkipped(newStatus *rolloutv1alpha1.ScaleRunStatus, batchSize int, batches []rolloutv1alpha1.ScaleRunStep, workloads *workload.Set) {
	currentBatchIndex := newStatus.Batches.CurrentBatchIndex
	if newStatus.Error != nil {
		newStatus.Error = nil
	}

	newStatus.Batches.Records[currentBatchIndex].State = rorexecutor.StepSkipped

	// Record tolerations for each workload in the current batch (scale-up only)
	recordScaleRunTolerations(&newStatus.Batches.Tolerations, batches, workloads, currentBatchIndex, newStatus.Batches.Records[currentBatchIndex].Targets)

	if int(currentBatchIndex) >= (batchSize - 1) {
		// Last batch: advance to PostRollout phase (will transition to Succeeded)
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
		return
	}

	// Not the last batch: advance to the next batch
	newStatus.Batches.CurrentBatchIndex = currentBatchIndex + 1
	newStatus.Batches.CurrentBatchState = rorexecutor.StepNone
}

// recordScaleRunTolerations upserts tolerations for each workload in the current batch.
// Toleration only applies for scale-up scenarios (ScaleFrom < ScaleTo).
// For each target, it computes the gap between ScaleTo and the workload's AvailableReplicas,
// and upserts (insert or update) the toleration value into the tolerations slice.
func recordScaleRunTolerations(tolerations *[]rolloutv1alpha1.RolloutRunTolerationTarget, batches []rolloutv1alpha1.ScaleRunStep, workloads *workload.Set, currentBatchIndex int32, currentTargets []rolloutv1alpha1.ScaleWorkloadStatus) {
	if workloads == nil || int(currentBatchIndex) >= len(batches) {
		return
	}
	currentBatch := batches[currentBatchIndex]
	for _, target := range currentBatch.Targets {
		info := workloads.Get(target.Cluster, target.Name)
		if info == nil {
			continue
		}
		// Locate ScaleFrom/ScaleTo recorded for this workload in the current batch status
		var scaledFrom, scaledTo int32
		found := false
		for _, st := range currentTargets {
			if st.Cluster == target.Cluster && st.Name == target.Name {
				scaledFrom = st.ScaleFrom
				scaledTo = st.ScaleTo
				found = true
				break
			}
		}
		if !found {
			// No status yet; fall back to spec values
			scaledFrom = info.Status.DesiredReplicas
			scaledTo = target.Replicas
		}
		// Toleration only applies for scale-up scenarios
		if scaledFrom >= scaledTo {
			continue
		}
		gap := scaledTo - info.Status.AvailableReplicas
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
