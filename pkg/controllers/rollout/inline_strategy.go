// Copyright 2025 The KusionStack Authors
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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"

	"kusionstack.io/rollout/pkg/features"
	"kusionstack.io/rollout/pkg/features/ontimestrategy"
	"kusionstack.io/rollout/pkg/workload"
)

// constructRolloutRunFromInlineStrategy constructs RolloutRun from inline strategy
// Returns the constructed RolloutRun, a boolean indicating if inline strategy was used, and an error if validation fails
func constructRolloutRunFromInlineStrategy(
	obj *rolloutv1alpha1.Rollout,
	workloadWrappers []*workload.Info,
	rolloutId string,
) (*rolloutv1alpha1.RolloutRun, bool, error) {
	if obj.Spec.BatchStrategy == nil {
		return nil, false, nil
	}

	// Build workload map for validation
	workloadMap := buildWorkloadMap(workloadWrappers)

	owner := metav1.NewControllerRef(obj, rolloutv1alpha1.SchemeGroupVersion.WithKind("Rollout"))
	run := &rolloutv1alpha1.RolloutRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       obj.Namespace,
			Name:            rolloutId,
			Labels:          map[string]string{},
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{*owner},
			Finalizers:      []string{rolloutapi.FinalizerRolloutProtection},
		},
		Spec: rolloutv1alpha1.RolloutRunSpec{
			TargetType: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: obj.Spec.WorkloadRef.APIVersion,
				Kind:       obj.Spec.WorkloadRef.Kind,
			},
			TrafficTopologyRefs: obj.Spec.TrafficTopologyRefs,
			Webhooks:            []rolloutv1alpha1.RolloutWebhook{}, // Empty for inline strategy
		},
	}

	if obj.Spec.CanaryStrategy != nil {
		canary, err := validateAndCopyCanaryStrategy(obj.Spec.CanaryStrategy, workloadMap)
		if err != nil {
			return nil, true, err
		}
		run.Spec.Canary = canary
	}

	batch, err := validateAndCopyBatchStrategy(obj.Spec.BatchStrategy, workloadMap)
	if err != nil {
		return nil, true, err
	}
	run.Spec.Batch = batch

	// Set OneTimeStrategy annotation for inline batch strategy
	if features.DefaultFeatureGate.Enabled(features.OneTimeStrategy) {
		onetime := ontimestrategy.ConvertFromInline(run.Spec.Batch)
		data := onetime.JSONData()
		run.Annotations[ontimestrategy.AnnoOneTimeStrategy] = string(data)
	}

	if features.DefaultFeatureGate.Enabled(features.RolloutClassPredicate) {
		class, ok := obj.Labels[rolloutapi.LabelRolloutClass]
		if ok {
			run.Labels[rolloutapi.LabelRolloutClass] = class
		}
	}

	return run, true, nil
}

// validateAndCopyCanaryStrategy validates targets and creates a copy for RolloutRun
// For inline strategy, targets are already pre-resolved by user
// Returns error if any target is not found in workloadMap
func validateAndCopyCanaryStrategy(
	canary *rolloutv1alpha1.RolloutRunCanaryStrategy,
	workloadMap map[string]*workload.Info,
) (*rolloutv1alpha1.RolloutRunCanaryStrategy, error) {
	if canary == nil {
		return nil, nil
	}

	// Validate all targets exist
	validatedTargets := make([]rolloutv1alpha1.RolloutRunStepTarget, 0, len(canary.Targets))
	for _, target := range canary.Targets {
		key := workloadKey(target.Cluster, target.Name)
		if _, exists := workloadMap[key]; !exists {
			return nil, fmt.Errorf("canary target cluster=%s name=%s not found in workloads", target.Cluster, target.Name)
		}
		// Direct copy - targets are already in correct format
		validatedTargets = append(validatedTargets, target)
	}

	return &rolloutv1alpha1.RolloutRunCanaryStrategy{
		Targets:               validatedTargets,
		Traffic:               canary.Traffic,
		Properties:            canary.Properties,
		TemplateMetadataPatch: canary.TemplateMetadataPatch,
	}, nil
}

// validateAndCopyBatchStrategy validates and creates a copy for RolloutRun
// For inline strategy, targets are already pre-resolved by user
// Returns error if any target is not found in workloadMap
func validateAndCopyBatchStrategy(
	batch *rolloutv1alpha1.RolloutRunBatchStrategy,
	workloadMap map[string]*workload.Info,
) (*rolloutv1alpha1.RolloutRunBatchStrategy, error) {
	if batch == nil {
		return nil, nil
	}

	if len(batch.Batches) == 0 {
		// Return as-is if no batches defined
		return &rolloutv1alpha1.RolloutRunBatchStrategy{
			Toleration: batch.Toleration,
			Batches:    []rolloutv1alpha1.RolloutRunStep{},
		}, nil
	}

	validatedBatches := make([]rolloutv1alpha1.RolloutRunStep, 0, len(batch.Batches))

	for batchIdx, step := range batch.Batches {
		if len(step.Targets) == 0 {
			// Skip steps without targets
			continue
		}

		// Validate all targets exist
		validatedTargets := make([]rolloutv1alpha1.RolloutRunStepTarget, 0, len(step.Targets))
		for _, target := range step.Targets {
			key := workloadKey(target.Cluster, target.Name)
			if _, exists := workloadMap[key]; !exists {
				return nil, fmt.Errorf("batch[%d] target cluster=%s name=%s not found in workloads", batchIdx, target.Cluster, target.Name)
			}
			// Direct copy - targets are already in correct format
			validatedTargets = append(validatedTargets, target)
		}

		if len(validatedTargets) > 0 {
			validatedStep := rolloutv1alpha1.RolloutRunStep{
				Targets:    validatedTargets,
				Traffic:    step.Traffic,
				Breakpoint: step.Breakpoint,
				Properties: step.Properties,
			}
			validatedBatches = append(validatedBatches, validatedStep)
		}
	}

	return &rolloutv1alpha1.RolloutRunBatchStrategy{
		Toleration: batch.Toleration,
		Batches:    validatedBatches,
	}, nil
}

// Helper functions

func buildWorkloadMap(workloads []*workload.Info) map[string]*workload.Info {
	m := make(map[string]*workload.Info)
	for _, wl := range workloads {
		key := workloadKey(wl.ClusterName, wl.Name)
		m[key] = wl
	}
	return m
}

func workloadKey(cluster, name string) string {
	return cluster + "/" + name
}
