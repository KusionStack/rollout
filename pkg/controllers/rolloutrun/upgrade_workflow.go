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

package rolloutrun

import (
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	workflowv1alpha1 "kusionstack.io/rollout/apis/workflow/v1alpha1"
	workflowutil "kusionstack.io/rollout/pkg/workflow"
	"kusionstack.io/rollout/pkg/workload"
)

func constructRunWorkflow(obj *rolloutv1alpha1.RolloutRun, workloads *workload.Set) (*workflowv1alpha1.Workflow, error) {
	owner := metav1.NewControllerRef(obj, rolloutv1alpha1.SchemeGroupVersion.WithKind("RolloutRun"))
	workflow := &workflowv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: obj.Namespace,
			Name:      obj.Name,
			Labels: map[string]string{
				rollout.LabelControl:   "true",
				rollout.LabelCreatedBy: obj.Name,
			},
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec: workflowv1alpha1.WorkflowSpec{},
	}

	var initialDelaySeconds int32
	var failureThreshold *intstr.IntOrString
	if obj.Spec.Batch.Toleration != nil {
		initialDelaySeconds = obj.Spec.Batch.Toleration.InitialDelaySeconds
		failureThreshold = obj.Spec.Batch.Toleration.WorkloadFailureThreshold
	}

	tasks := make([]*workflowv1alpha1.WorkflowTask, 0)
	var lastBatchEndTasks []*workflowv1alpha1.WorkflowTask
	taskID := int32(1)

	for index, batch := range obj.Spec.Batch.Batches {
		if len(batch.Targets) == 0 {
			return nil, fmt.Errorf("invalid batch without workload tasks")
		}
		var workloadTask []*workflowv1alpha1.WorkflowTask
		var pauseTask *workflowv1alpha1.WorkflowTask

		batchIndex := strconv.Itoa(index)
		nextBatchIndex := strconv.Itoa(index + 1)
		for _, target := range batch.Targets {
			w := workloads.Get(target.Cluster, target.Name)
			if w == nil {
				//TODO: Should it be an error?
				continue
			}
			task := workflowutil.InitUpgradeTask(batchIndex, w, target.Replicas, initialDelaySeconds, failureThreshold, taskID)
			workloadTask = append(workloadTask, task)
			taskID++
		}
		if batch.Pause != nil && *batch.Pause {
			// NOTE: we put the pauseTask to next batch
			pauseTask = workflowutil.InitSuspendTask(nextBatchIndex, batchIndex, nextBatchIndex, taskID)
			taskID++
		}
		workflowutil.BindTaskFlow(lastBatchEndTasks, workloadTask, []*workflowv1alpha1.WorkflowTask{pauseTask})
		tasks = workflowutil.AggregateTasks(tasks, workloadTask, []*workflowv1alpha1.WorkflowTask{pauseTask})

		if pauseTask != nil {
			lastBatchEndTasks = []*workflowv1alpha1.WorkflowTask{pauseTask}
		} else {
			lastBatchEndTasks = workloadTask
		}
	}

	workflow.Spec.Tasks = make([]workflowv1alpha1.WorkflowTask, 0)
	for i := range tasks {
		workflow.Spec.Tasks = append(workflow.Spec.Tasks, *tasks[i])
	}
	return workflow, nil
}
