package executor

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kusionstack.io/rollout/apis/rollout"
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

// newUpgradingError construct CodeReasonMessage
func newUpgradingError(reason string, msg string) *rolloutv1alpha1.CodeReasonMessage {
	return &rolloutv1alpha1.CodeReasonMessage{Code: CodeUpgradingError, Reason: reason, Message: msg}
}

// doBatchUpgrading process upgrading state
func (r *batchExecutor) doBatchUpgrading(ctx *ExecutorContext) (ctrl.Result, error) {
	rolloutRun := ctx.RolloutRun
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
	currentBatch := rolloutRun.Spec.Batch.Batches[currentBatchIndex]

	logger := r.loggerWithContext(ctx)
	logger.Info("do batch upgrading and check")

	// upgrade partition
	metadataPatch := progressMetadataPatch(ctx)
	batchTargetStatuses := make([]rolloutv1alpha1.RolloutWorkloadStatus, 0)
	workloadChanged := false
	for _, item := range currentBatch.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			newStatus.Error = newUpgradingError(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("the workload (%s) does not exists", item.CrossClusterObjectNameReference),
			)
			return ctrl.Result{}, errors.New(newStatus.Error.Message)
		}

		// upgradePartition is an idempotent function
		changed, err := wi.UpgradePartition(item.Replicas, metadataPatch)
		if err != nil {
			newStatus.Error = newUpgradingError(
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
	newStatus.BatchStatus.Records[currentBatchIndex].Targets = batchTargetStatuses

	if workloadChanged {
		// check next time, give the controller a little time to react
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	// all workloads are updated now, then check if they are ready
	for _, item := range currentBatch.Targets {
		// target will not be nil here
		target := ctx.Workloads.Get(item.Cluster, item.Name)

		status := target.GetStatus()
		partition, _ := workload.CalculatePartitionReplicas(&status.Replicas, item.Replicas)

		if !workload.CheckPartitionReady(status, partition) {
			// ready
			logger.Info("still waiting for target ready", "target", item.CrossClusterObjectNameReference)
			return reconcile.Result{RequeueAfter: defaultRequeueAfter}, nil
		}
	}

	// all workloads are ready now, move to next phase
	newStatus.BatchStatus.CurrentBatchState = rolloutv1alpha1.RolloutStepPostBatchStepHook
	newStatus.BatchStatus.Records[currentBatchIndex].State = newStatus.BatchStatus.CurrentBatchState

	return ctrl.Result{Requeue: true}, nil
}

func progressMetadataPatch(ctx *ExecutorContext) rolloutv1alpha1.MetadataPatch {
	info := rolloutv1alpha1.ProgressingInfo{
		RolloutName: ctx.Rollout.Name,
		RolloutID:   ctx.Rollout.Status.RolloutID,
		Batch: &rolloutv1alpha1.BatchProgressingInfo{
			CurrentBatchIndex: ctx.NewStatus.BatchStatus.CurrentBatchIndex,
		},
	}
	progress, _ := json.Marshal(info)
	return rolloutv1alpha1.MetadataPatch{
		Annotations: map[string]string{rollout.AnnoRolloutProgressingInfo: string(progress)},
	}
}
