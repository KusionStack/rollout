package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/workloadregistry"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

const (
	ctxKeyLastUpgradeAt = "LastUpgradeAt"
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
func (r *Executor) doBatchUpgrading(ctx context.Context, executorContext *ExecutorContext) (ctrl.Result, error) {
	rolloutRun := executorContext.RolloutRun
	newProgressingStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newProgressingStatus.CurrentBatchIndex
	currentBatch := rolloutRun.Spec.Batch.Batches[currentBatchIndex]
	r.logger.Info(
		"DefaultExecutor doBatchUpgrading start", "currentBatchIndex", currentBatchIndex,
	)

	if len(currentBatch.Targets) == 0 {
		r.logger.Info("DefaultExecutor doUpgrading skip since targets empty")
		newProgressingStatus.CurrentBatchState = BatchStatePostBatchHook
		newProgressingStatus.Records[currentBatchIndex].State = newProgressingStatus.CurrentBatchState
		return ctrl.Result{Requeue: true}, nil
	}

	targetType := rolloutRun.Spec.TargetType
	gvk := schema.FromAPIVersionAndKind(targetType.APIVersion, targetType.Kind)
	store, err := workloadregistry.DefaultRegistry.Get(gvk)
	if err != nil {
		newProgressingStatus.Error = newUpgradingCodeReasonMessage(
			ReasonWorkloadStoreNotExist,
			fmt.Sprintf("gvk(%s) is unsupported, err=%v", gvk, err),
		)
		return ctrl.Result{}, errors.New(newProgressingStatus.Error.Message)
	}

	// upgrade partition
	targets := newProgressingStatus.Records[currentBatchIndex].Targets
	for _, item := range currentBatch.Targets {
		var wi workload.Interface
		wi, err = store.Get(ctx, item.Cluster, rolloutRun.Namespace, item.Name)
		if err != nil {
			newProgressingStatus.Error = newUpgradingCodeReasonMessage(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("get Workload Interface(%s) error, err=%v", gvk, err),
			)
			return ctrl.Result{}, errors.New(newProgressingStatus.Error.Message)
		} else if wi == nil {
			newProgressingStatus.Error = newUpgradingCodeReasonMessage(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("Workload Interface(%s) is not exist", gvk),
			)
			return ctrl.Result{}, errors.New(newProgressingStatus.Error.Message)
		}

		_, exist := utils.Find(
			targets,
			func(s *rolloutv1alpha1.RolloutWorkloadStatus) bool {
				return (s.Name == item.Name) && (s.Cluster == item.Cluster)
			},
		)
		if !exist {
			_, err = wi.UpgradePartition(&item.Replicas)
			if err == nil {
				target := &rolloutv1alpha1.RolloutWorkloadStatus{
					Name: item.Name, Cluster: item.Cluster,
				}
				lastUpgradeAt := time.Now().UTC().Format(time.RFC3339)
				if newProgressingStatus.Context == nil {
					newProgressingStatus.Context = map[string]string{}
				}
				newProgressingStatus.Context[ctxKeyLastUpgradeAt] = lastUpgradeAt
				newProgressingStatus.Records[currentBatchIndex].Targets = append(targets, *target)
			} else {
				newProgressingStatus.Error = newUpgradingCodeReasonMessage(
					ReasonUpgradePartitionError,
					fmt.Sprintf("Upgrade partition(%v) error, err=%v", item, err),
				)
				return ctrl.Result{}, errors.New(newProgressingStatus.Error.Message)
			}
		}
	}

	// wait for initial delay
	var (
		initialDelaySeconds int32
		lastUpgradeAt       = time.Now()
		failureThreshold    *intstr.IntOrString
	)
	if val, exist := newProgressingStatus.Context[ctxKeyLastUpgradeAt]; exist {
		if lastUpgradeAt, err = time.Parse(time.RFC3339, val); err != nil {
			lastUpgradeAt = time.Now()
			r.logger.Error(
				err, "failed since err lastUpgradeAt value", ctxKeyLastUpgradeAt, val,
			)
		}
	}
	if rolloutRun.Spec.Batch.Toleration != nil {
		initialDelaySeconds = rolloutRun.Spec.Batch.Toleration.InitialDelaySeconds
		failureThreshold = rolloutRun.Spec.Batch.Toleration.WorkloadFailureThreshold
	}
	duration := time.Until(lastUpgradeAt.Add(time.Duration(initialDelaySeconds) * time.Second))
	if duration > 0 {
		r.logger.Info(
			"skip since wait for initialDelay", "initialDelaySeconds", duration,
		)
		return ctrl.Result{RequeueAfter: duration}, nil
	}

	// check ready
	for _, item := range currentBatch.Targets {
		var (
			ready               bool
			expectReadyReplicas int
			wi                  workload.Interface
		)
		wi, err = store.Get(ctx, item.Cluster, rolloutRun.Namespace, item.Name)
		if err != nil {
			newProgressingStatus.Error = newUpgradingCodeReasonMessage(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("get Workload Interface(%s) error, err=%v", gvk, err),
			)
			return ctrl.Result{}, errors.New(newProgressingStatus.Error.Message)
		} else if wi == nil {
			newProgressingStatus.Error = newUpgradingCodeReasonMessage(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("Workload Interface(%s) is not exist", gvk),
			)
			return ctrl.Result{}, errors.New(newProgressingStatus.Error.Message)
		}

		ready, err = wi.CheckReady(nil)
		if err != nil {
			r.logger.Info("Target CheckReady error", "target", item, "error", err)
			newProgressingStatus.Error = newUpgradingCodeReasonMessage(
				ReasonUpgradePartitionError,
				fmt.Sprintf("%v CheckReady error, err=%v", item, err),
			)
			return ctrl.Result{RequeueAfter: defaultRequeueAfter}, err
		} else if !ready {
			if failureThreshold == nil {
				failureThreshold = ptr.To(intstr.FromInt(0))
			}
			expectReadyReplicas, err = wi.CalculateAtLeastUpdatedAvailableReplicas(failureThreshold)
			if err != nil {
				newProgressingStatus.Error = newUpgradingCodeReasonMessage(
					ReasonCheckReadyError,
					fmt.Sprintf("%v Detect ExpectReadyReplicas error, err=%v", item, err),
				)
				return ctrl.Result{}, errors.New(newProgressingStatus.Error.Message)
			}

			ready, err = wi.CheckReady(ptr.To(int32(expectReadyReplicas)))
			if err != nil {
				r.logger.Info("Target CheckReady error", "target", item, "error", err)
				newProgressingStatus.Error = newUpgradingCodeReasonMessage(
					ReasonUpgradePartitionError,
					fmt.Sprintf("%v CheckReady error, err=%v", item, err),
				)
				return ctrl.Result{RequeueAfter: defaultRequeueAfter}, err
			} else if !ready {
				r.logger.Info("Target CheckReady not ready", "target", item)
				return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
			}
		}
	}

	newProgressingStatus.CurrentBatchState = BatchStatePostBatchHook
	newProgressingStatus.Records[currentBatchIndex].State = newProgressingStatus.CurrentBatchState

	return ctrl.Result{Requeue: true}, nil
}
