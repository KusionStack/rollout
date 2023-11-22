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

	"k8s.io/apimachinery/pkg/util/intstr"

	workflowapi "kusionstack.io/rollout/apis/workflow"
	workflowv1alpha1 "kusionstack.io/rollout/apis/workflow/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

// AggregateTasks will migrate the 'from' into the 'to' workflowTask.
func AggregateTasks(to []*workflowv1alpha1.WorkflowTask, from ...[]*workflowv1alpha1.WorkflowTask) []*workflowv1alpha1.WorkflowTask {
	if from == nil {
		return to
	}

	if to == nil {
		to = make([]*workflowv1alpha1.WorkflowTask, 0)
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
func BindTaskFlow(targets ...[]*workflowv1alpha1.WorkflowTask) {
	// filter the nil workflowTasks.
	filtered := make([][]*workflowv1alpha1.WorkflowTask, 0)
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
func AddSubTasks(parentTasks []*workflowv1alpha1.WorkflowTask, subTasks []*workflowv1alpha1.WorkflowTask) {
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
func InitSuspendTask(batchIndex string, from, to string, taskID int32) *workflowv1alpha1.WorkflowTask {
	return &workflowv1alpha1.WorkflowTask{
		Name: fmt.Sprintf("suspend-%s-%d", batchIndex, taskID),
		TaskSpec: workflowv1alpha1.TaskSpec{
			Suspend: &workflowv1alpha1.SuspendTask{},
		},
		Labels: map[string]string{
			workflowapi.LabelBatchIndex:  batchIndex,
			workflowapi.LabelSuspendName: SuspendKey(from, to),
			workflowapi.LabelTaskID:      fmt.Sprintf("%d", taskID),
		},
	}
}

// InitUpgradeTask returns a new upgrade task instance.
func InitUpgradeTask(batchIndex string, workload workload.Interface, partition intstr.IntOrString, waitingSeconds int32, failureThreshold *intstr.IntOrString, taskID int32) *workflowv1alpha1.WorkflowTask {
	info := workload.GetInfo()
	workflowTask := &workflowv1alpha1.WorkflowTask{
		Name: fmt.Sprintf("upgrade-%s-%d", batchIndex, taskID),
		TaskSpec: workflowv1alpha1.TaskSpec{
			WorkloadRelease: &workflowv1alpha1.WorkloadReleaseTask{
				Workload: workflowv1alpha1.Workload{
					APIVersion: info.GVK.GroupVersion().String(),
					Kind:       info.GVK.Kind,
					Name:       info.Name,
					Namespace:  info.Namespace,
					Cluster:    info.Cluster,
				},
				Partition: partition,
				ReadyCondition: &workflowv1alpha1.ReadyCondition{
					AtLeastWaitingSeconds: waitingSeconds,
				},
			},
		},
		Labels: map[string]string{
			workflowapi.LabelBatchIndex: batchIndex,
			workflowapi.LabelCluster:    info.Cluster,
			workflowapi.LabelTaskID:     fmt.Sprintf("%d", taskID),
		},
	}
	if failureThreshold != nil {
		workflowTask.TaskSpec.WorkloadRelease.ReadyCondition.FailureThreshold = *failureThreshold
	}
	return workflowTask
}

// IsWorkflowFinalPhase checks if the workflow is in final phase.
func IsWorkflowFinalPhase(flow *workflowv1alpha1.Workflow) bool {
	return flow.Status.IsSucceeded() || flow.Status.IsCanceled()
}

// SuspendKey prints the suspend key.
func SuspendKey(from, to string) string {
	return fmt.Sprintf("%s:%s", from, to)
}
