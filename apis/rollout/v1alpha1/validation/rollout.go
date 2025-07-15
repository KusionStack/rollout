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
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

type SupportedGVKFunc func(gvk schema.GroupVersionKind) bool

func ValidateRollout(rollout *rolloutv1alpha1.Rollout, isSupportedGVK SupportedGVKFunc) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMeta(&rollout.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	allErrs = append(allErrs, ValidateRolloutSpec(&rollout.Spec, field.NewPath("spec"), isSupportedGVK)...)

	return allErrs
}

func ValidateRolloutSpec(spec *rolloutv1alpha1.RolloutSpec, fldPath *field.Path, isSupportedGVK SupportedGVKFunc) field.ErrorList {
	allErrs := field.ErrorList{}

	switch spec.TriggerPolicy {
	case rolloutv1alpha1.AutoTriggerPolicy, rolloutv1alpha1.ManualTriggerPolicy:
	default:
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("triggerPolicy"), spec.TriggerPolicy, []string{string(rolloutv1alpha1.AutoTriggerPolicy), string(rolloutv1alpha1.ManualTriggerPolicy)}))
	}

	if len(spec.StrategyRef) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("strategyRef"), "must specify a strategy"))
	}

	allErrs = append(allErrs, ValidateWorkloadRef(&spec.WorkloadRef, fldPath.Child("workloadRef"), isSupportedGVK)...)

	return allErrs
}

func ValidateWorkloadRef(ref *rolloutv1alpha1.WorkloadRef, fldPath *field.Path, isSupportedGVK SupportedGVKFunc) field.ErrorList {
	allErrs := field.ErrorList{}

	gvk := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)

	if !isSupportedGVK(gvk) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), gvk, "unsupported workload kind"))
	}

	allErrs = append(allErrs, ValidateResourceMatch(&ref.Match, fldPath.Child("match"))...)

	return allErrs
}

func ValidateResourceMatch(match *rolloutv1alpha1.ResourceMatch, fldPath *field.Path) field.ErrorList {
	if match == nil {
		return nil
	}

	allErrs := field.ErrorList{}

	if match.Selector == nil && len(match.Names) == 0 {
		allErrs = append(allErrs, field.Required(fldPath, "must specify a selector or names"))
	}

	if match.Selector != nil {
		allErrs = append(allErrs, metav1validation.ValidateLabelSelector(match.Selector, fldPath.Child("selector"))...)
	}

	return allErrs
}

func ValidateRolloutUpdate(newObj, oldObj *rolloutv1alpha1.Rollout) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMetaUpdate(&newObj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))
	return allErrs
}
