/*
 * Copyright 2023 The KusionStack Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package workflow

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/KusionStack/rollout/api"
	rolloutv1alpha1 "github.com/KusionStack/rollout/api/v1alpha1"
	"github.com/KusionStack/rollout/pkg/workload"
)

// AggregateTasks will migrate the 'from' into the 'to' workflowTask.
func AggregateTasks(to []*rolloutv1alpha1.WorkflowTask, from ...[]*rolloutv1alpha1.WorkflowTask) []*rolloutv1alpha1.WorkflowTask {
	if from == nil {
		return to
	}

	if to == nil {
		to = make([]*rolloutv1alpha1.WorkflowTask, 0)
	}
	for i := range from {
		if from[i] != nil && len(from[i]) > 0 {
			for j := range from[i] {
				if from[i][j] != nil {
					to = append(to, from[i][j])
				}
			}
		}
	}
	return to
}

// BindTaskFlow will build the workflowTask chain that depends on the order of the input targets.
func BindTaskFlow(targets ...[]*rolloutv1alpha1.WorkflowTask) {
	// filter the nil workflowTasks.
	filtered := make([][]*rolloutv1alpha1.WorkflowTask, 0)
	for i := range targets {
		if targets[i] != nil && len(targets[i]) > 0 && targets[i][0] != nil {
			filtered = append(filtered, targets[i])
		}
	}

	if len(filtered) > 1 {
		for i := 0; i < len(filtered)-1; i++ {
			AddSubTasks(filtered[i], filtered[i+1])
		}
	}
}

// AddSubTasks will assemble the execution order of tasks.
func AddSubTasks(parentTasks []*rolloutv1alpha1.WorkflowTask, subTasks []*rolloutv1alpha1.WorkflowTask) {
	if parentTasks == nil || len(parentTasks) < 1 {
		return
	}
	if subTasks == nil || len(subTasks) < 1 {
		return
	}
	for i := range subTasks {
		if subTasks[i] != nil {
			if subTasks[i].RunAfter == nil {
				subTasks[i].RunAfter = []string{}
			}
			for j := range parentTasks {
				if parentTasks[j] != nil {
					subTasks[i].RunAfter = append(subTasks[i].RunAfter, parentTasks[j].Name)
				}
			}
		}
	}
}

// InitSuspendTask returns a new suspend task instance.
func InitSuspendTask(batchName string, suffix, from, to string, taskID int32) *rolloutv1alpha1.WorkflowTask {
	return &rolloutv1alpha1.WorkflowTask{
		Name: fmt.Sprintf("suspend-%s-%s-%d", suffix, batchName, taskID),
		TaskSpec: rolloutv1alpha1.TaskSpec{
			Suspend: &rolloutv1alpha1.SuspendTask{},
		},
		Labels: map[string]string{
			api.LabelSuspendName:   SuspendKey(from, to),
			api.LabelRolloutTaskID: fmt.Sprintf("%d", taskID),
		},
	}
}

// InitAnalysisTask returns a new analysis task instance.
func InitAnalysisTask(batchName string, rule *rolloutv1alpha1.AnalysisRule, suffix string, taskID int32) *rolloutv1alpha1.WorkflowTask {
	return &rolloutv1alpha1.WorkflowTask{
		Name: fmt.Sprintf("analysis-%s-%s-%d", suffix, batchName, taskID),
		TaskSpec: rolloutv1alpha1.TaskSpec{
			Analysis: &rolloutv1alpha1.AnalysisTask{},
		},
		Labels: map[string]string{
			api.LabelRolloutBatchName:        batchName,
			api.LabelRolloutAnalysisProvider: rule.Provider.Name,
			api.LabelRolloutTaskID:           fmt.Sprintf("%d", taskID),
		},
	}
}

// InitUpgradeTask returns a new upgrade task instance.
func InitUpgradeTask(batchName string, workload workload.Interface, partition intstr.IntOrString, waitingSeconds int32, failureThreshold *intstr.IntOrString, taskID int32) *rolloutv1alpha1.WorkflowTask {
	workflowTask := &rolloutv1alpha1.WorkflowTask{
		Name: fmt.Sprintf("upgrade-%s-%d", batchName, taskID),
		TaskSpec: rolloutv1alpha1.TaskSpec{
			WorkloadRelease: &rolloutv1alpha1.WorkloadReleaseTask{
				Workload: rolloutv1alpha1.Workload{
					TypeMeta:  *workload.GetTypeMeta(),
					Name:      workload.GetObj().GetName(),
					Namespace: workload.GetObj().GetNamespace(),
					Cluster:   workload.GetCluster(),
				},
				Partition: partition,
				ReadyCondition: &rolloutv1alpha1.ReadyCondition{
					AtLeastWaitingSeconds: waitingSeconds,
				},
			},
		},
		Labels: map[string]string{
			api.LabelRolloutBatchName: batchName,
			api.LabelRolloutCluster:   workload.GetCluster(),
			api.LabelRolloutWorkload:  workload.GetObj().GetName(),
			api.LabelRolloutTaskID:    fmt.Sprintf("%d", taskID),
		},
	}
	if failureThreshold != nil {
		workflowTask.TaskSpec.WorkloadRelease.ReadyCondition.FailureThreshold = *failureThreshold
	}
	return workflowTask
}

// IsWorkflowFinalPhase checks if the workflow is in final phase.
func IsWorkflowFinalPhase(flow *rolloutv1alpha1.Workflow) bool {
	return flow.Status.IsSucceeded() || flow.Status.IsCancelled()
}

// SuspendKey prints the suspend key.
func SuspendKey(from, to string) string {
	return fmt.Sprintf("%s:%s", from, to)
}

// UpdateStatusCallBack is a callback func that called at the end of CalcRollout if rollout instance should be updated.
type UpdateStatusCallBack func(stages []rolloutv1alpha1.Stage, latestProcessingStageIndex, latestBatchStageIndex int)

// CalcRollout will find the latest processing stage and latest batch stage.
// At the end of func, will call the callback func if the rollout instance needs to update status.
func CalcRollout(stages []rolloutv1alpha1.Stage, workflowInfo *rolloutv1alpha1.WorkflowInfo, workflow *rolloutv1alpha1.Workflow, cbFn UpdateStatusCallBack) error {
	if len(stages) == 0 {
		if workflow.Annotations == nil {
			workflow.Annotations = map[string]string{}
		}
		if val, ok := workflow.Annotations[api.AnnoWorkflowBatchInfo]; ok && val != "" {
			annoStages := make([]rolloutv1alpha1.Stage, 0)
			err := json.Unmarshal([]byte(val), &annoStages)
			if err != nil {
				return err
			}
			stages = annoStages
		} else {
			return fmt.Errorf("no valid batch info")
		}
	}
	workflowInfo.Name = workflow.Name
	workflowInfo.Conditions = workflow.Status.Conditions

	taskStatuses := workflow.Status.Tasks
	if taskStatuses == nil || len(taskStatuses) < 1 {
		return nil
	}

	batchTaskMap := make(map[string][]*rolloutv1alpha1.WorkflowTaskStatus)
	suspendTaskMap := make(map[string]*rolloutv1alpha1.WorkflowTaskStatus)
	for i := range taskStatuses {
		if taskStatuses[i].Labels != nil && taskStatuses[i].Labels[api.LabelRolloutBatchName] != "" {
			key := taskStatuses[i].Labels[api.LabelRolloutBatchName]
			if batchTaskMap[key] == nil {
				batchTaskMap[key] = []*rolloutv1alpha1.WorkflowTaskStatus{}
			}
			batchTaskMap[key] = append(batchTaskMap[key], &taskStatuses[i])
		} else if taskStatuses[i].Labels != nil && taskStatuses[i].Labels[api.LabelSuspendName] != "" {
			key := taskStatuses[i].Labels[api.LabelSuspendName]
			suspendTaskMap[key] = &taskStatuses[i]
		}
	}

	latestProcessingStageIndex := -1
	latestBatchStageIndex := -1
	for i := range stages {
		if stages[i].Type == rolloutv1alpha1.StageTypeSuspend {
			key := SuspendKey(stages[i].PrevStage, stages[i].NextStage)
			if suspendTask, has := suspendTaskMap[key]; has && suspendTask != nil {
				stages[i].SuccessCondition = suspendTask.SuccessCondition
				if latestProcessingStageIndex == 0 && suspendTask.IsProcessing() {
					// index begin from 1
					latestProcessingStageIndex = i
				}
			}
		} else if stages[i].Type == rolloutv1alpha1.StageTypeBatch {
			if tasks, ok := batchTaskMap[stages[i].Name]; ok {
				stages[i].UpdateBatchInfo(tasks)
				if latestProcessingStageIndex == 0 && stages[i].IsProcessing() {
					// index begin from 1
					latestProcessingStageIndex = i
				}
				latestBatchStageIndex = i
			}
		}
	}

	if latestProcessingStageIndex == -1 {
		latestProcessingStageIndex = latestBatchStageIndex
	}
	if cbFn != nil {
		cbFn(stages, latestProcessingStageIndex, latestBatchStageIndex)
	}
	return nil
}

// ParsePausedSuspendTask will return the paused suspend task if exists.
func ParsePausedSuspendTask(stages []rolloutv1alpha1.Stage) *rolloutv1alpha1.Stage {
	if stages == nil {
		return nil
	}
	for i := range stages {
		if stages[i].Type == rolloutv1alpha1.StageTypeSuspend && stages[i].IsPaused() {
			return &stages[i]
		}
	}
	return nil
}

// SyncFlow will update the rollout instance labels and annotations when the workflow has new manual command or resume context
// and return whether it needs to update rollout instance and workflow or not.
func SyncFlow(rolloutInstanceLabels, rolloutInstanceAnnotations map[string]string, stages []rolloutv1alpha1.Stage, workflow *rolloutv1alpha1.Workflow) (needUpdateRolloutInstance, needUpdateWorkflow bool) {
	needUpdateRolloutInstance = false
	needUpdateWorkflow = false
	if rolloutInstanceAnnotations == nil {
		rolloutInstanceAnnotations = map[string]string{}
	}
	if workflow.Annotations == nil {
		workflow.Annotations = map[string]string{}
	}
	if command, has := rolloutInstanceLabels[api.LabelRolloutManualCommand]; has {
		if command == api.LabelRolloutManualCommandResume {
			if workflow.Status.IsPaused() {
				if suspendStage := ParsePausedSuspendTask(stages); suspendStage != nil {
					// paused by suspend task
					suspendTaskNames := []string{suspendStage.Name}
					suspendTaskNameJson, _ := json.Marshal(suspendTaskNames)
					rolloutInstanceAnnotations[api.AnnoRolloutResumeContext] = string(suspendTaskNameJson)
					// TODO: what if workflow resume failed, but rollout resume label is deleted?
					delete(rolloutInstanceLabels, api.LabelRolloutManualCommand)
					needUpdateRolloutInstance = true
				}
				if workflow.Spec.Status == rolloutv1alpha1.WorkflowSpecStatusPaused {
					needUpdateWorkflow = false
					workflow.Spec.Status = ""
				}
			} else {
				// no need to resume
				needUpdateRolloutInstance = true
				delete(rolloutInstanceLabels, api.LabelRolloutManualCommand)
			}
		} else if command == api.LabelRolloutManualCommandPause {
			if workflow.Status.IsPaused() || workflow.Spec.Status == rolloutv1alpha1.WorkflowSpecStatusPaused {
				// no need to trigger pause
				needUpdateRolloutInstance = true
				delete(rolloutInstanceLabels, api.LabelRolloutManualCommand)
			} else {
				needUpdateWorkflow = true
				workflow.Spec.Status = rolloutv1alpha1.WorkflowSpecStatusPaused
			}
		} else if command == api.LabelRolloutManualCommandCancel {
			if workflow.Status.IsCancelled() || workflow.Spec.Status == rolloutv1alpha1.WorkflowSpecStatusCancelled {
				// no need to trigger cancel
				needUpdateRolloutInstance = true
				delete(rolloutInstanceLabels, api.LabelRolloutManualCommand)
			} else {
				needUpdateWorkflow = true
				workflow.Spec.Status = rolloutv1alpha1.WorkflowSpecStatusCancelled
			}
		} else {
			// invalid value
			needUpdateRolloutInstance = true
			delete(rolloutInstanceLabels, api.LabelRolloutManualCommand)
		}
	}
	if resumeContext, has := rolloutInstanceAnnotations[api.AnnoRolloutResumeContext]; has {
		if workflowResumeContext, has2 := workflow.Annotations[api.AnnoWorkflowResumeSuspendTasks]; !has2 || workflowResumeContext != resumeContext {
			workflow.Annotations[api.AnnoWorkflowResumeSuspendTasks] = resumeContext
			needUpdateWorkflow = true
		}
	}
	return
}
