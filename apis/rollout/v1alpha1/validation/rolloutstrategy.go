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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	appsvalidation "k8s.io/kubernetes/pkg/apis/apps/validation"

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

func ValidateRolloutStrategy(obj *rolloutv1alpha1.RolloutStrategy) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))

	if obj.Canary != nil && obj.Batch == nil {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("canary"), "cannot set canary independently"))
	}

	allErrs = append(allErrs, ValidateBatchStrategy(obj.Batch, field.NewPath("batch"))...)
	allErrs = append(allErrs, ValidateCanaryStrategy(obj.Canary, field.NewPath("canary"))...)
	allErrs = append(allErrs, ValidateWebhooks(obj.Webhooks, field.NewPath("webhooks"))...)

	return allErrs
}

func ValidateBatchStrategy(strategy *rolloutv1alpha1.BatchStrategy, fldPath *field.Path) field.ErrorList {
	if strategy == nil {
		return nil
	}

	allErrs := field.ErrorList{}

	if len(strategy.Batches) == 0 {
		return append(allErrs, field.Required(fldPath.Child("batches"), "must have at least one batch"))
	}

	for i := range strategy.Batches {
		batch := strategy.Batches[i]
		allErrs = append(allErrs, ValidateRolloutStep(&batch, fldPath.Child("batches").Index(i))...)
	}

	return allErrs
}

func ValidateRolloutStep(step *rolloutv1alpha1.RolloutStep, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(step.Replicas, fldPath.Child("replicas"))...)
	allErrs = append(allErrs, ValidateResourceMatch(step.Match, fldPath.Child("matchTargets"))...)
	allErrs = append(allErrs, validateTrafficStrategy(step.Traffic, fldPath.Child("traffic"))...)

	return allErrs
}

func ValidateCanaryStrategy(strategy *rolloutv1alpha1.CanaryStrategy, fldPath *field.Path) field.ErrorList {
	if strategy == nil {
		return nil
	}

	allErrs := field.ErrorList{}

	allErrs = append(allErrs, appsvalidation.ValidatePositiveIntOrPercent(strategy.Replicas, fldPath.Child("replicas"))...)
	allErrs = append(allErrs, ValidateResourceMatch(strategy.Match, fldPath.Child("matchTargets"))...)
	allErrs = append(allErrs, validateTemplateMetadataPatch(strategy.TemplateMetadataPatch, fldPath.Child("patch"))...)
	allErrs = append(allErrs, validateTrafficStrategy(strategy.Traffic, fldPath.Child("traffic"))...)

	return allErrs
}

func validateTemplateMetadataPatch(patch *rolloutv1alpha1.MetadataPatch, fldPath *field.Path) field.ErrorList {
	if patch == nil {
		return nil
	}

	allErrs := field.ErrorList{}

	if len(patch.Annotations) > 0 {
		allErrs = append(allErrs, apimachineryvalidation.ValidateAnnotations(patch.Annotations, fldPath.Child("annotations"))...)
	}

	if len(patch.Labels) > 0 {
		allErrs = append(allErrs, metav1validation.ValidateLabels(patch.Labels, fldPath.Child("labels"))...)
	}

	return allErrs
}

func ValidateWebhooks(webhooks []rolloutv1alpha1.RolloutWebhook, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	nameSet := sets.String{}

	for i := range webhooks {
		webhook := webhooks[i]

		// check duplicate webhook name
		if nameSet.Has(webhook.Name) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Child("name"), webhook.Name))
		}

		allErrs = append(allErrs, ValidateRolloutWebhook(&webhook, field.NewPath("webhooks").Index(i))...)
		nameSet.Insert(webhook.Name)
	}

	return allErrs
}

func ValidateRolloutWebhook(webhook *rolloutv1alpha1.RolloutWebhook, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(webhook.HookTypes) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("hookTypes"), "must specify at least one hook type"))
	}

	allErrs = append(allErrs, ValidateWebhookURL(fldPath.Child("url"), webhook.ClientConfig.URL, false)...)

	return allErrs
}

func validateTrafficStrategy(traffic *rolloutv1alpha1.TrafficStrategy, fldPath *field.Path) field.ErrorList {
	if traffic == nil {
		return nil
	}
	allErrs := field.ErrorList{}

	if traffic.HTTP != nil {
		if traffic.HTTP.Weight != nil {
			if len(traffic.HTTP.Matches) > 0 {
				allErrs = append(allErrs, field.Forbidden(fldPath, "weight and http rule matches cannot be specified together"))
			}
			if traffic.HTTP.BaseTraffic != nil {
				allErrs = append(allErrs, field.Forbidden(fldPath, "weight and base traffic cannot be specified together"))
			}
		}
	}

	return allErrs
}
