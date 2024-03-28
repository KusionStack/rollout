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
	"strings"

	"github.com/elliotchance/pie/v2"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/names"
	"kusionstack.io/kube-utils/multicluster"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
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
	status.Conditions = []rolloutv1alpha1.Condition{}
}

func setStatusCondition(newStatus *rolloutv1alpha1.RolloutStatus, ctype rolloutv1alpha1.ConditionType, status metav1.ConditionStatus, reason, message string) {
	cond := condition.NewCondition(ctype, status, reason, message)
	newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *cond)
}

func filterWorkloadsByMatch(workloads []*workload.Info, match *rolloutv1alpha1.ResourceMatch) []*workload.Info {
	if match == nil || (match.Selector == nil && len(match.Names) == 0) {
		// match all
		return workloads
	}
	result := make([]*workload.Info, 0)
	macher := workload.MatchAsMatcher(*match)
	for i := range workloads {
		info := workloads[i]
		if macher.Matches(info.ClusterName, info.Name, info.Labels) {
			result = append(result, info)
		}
	}
	return result
}

func constructRolloutRun(obj *rolloutv1alpha1.Rollout, strategy *rolloutv1alpha1.RolloutStrategy, workloadWrappers []*workload.Info, rolloutId string) *rolloutv1alpha1.RolloutRun {
	owner := metav1.NewControllerRef(obj, rolloutv1alpha1.SchemeGroupVersion.WithKind("Rollout"))
	run := &rolloutv1alpha1.RolloutRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: obj.Namespace,
			Name:      rolloutId,
			Labels: map[string]string{
				rolloutapi.LabelControl:     "true",
				rolloutapi.LabelGeneratedBy: obj.Name,
			},
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec: rolloutv1alpha1.RolloutRunSpec{
			TargetType: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: obj.Spec.WorkloadRef.APIVersion,
				Kind:       obj.Spec.WorkloadRef.Kind,
			},
			TrafficTopologyRefs: obj.Spec.TrafficTopologyRefs,
			Canary:              constructRolloutRunCanary(strategy.Canary, workloadWrappers),
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

func constructRolloutRunCanary(strategy *rolloutv1alpha1.CanaryStrategy, workloadWrappers []*workload.Info) *rolloutv1alpha1.RolloutRunCanaryStrategy {
	if strategy == nil {
		return nil
	}
	targets := make([]rolloutv1alpha1.RolloutRunStepTarget, 0)
	filteredWorkloads := filterWorkloadsByMatch(workloadWrappers, strategy.Match)
	for _, info := range filteredWorkloads {
		target := rolloutv1alpha1.RolloutRunStepTarget{
			CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
				Cluster: info.ClusterName,
				Name:    info.Name,
			},
			Replicas: strategy.Replicas,
		}
		targets = append(targets, target)
	}

	step := &rolloutv1alpha1.RolloutRunCanaryStrategy{
		Targets:                  targets,
		Traffic:                  strategy.Traffic,
		Properties:               strategy.Properties,
		PodTemplateMetadataPatch: strategy.PodTemplateMetadataPatch,
	}
	return step
}

func constructRolloutRunBatches(strategy *rolloutv1alpha1.BatchStrategy, workloadWrappers []*workload.Info) []rolloutv1alpha1.RolloutRunStep {
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
		for _, info := range filteredWorkloads {
			target := rolloutv1alpha1.RolloutRunStepTarget{
				CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
					Cluster: info.ClusterName,
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

func getWatchableWorkloads(registry workload.Registry, logger logr.Logger, discoveryClient multicluster.PartialCachedDiscoveryInterface) []workload.Accessor {
	result := make([]workload.Accessor, 0)

	registry.Range(func(gvk schema.GroupVersionKind, item workload.Accessor) bool {
		if !item.Watchable() {
			// skip it
			logger.Info("workload interface does not support watch, skip it", "gvk", gvk.String())
			return true
		}

		supported, err := isGVKSupportedInMembers(discoveryClient, gvk)
		if err != nil {
			logger.Error(err, "failed to get discovery result from member clusters, skip it", "gvk", gvk.String())
			return true
		}
		if !supported {
			logger.Info("gvk is not supported in all members clusters, skip it", "gvk", gvk.String())
			return true
		}

		result = append(result, item)

		return true
	})
	return result
}

func isGVKSupportedInMembers(discoveryClient multicluster.PartialCachedDiscoveryInterface, gvk schema.GroupVersionKind) (bool, error) {
	if discoveryClient == nil {
		return false, fmt.Errorf("member clusters discovery interface is not set, please use SetupWithManager() firstly")
	}

	_, resources, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return false, err
	}

	for _, resourceList := range resources {
		if resourceList.GroupVersion != gvk.GroupVersion().String() {
			continue
		}
		found := pie.Any(resourceList.APIResources, func(value metav1.APIResource) bool {
			return value.Kind == gvk.Kind
		})
		if found {
			return true, nil
		}
	}
	return false, nil
}
