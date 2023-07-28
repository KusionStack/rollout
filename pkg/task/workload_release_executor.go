package task

import (
	"context"
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"

	"github.com/KusionStack/rollout/api/v1alpha1"
	"github.com/KusionStack/rollout/pkg/workload"
)

const defaultRequeueDuration = 10 * time.Second

const (
	resultReasonSucceeded              = "Succeeded"
	resultReasonFailed                 = "Failed"
	resultReasonPartitionUpdated       = "PartitionUpdated"
	resultReasonEnsureUpdatedAvailable = "EnsureUpdatedAvailable"
	resultReasonReadyConditionMet      = "ReadyConditionMet"
)

// WorkloadReleaseExecutor is the executor for workload release task
type WorkloadReleaseExecutor struct {
	TaskExecutor
	workload workload.Interface
	release  *v1alpha1.WorkloadReleaseTask
	client.Client
}

// NewWorkloadReleaseExecutor creates a new workload release executor
func NewWorkloadReleaseExecutor(client client.Client, task *v1alpha1.Task) (Executor, error) {
	var wi workload.Interface
	var err error

	switch task.Spec.WorkloadRelease.Workload.GroupVersionKind() {
	// TODO: initialize wi with real workload implementation
	default:
		err = fmt.Errorf("unsupported workload type %s", task.Spec.WorkloadRelease.Workload.GroupVersionKind())
	}
	return &WorkloadReleaseExecutor{
		TaskExecutor: TaskExecutor{Task: task},
		workload:     wi,
		release:      task.Spec.WorkloadRelease,
		Client:       client,
	}, err
}

// Run runs the workload release task
func (e *WorkloadReleaseExecutor) Run(ctx context.Context) (d time.Duration, err error) {
	logger := log.FromContext(ctx)
	defer func() {
		if err != nil {
			e.Result.Status = TaskResultStatusFailed
			e.Result.Reason = resultReasonFailed
			e.Result.Message = err.Error()
		}
	}()

	var needUpdate bool
	needUpdate, err = e.workload.UpgradePartition(&e.release.Partition)
	if err != nil {
		return 0, err
	}
	if needUpdate {
		logger.Info("partition updated")
		e.Result.Status = TaskResultStatusRunning
		e.Result.Reason = resultReasonPartitionUpdated
		e.Result.Message = fmt.Sprintf("partition updated to %s", e.release.Partition.String())
		return 0, nil
	}

	var ready bool
	ready, err = e.workload.CheckReady(nil)
	if err != nil {
		return 0, err
	}
	if ready {
		e.Result.Status = TaskResultStatusSucceeded
		e.Result.Reason = resultReasonSucceeded
		e.Result.Message = ""
		return 0, nil
	}

	// TODO(hengzhuo): check observedGeneration and generation

	readyCondition := e.release.ReadyCondition
	if readyCondition != nil && e.release.Partition.String() != "100%" {
		if e.Task.Status.StartedAt.Add(time.Duration(readyCondition.AtLeastWaitingSeconds) * time.Second).After(time.Now()) {
			var expectReadyReplicas int
			expectReadyReplicas, err = e.workload.CalculateAtLeastUpdatedAvailableReplicas(&readyCondition.FailureThreshold)
			if err != nil {
				return 0, err
			}
			ready, err = e.workload.CheckReady(int32Ptr((int32)(expectReadyReplicas)))
			if err != nil {
				return 0, err
			}
			if ready {
				// TODO(common_release/rollout#30): emit event
				e.Result.Status = TaskResultStatusSucceeded
				e.Result.Reason = resultReasonReadyConditionMet
				e.Result.Message = fmt.Sprintf("ready condition met, wait seconds: %d, expect ready replicas: %d", readyCondition.AtLeastWaitingSeconds, expectReadyReplicas)
				return 0, nil
			}
		}
	}

	e.Result.Status = TaskResultStatusRunning
	e.Result.Reason = resultReasonEnsureUpdatedAvailable
	e.Result.Message = ""
	return defaultRequeueDuration, nil
}

// CalculateStatus calculates the status of the task
func (e *WorkloadReleaseExecutor) CalculateStatus() {
	e.Task.Status.WorkloadRelease = e.workload.GetReleaseStatus()
	e.TaskExecutor.CalculateStatus()
}

// convert int32 to int32 pointer
func int32Ptr(i int32) *int32 { return &i }
