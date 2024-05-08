/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package validation

import (
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

func ValidateRolloutRun(obj *rolloutv1alpha1.RolloutRun) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	allErrs = append(allErrs, ValidateRolloutRunSpec(&obj.Spec, field.NewPath("spec"))...)

	return allErrs
}

func ValidateRolloutRunSpec(spec *rolloutv1alpha1.RolloutRunSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, ValidateWebhooks(spec.Webhooks, fldPath.Child("webhooks"))...)

	if spec.Canary != nil && spec.Batch == nil {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("canary"), "cannot set canary independently"))
	}

	allErrs = append(allErrs, ValidateRolloutRunCanaryStrategy(spec.Canary, fldPath.Child("canary"))...)
	allErrs = append(allErrs, ValidateRolloutRunBatchStrategy(spec.Batch, fldPath.Child("batch"))...)

	return allErrs
}

func ValidateRolloutRunCanaryStrategy(canary *rolloutv1alpha1.RolloutRunCanaryStrategy, fldPath *field.Path) field.ErrorList {
	if canary == nil {
		return nil
	}

	var allErrs field.ErrorList

	// validate targets
	allErrs = append(allErrs, validateRolloutRunStepTargets(canary.Targets, fldPath.Child("targets"))...)
	// validate pod template metadata path
	allErrs = append(allErrs, validatePodTemplatePatch(canary.PodTemplateMetadataPatch, fldPath.Child("podTemplateMetadataPath"))...)
	// validate traffic
	allErrs = append(allErrs, validateTrafficStrategy(canary.Traffic, fldPath.Child("traffic"))...)

	return allErrs
}

func ValidateRolloutRunBatchStrategy(batch *rolloutv1alpha1.RolloutRunBatchStrategy, fldPath *field.Path) field.ErrorList {
	if batch == nil {
		return nil
	}

	var allErrs field.ErrorList

	if len(batch.Batches) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("batches"), "must specify at least one batch"))
	}

	for i := range batch.Batches {
		step := batch.Batches[i]
		allErrs = append(allErrs, validateRolloutRunStep(&step, fldPath.Index(i))...)
	}

	return allErrs
}

func validateRolloutRunStep(step *rolloutv1alpha1.RolloutRunStep, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, validateRolloutRunStepTargets(step.Targets, fldPath.Child("targets"))...)
	// validate traffic
	allErrs = append(allErrs, validateTrafficStrategy(step.Traffic, fldPath.Child("traffic"))...)
	return allErrs
}

func validateRolloutRunStepTargets(targets []rolloutv1alpha1.RolloutRunStepTarget, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if len(targets) == 0 {
		allErrs = append(allErrs, field.Required(fldPath, "must specify at least one target"))
	}

	targetMap := map[rolloutv1alpha1.CrossClusterObjectNameReference]bool{}
	for i, target := range targets {
		allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(target.Replicas, fldPath.Index(i).Child("replicas"))...)
		if len(target.Name) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Index(i).Child("name"), "name is required"))
		}
		if _, ok := targetMap[target.CrossClusterObjectNameReference]; ok {
			allErrs = append(allErrs, field.Duplicate(fldPath.Index(i).Child("name"), target.CrossClusterObjectNameReference))
		}
		targetMap[target.CrossClusterObjectNameReference] = true
	}

	return allErrs
}

func ValidateRolloutRunUpdate(newObj, oldObj *rolloutv1alpha1.RolloutRun) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMetaUpdate(&newObj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))

	// immutable fields
	if !apiequality.Semantic.DeepEqual(newObj.Spec.TargetType, oldObj.Spec.TargetType) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("targetType"), "targetType is immutable"))
	}
	if !apiequality.Semantic.DeepEqual(newObj.Spec.Webhooks, oldObj.Spec.Webhooks) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("webhooks"), "webhooks is immutable"))
	}
	if !apiequality.Semantic.DeepEqual(newObj.Spec.TrafficTopologyRefs, oldObj.Spec.TrafficTopologyRefs) {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("trafficTopologyRefs"), "trafficTopologyRefs is immutable"))
	}

	if (oldObj.Spec.Canary == nil) != (newObj.Spec.Canary == nil) {
		// canary is muated
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("canary"), "canary is immutable"))
	} else if oldObj.Spec.Canary != nil && newObj.Spec.Canary != nil {
		// pod template metadata patch is immutable
		if !apiequality.Semantic.DeepEqual(newObj.Spec.Canary.PodTemplateMetadataPatch, oldObj.Spec.Canary.PodTemplateMetadataPatch) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("canary").Child("podTemplateMetadataPatch"), "podTemplateMetadataPatch is immutable"))
		}

		// check orthers immutable fields according to current state
		beforeRunning := true
		if newObj.Status.CanaryStatus != nil &&
			newObj.Status.CanaryStatus.State != rolloutv1alpha1.RolloutStepNone &&
			newObj.Status.CanaryStatus.State != rolloutv1alpha1.RolloutStepPending {
			beforeRunning = false
		}

		if !beforeRunning && !apiequality.Semantic.DeepEqual(newObj.Spec.Canary, oldObj.Spec.Canary) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("canary"), "canary is immutable after running"))
		}
	}

	if (oldObj.Spec.Batch == nil) != (newObj.Spec.Batch == nil) {
		// batch is muated
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("batch"), "batch is immutable"))
	} else if oldObj.Spec.Batch != nil && newObj.Spec.Batch != nil {
		// check orthers immutable fields according to current state
		immutableBatchIndex := -1
		currentBatchIndex := -1
		if newObj.Status.BatchStatus != nil {
			currentBatchIndex = int(newObj.Status.BatchStatus.CurrentBatchIndex)
			if newObj.Status.BatchStatus.CurrentBatchState == rolloutv1alpha1.RolloutStepNone ||
				newObj.Status.BatchStatus.CurrentBatchState == rolloutv1alpha1.RolloutStepPending {
				// current batch is not running yet, it can be mutated
				immutableBatchIndex = int(newObj.Status.BatchStatus.CurrentBatchIndex - 1)
			} else {
				immutableBatchIndex = int(newObj.Status.BatchStatus.CurrentBatchIndex)
			}
		}

		// check if new batch size count is less than current running batch
		newBatchCount := len(newObj.Spec.Batch.Batches)
		if newBatchCount < currentBatchIndex+1 {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "batch", "batches"), "batches count is less then currently running batch"))
		} else {
			// check immutable fields
			for i := 0; i <= immutableBatchIndex; i++ {
				if !apiequality.Semantic.DeepEqual(newObj.Spec.Batch.Batches[i], oldObj.Spec.Batch.Batches[i]) {
					allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "batch", "batches").Index(i), "batch is immutable after running"))
				}
			}
			// current batch's breakpoint is immutable
			if immutableBatchIndex < currentBatchIndex &&
				oldObj.Spec.Batch.Batches[currentBatchIndex].Breakpoint != newObj.Spec.Batch.Batches[currentBatchIndex].Breakpoint {
				allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "batch", "batches").Index(currentBatchIndex).Child("breakpoint"), "breakpoint in current batch is immutable"))
			}
		}
	}

	return allErrs
}
