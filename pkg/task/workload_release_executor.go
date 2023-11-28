// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package task

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"kusionstack.io/rollout/apis/workflow/v1alpha1"
	"kusionstack.io/rollout/pkg/registry"
	"kusionstack.io/rollout/pkg/workload"
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
	gvk := schema.FromAPIVersionAndKind(task.Spec.WorkloadRelease.Workload.APIVersion, task.Spec.WorkloadRelease.Workload.Kind)
	store, err := registry.WorkloadRegistry.Get(gvk)
	if err != nil {
		return nil, err
	}

	wi, err := store.Get(context.TODO(), task.Spec.WorkloadRelease.Workload.Cluster, task.Spec.WorkloadRelease.Workload.Namespace, task.Spec.WorkloadRelease.Workload.Name)
	if err != nil {
		return nil, err
	}

	return &WorkloadReleaseExecutor{
		TaskExecutor: TaskExecutor{Task: task},
		workload:     wi,
		release:      task.Spec.WorkloadRelease,
		Client:       client,
	}, nil
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
	status := e.workload.GetStatus()

	releaseStatus := &v1alpha1.WorkloadReleaseTaskStatus{
		Name:                      status.Name,
		ObservedGenerationUpdated: status.Generation == status.ObservedGeneration,
		Replicas:                  status.Replicas,
		UpdatedReplicas:           status.UpdatedReplicas,
		UpdatedReadyReplicas:      status.UpdatedReadyReplicas,
		UpdatedAvailableReplicas:  status.UpdatedAvailableReplicas,
	}
	e.Task.Status.WorkloadRelease = releaseStatus
	e.TaskExecutor.CalculateStatus()
}

// convert int32 to int32 pointer
func int32Ptr(i int32) *int32 { return &i }
