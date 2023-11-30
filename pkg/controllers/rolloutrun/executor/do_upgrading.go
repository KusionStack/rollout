package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/workloadregistry"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

const (
	defaultRequeueDuration = 10 * time.Second

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

// doUpgrading process upgrading state
func (r *Executor) doUpgrading(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	logger := logr.FromContext(ctx)

	rolloutRun := executorContext.rolloutRun
	targetType := rolloutRun.Spec.TargetType
	newBatchStatus := executorContext.newStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	currentBatch := rolloutRun.Spec.Batch.Batches[currentBatchIndex]
	logger.Info(fmt.Sprintf(
		"DefaultExecutor doUpgrading start , currentBatchIndex=%d", currentBatchIndex,
	))

	if len(currentBatch.Targets) == 0 {
		logger.Info("DefaultExecutor doUpgrading skip since targets empty")
		return true, ctrl.Result{}, nil
	}

	gvk := schema.FromAPIVersionAndKind(targetType.APIVersion, targetType.Kind)
	store, err := workloadregistry.DefaultRegistry.Get(gvk)
	if err != nil {
		newBatchStatus.CurrentBatchError = newUpgradingCodeReasonMessage(
			ReasonWorkloadStoreNotExist,
			fmt.Sprintf("Store(%s) found error, err=%v", gvk, err),
		)
		return false, ctrl.Result{}, errors.New(newBatchStatus.CurrentBatchError.Message)
	}

	// upgrade partition
	var lastUpdateAt *metav1.Time
	targets := newBatchStatus.Records[currentBatchIndex].Targets
	for _, item := range currentBatch.Targets {
		var wi workload.Interface
		wi, err = store.Get(ctx, item.Cluster, rolloutRun.Namespace, item.Name)
		if err != nil {
			newBatchStatus.CurrentBatchError = newUpgradingCodeReasonMessage(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("Workload Interface(%s) found error, err=%v", gvk, err),
			)
			return false, ctrl.Result{}, errors.New(newBatchStatus.CurrentBatchError.Message)
		}

		target, exist := utils.Find(
			targets,
			func(s *rolloutv1alpha1.RolloutWorkloadStatus) bool {
				return (s.Name == item.Name) && (s.Cluster == item.Cluster)
			},
		)
		if !exist {
			target = rolloutv1alpha1.RolloutWorkloadStatus{
				Name: item.Name, Cluster: item.Cluster,
			}
			newBatchStatus.Records[currentBatchIndex].Targets = append(targets, target)
		}

		if target.UpdatedAt == nil {
			var ok bool
			ok, err = wi.UpgradePartition(&item.Replicas)
			if ok {
				target.UpdatedAt = &metav1.Time{Time: time.Now()}
				if lastUpdateAt.Before(target.UpdatedAt) {
					lastUpdateAt = target.UpdatedAt
				}
			} else if err == nil {
				newBatchStatus.CurrentBatchError = newUpgradingCodeReasonMessage(
					ReasonUpgradePartitionError,
					fmt.Sprintf("Upgrade partition(%v) failed", item),
				)
				return false, ctrl.Result{Requeue: true}, nil
			} else {
				newBatchStatus.CurrentBatchError = newUpgradingCodeReasonMessage(
					ReasonUpgradePartitionError,
					fmt.Sprintf("Upgrade partition(%v) error, err=%v", item, err),
				)
				return false, ctrl.Result{}, errors.New(newBatchStatus.CurrentBatchError.Message)
			}
		}
	}

	// wait for initial delay
	var initialDelaySeconds int32
	var failureThreshold *intstr.IntOrString
	if rolloutRun.Spec.Batch.Toleration != nil {
		initialDelaySeconds = rolloutRun.Spec.Batch.Toleration.InitialDelaySeconds
		failureThreshold = rolloutRun.Spec.Batch.Toleration.WorkloadFailureThreshold
	}
	duration := time.Until(lastUpdateAt.Add(time.Duration(initialDelaySeconds) * time.Second))
	if duration > 0 {
		logger.Info(fmt.Sprintf(
			"DefaultExecutor doUpgrading skip since wait for initialDelaySeconds %s s", duration,
		))
		return false, ctrl.Result{RequeueAfter: duration}, nil
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
			newBatchStatus.CurrentBatchError = newUpgradingCodeReasonMessage(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("Workload Interface(%s) found error, err=%v", gvk, err),
			)
			return false, ctrl.Result{}, errors.New(newBatchStatus.CurrentBatchError.Message)
		}

		expectReadyReplicas, err = wi.CalculateAtLeastUpdatedAvailableReplicas(failureThreshold)
		if err != nil {
			newBatchStatus.CurrentBatchError = newUpgradingCodeReasonMessage(
				ReasonCheckReadyError,
				fmt.Sprintf("%v Detect ExpectReadyReplicas error, err=%v", item, err),
			)
			return false, ctrl.Result{}, errors.New(newBatchStatus.CurrentBatchError.Message)
		}

		ready, err = wi.CheckReady(pointer.Int32((int32)(expectReadyReplicas)))
		if err != nil {
			logger.Info("%v CheckReady error, err=%v", item, err)
			err = fmt.Errorf("%v CheckReady error, err=%v", item, err)
			return false, ctrl.Result{RequeueAfter: defaultRequeueDuration}, err
		} else if !ready {
			logger.Info(fmt.Sprintf("%v CheckReady not ready", item))
			return false, ctrl.Result{RequeueAfter: defaultRequeueDuration}, nil
		}
	}

	return true, ctrl.Result{Requeue: true}, nil
}
