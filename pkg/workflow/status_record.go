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

package workflow

import (
	"fmt"
	"sort"
	"strconv"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	workflowapi "kusionstack.io/rollout/apis/workflow"
	workflowv1alpha1 "kusionstack.io/rollout/apis/workflow/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
)

type batchStatusRecord struct {
	index         int32
	pausedTask    *workflowv1alpha1.WorkflowTaskStatus
	workflowTasks []*workflowv1alpha1.WorkflowTaskStatus
}

func GetBatchStatusRecords(workflow *workflowv1alpha1.Workflow) ([]rolloutv1alpha1.RolloutRunBatchStatusRecord, error) {
	stages := make(map[int]batchStatusRecord, 0)

	for _, t := range workflow.Spec.Tasks {
		batchIndex, ok := t.Labels[workflowapi.LabelBatchIndex]
		if !ok {
			// this is an error
			continue
		}

		index, err := strconv.Atoi(batchIndex)
		if err != nil {
			continue
		}
		stage, ok := stages[index]
		if !ok {
			stages[index] = batchStatusRecord{
				index: int32(index),
			}
			stage = stages[index]
		}
		taskStatus := getTaskStatus(workflow.Status.Tasks, t.Name)
		if taskStatus == nil {
			continue
		}

		if t.TaskSpec.WorkloadRelease != nil {
			stage.workflowTasks = append(stage.workflowTasks, taskStatus)
		} else if t.TaskSpec.Suspend != nil {
			stage.pausedTask = taskStatus
		}
		stages[index] = stage
	}

	result := []rolloutv1alpha1.RolloutRunBatchStatusRecord{}

	for _, s := range stages {
		result = append(result, s.Convert())
	}

	sort.Slice(result, func(i, j int) bool {
		return *result[i].Index < *result[j].Index
	})

	return result, nil
}

func getTaskStatus(taskStatuses []workflowv1alpha1.WorkflowTaskStatus, taskName string) *workflowv1alpha1.WorkflowTaskStatus {
	for i, s := range taskStatuses {
		if s.Name == taskName {
			return &taskStatuses[i]
		}
	}
	return nil
}

func (ss batchStatusRecord) Convert() rolloutv1alpha1.RolloutRunBatchStatusRecord {
	status := rolloutv1alpha1.RolloutRunBatchStatusRecord{
		Index: &ss.index,
		State: rolloutv1alpha1.BatchStepStatePending,
	}

	// the pausedTask will be first task in batch now
	if ss.pausedTask != nil {
		if ss.pausedTask.StartedAt == nil {
			// paused Task is not started
			status.State = rolloutv1alpha1.BatchStepStatePending
			status.Message = "this batch is not started now"
			return status
		}
		status.StartTime = ss.pausedTask.StartedAt
		if !ss.pausedTask.IsSucceeded() {
			// paused Task is not finished
			status.State = rolloutv1alpha1.BatchStepStatePaused
			status.Message = "this batch is paused now"
			return status
		}
	}

	var succeed, canceled, failed, skipped, running, pending, paused int
	all := len(ss.workflowTasks)
	for _, task := range ss.workflowTasks {
		if task.WorkloadRelease != nil {
			cluster, _ := utils.GetMapValue(task.Labels, workflowapi.LabelCluster)
			detail := rolloutv1alpha1.RolloutWorkloadStatus{
				RolloutReplicasSummary: rolloutv1alpha1.RolloutReplicasSummary{
					Replicas:                 task.WorkloadRelease.Replicas,
					UpdatedReplicas:          task.WorkloadRelease.UpdatedReplicas,
					UpdatedReadyReplicas:     task.WorkloadRelease.UpdatedReadyReplicas,
					UpdatedAvailableReplicas: task.WorkloadRelease.UpdatedAvailableReplicas,
				},
				Name:    task.WorkloadRelease.Name,
				Cluster: cluster,
			}
			status.Targets = append(status.Targets, detail)
		}

		if status.StartTime == nil {
			status.StartTime = task.StartedAt
		} else if !status.StartTime.Before(task.StartedAt) {
			status.StartTime = task.StartedAt
		}

		if status.FinishTime == nil {
			status.FinishTime = task.FinishedAt
		} else if status.FinishTime.Before(task.FinishedAt) {
			status.FinishTime = task.FinishedAt
		}

		if task.SuccessCondition == nil || task.SuccessCondition.Type != workflowv1alpha1.WorkflowConditionSucceeded {
			pending++
		} else {
			switch {
			case task.IsSucceeded():
				succeed++
			case task.IsCanceled():
				canceled++
			case task.IsFailed():
				failed++
			case task.IsSkipped():
				skipped++
			case task.IsRunning():
				running++
			case task.IsPaused():
				paused++
			default:
				pending++
			}
		}
	}

	completedTasks := succeed + canceled
	incompleteTasks := running + pending + paused + failed
	status.Message = fmt.Sprintf("Tasks Completed: %d (Succeed: %d, Canceled: %d), Tasks Incomplete: %d (Canceled: %d, Paused: %d, Running: %d, Pending: %d), Skipped: %d",
		completedTasks, succeed, failed, incompleteTasks, failed, paused, running, pending, skipped)

	if failed > 0 {
		status.State = rolloutv1alpha1.BatchStepStateRunning
		status.Error = &rolloutv1alpha1.CodeReasonMessage{
			Reason:  "TaskFailed",
			Message: fmt.Sprintf("%d tasks failed", failed),
		}
	} else if all == pending {
		status.State = rolloutv1alpha1.BatchStepStatePending
	} else if all == succeed {
		status.State = rolloutv1alpha1.BatchStepStateSucceeded
	} else if running > 0 {
		// running has a higher priority than canceled
		status.State = rolloutv1alpha1.BatchStepStateRunning
	} else if canceled > 0 {
		// no running no error with canceled
		status.State = rolloutv1alpha1.BatchStepStateCanceled
	}

	switch status.State {
	case rolloutv1alpha1.BatchStepStatePending, rolloutv1alpha1.BatchStepStateRunning:
		// delete finish time if step is still waiting or running
		status.FinishTime = nil
	}

	return status
}

func GetCurrentBatch(ss []rolloutv1alpha1.RolloutRunBatchStatusRecord) rolloutv1alpha1.RolloutBatchStatus {
	currentIndex := -1
	allSteps := len(ss)
	for i, s := range ss {
		if i < allSteps-1 && s.State == rolloutv1alpha1.BatchStepStateSucceeded {
			continue
		}
		// first not successed step or last step
		currentIndex = i
		break
	}
	if currentIndex == -1 {
		return rolloutv1alpha1.RolloutBatchStatus{}
	}
	currentBatch := ss[currentIndex]

	return rolloutv1alpha1.RolloutBatchStatus{
		CurrentBatchIndex: int32(currentIndex),
		CurrentBatchState: currentBatch.State,
		CurrentBatchError: currentBatch.Error,
	}
}
