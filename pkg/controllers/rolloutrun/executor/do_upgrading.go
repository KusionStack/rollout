package executor

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

const (
	CodeUpgradingError = "UpgradingError"

	ReasonCheckReadyError           = "CheckReadyError"
	ReasonUpgradePartitionError     = "UpgradePartitionError"
	ReasonWorkloadStoreNotExist     = "WorkloadStoreNotExist"
	ReasonWorkloadInterfaceNotExist = "WorkloadInterfaceNotExist"
)

// newUpgradingCodeReasonMessage construct CodeReasonMessage
func newUpgradingCodeReasonMessage(reason string, msg string) *rolloutv1alpha1.CodeReasonMessage {
	return &rolloutv1alpha1.CodeReasonMessage{Code: CodeUpgradingError, Reason: reason, Message: msg}
}

// doBatchUpgrading process upgrading state
func (r *Executor) doBatchUpgrading(_ context.Context, executorContext *ExecutorContext) (ctrl.Result, error) {
	rolloutRun := executorContext.RolloutRun
	newStatus := executorContext.NewStatus
	newBatchStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	currentBatch := rolloutRun.Spec.Batch.Batches[currentBatchIndex]

	logger := r.logger.WithValues("rollout", executorContext.Rollout.Name, "rolloutRun", executorContext.RolloutRun.Name, "currentBatchIndex", currentBatchIndex)

	logger.Info("do batch upgrading and check")

	// upgrade partition
	batchTargetStatuses := make([]rolloutv1alpha1.RolloutWorkloadStatus, 0)
	workloadChanged := false
	for _, item := range currentBatch.Targets {
		wi := executorContext.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			newStatus.Error = newUpgradingCodeReasonMessage(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("the workload (%s) does not exists", item.CrossClusterObjectNameReference),
			)
			return ctrl.Result{}, errors.New(newStatus.Error.Message)
		}

		// upgradePartition is an idempotent function
		changed, err := wi.UpgradePartition(item.Replicas)
		if err != nil {
			newStatus.Error = newUpgradingCodeReasonMessage(
				ReasonUpgradePartitionError,
				fmt.Sprintf("failed to upgrade workload(%s) partition: %v", item.CrossClusterObjectNameReference, err),
			)
			return ctrl.Result{}, errors.New(newStatus.Error.Message)
		}

		batchTargetStatuses = append(batchTargetStatuses, wi.GetStatus())

		if changed {
			workloadChanged = true
		}
	}

	// update target status in batch
	newBatchStatus.Records[currentBatchIndex].Targets = batchTargetStatuses

	if workloadChanged {
		// check next time, give the controller a little time to react
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// all workloads are updated now, then check if they are ready
	for _, item := range currentBatch.Targets {
		// target will not be nil here
		target := executorContext.Workloads.Get(item.Cluster, item.Name)

		status := target.GetStatus()
		partition, _ := workload.CalculatePartitionReplicas(&status.Replicas, item.Replicas)

		if !workload.CheckPartitionReady(status, partition) {
			// ready
			logger.Info("still waiting for target ready", "target", item.CrossClusterObjectNameReference)
			return reconcile.Result{RequeueAfter: defaultRequeueAfter}, nil
		}
	}

	// all workloads are ready now, move to next phase
	newBatchStatus.CurrentBatchState = BatchStatePostBatchHook
	newBatchStatus.Records[currentBatchIndex].State = newBatchStatus.CurrentBatchState

	return ctrl.Result{Requeue: true}, nil
}
