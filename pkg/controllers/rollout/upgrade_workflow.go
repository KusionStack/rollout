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

package rollout

import (
	"fmt"
	"math"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"

	"kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/features"
	"kusionstack.io/rollout/pkg/features/ontimestrategy"
	"kusionstack.io/rollout/pkg/workload"
)

// func constructBatches(strategy *rolloutv1alpha1.RolloutStrategy, workloadWrappers []workload.Interface) []types.BatchDetail {
// 	batches := strategy.Batch.Batches
// 	if strategy.Batch.FastBatch != nil {
// 		batches = convertFastBatch(strategy.Batch.FastBatch)
// 	}

// 	var failureThreshold *intstr.IntOrString
// 	var initialDelaySeconds int32

// 	if strategy.Batch.Toleration != nil {
// 		failureThreshold = strategy.Batch.Toleration.WorkloadFailureThreshold
// 		initialDelaySeconds = strategy.Batch.Toleration.InitialDelaySeconds
// 	}

// 	result := make([]types.BatchDetail, 0)
// 	for i, b := range batches {
// 		workloadTasks := make(map[string]*types.WorkloadBatchTask)

// 		filteredWorkloads := filterWorkloadsByMatch(workloadWrappers, b.Match)
// 		for _, w := range filteredWorkloads {
// 			wkey := fmt.Sprintf("%s/%s", w.GetCluster(), w.GetObj().GetName())
// 			task := &types.WorkloadBatchTask{
// 				Workload:            w,
// 				Replicas:            b.Replicas,
// 				FailureThreshold:    failureThreshold,
// 				InitialDelaySeconds: initialDelaySeconds,
// 			}
// 			workloadTasks[wkey] = task
// 		}

// 		tasks := []types.WorkloadBatchTask{}
// 		for _, task := range workloadTasks {
// 			tasks = append(tasks, *task)
// 		}

// 		batch := types.BatchDetail{
// 			Index:         i,
// 			WorkloadTasks: tasks,
// 		}

// 		if b.Pause != nil && *b.Pause {
// 			batch.NeedPause = true
// 		}

// 		result = append(result, batch)
// 	}
// 	return result
// }

func filterWorkloadsByMatch(workloads []workload.Interface, match *rolloutv1alpha1.ResourceMatch) []workload.Interface {
	if match == nil || (match.Selector == nil && len(match.Names) == 0) {
		// match all
		return workloads
	}
	result := make([]workload.Interface, 0)
	macher := workload.MatchAsMatcher(*match)
	for i := range workloads {
		w := workloads[i]
		info := w.GetInfo()
		if macher.Matches(info.Cluster, info.Name, info.Labels) {
			result = append(result, w)
		}
	}
	return result
}

func convertFastBatch(fastBatch *rolloutv1alpha1.FastBatch) []rolloutv1alpha1.RolloutStep {
	result := make([]rolloutv1alpha1.RolloutStep, 0)

	batchCount := fastBatch.Count
	per := int(math.Floor(float64(100) / float64(batchCount)))
	if per < 1 {
		per = 1
	}

	pausedBatches := getPausedBatches(batchCount, fastBatch.PausedBatches, fastBatch.PauseMode)

	if fastBatch.Beta != nil {
		result = append(result, rolloutv1alpha1.RolloutStep{
			Pause:    pointer.Bool(true),
			Replicas: *fastBatch.Beta.Replicas,
		})
	}

	// batch index starts from 1 to batchCount
	for i := int32(1); i <= batchCount; i++ {
		var replicas intstr.IntOrString
		if i == batchCount {
			replicas = intstr.FromString("100%")
		} else {
			replicas = intstr.FromString(fmt.Sprintf("%d%s", int(i)*per, "%"))
		}

		step := rolloutv1alpha1.RolloutStep{
			Replicas: replicas,
		}
		if pausedBatches.Has(i) {
			step.Pause = pointer.Bool(true)
		}
		result = append(result, step)
	}
	return result
}

func getPausedBatches(count int32, pausePoint []int32, strategy rolloutv1alpha1.PauseModeType) sets.Int32 {
	result := sets.NewInt32()
	if count < 1 {
		return result
	}
	for _, v := range pausePoint {
		if v > 0 && v < count {
			result.Insert(v)
		}
	}
	switch strategy {
	case rolloutv1alpha1.PauseModeTypeFirstBatch:
		result.Insert(1)
	case rolloutv1alpha1.PauseModeTypeEachBatch:
		for i := int32(1); i < count; i++ {
			result.Insert(i)
		}
	}
	return result
}

func constructRolloutRun(instance *rolloutv1alpha1.Rollout, strategy *rolloutv1alpha1.RolloutStrategy, workloadWrappers []workload.Interface, rolloutId string) *rolloutv1alpha1.RolloutRun {
	owner := metav1.NewControllerRef(instance, rolloutv1alpha1.SchemeGroupVersion.WithKind("Rollout"))
	run := &rolloutv1alpha1.RolloutRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: instance.Namespace,
			Name:      rolloutId,
			Labels: map[string]string{
				rollout.LabelControl:   "true",
				rollout.LabelCreatedBy: instance.Name,
			},
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec: rolloutv1alpha1.RolloutRunSpec{
			TargetType: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: instance.Spec.WorkloadRef.APIVersion,
				Kind:       instance.Spec.WorkloadRef.Kind,
			},
			Batch: rolloutv1alpha1.RolloutRunBatchStrategy{
				Toleration: strategy.Batch.Toleration,
				Batches:    constructRolloutRunBatches(strategy.Batch, workloadWrappers),
			},
			Webhooks: strategy.Webhooks,
		},
	}

	if features.DefaultFeatureGate.Enabled(features.OneTimeStrategy) {
		onetime := ontimestrategy.ConvertFrom(strategy)
		data := onetime.JSONData()
		run.Annotations[ontimestrategy.AnnoOneTimeStrategy] = string(data)
	}
	return run
}

func constructRolloutRunBatches(strategy *rolloutv1alpha1.BatchStrategy, workloadWrappers []workload.Interface) []rolloutv1alpha1.RolloutRunStep {
	if strategy == nil {
		return nil
	}
	batches := strategy.Batches
	if strategy.FastBatch != nil {
		batches = convertFastBatch(strategy.FastBatch)
	}

	if len(batches) == 0 {
		panic("no valid batches found in strategy")
	}

	result := make([]rolloutv1alpha1.RolloutRunStep, 0)
	for _, b := range batches {
		step := rolloutv1alpha1.RolloutRunStep{}
		targets := make([]rolloutv1alpha1.RolloutRunStepTarget, 0)
		filteredWorkloads := filterWorkloadsByMatch(workloadWrappers, b.Match)
		for _, w := range filteredWorkloads {
			info := w.GetInfo()
			target := rolloutv1alpha1.RolloutRunStepTarget{
				CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
					Cluster: info.Cluster,
					Name:    info.Name,
				},
				Replicas: b.Replicas,
			}
			targets = append(targets, target)
		}

		step.Targets = targets
		step.Pause = b.Pause
		step.Properties = b.Properties
		step.TrafficStrategy = b.TrafficStrategy
		result = append(result, step)
	}
	return result
}
