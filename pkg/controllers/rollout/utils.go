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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/storage/names"

	"kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/apis/rollout/v1alpha1/condition"
	"kusionstack.io/rollout/pkg/features"
	"kusionstack.io/rollout/pkg/features/ontimestrategy"
	"kusionstack.io/rollout/pkg/workload"
)

func generateRolloutID(name string) string {
	prefix := name
	if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}
	return names.SimpleNameGenerator.GenerateName(prefix)
}

func resetRolloutStatus(status *rolloutv1alpha1.RolloutStatus, rolloutID string, phase rolloutv1alpha1.RolloutPhase) {
	// clean all existing status
	status.RolloutID = rolloutID
	status.Phase = phase
	status.BatchStatus = nil
	status.Conditions = []rolloutv1alpha1.Condition{}
}

func setStatusCondition(newStatus *rolloutv1alpha1.RolloutStatus, ctype rolloutv1alpha1.ConditionType, status metav1.ConditionStatus, reason, message string) {
	cond := condition.NewCondition(ctype, status, reason, message)
	newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *cond)
}

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
			Batch: &rolloutv1alpha1.RolloutRunBatchStrategy{
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
		step.Breakpoint = b.Breakpoint
		step.Properties = b.Properties
		step.Traffic = b.Traffic
		result = append(result, step)
	}
	return result
}
