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
	"k8s.io/apimachinery/pkg/util/validation/field"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

func ValidateTrafficTopology(obj *rolloutv1alpha1.TrafficTopology) field.ErrorList {
	allErrs := apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	allErrs = append(allErrs, ValidateTrafficTopologySpec(&obj.Spec, field.NewPath("spec"))...)
	return allErrs
}

func ValidateTrafficTopologySpec(spec *rolloutv1alpha1.TrafficTopologySpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, ValidateResourceMatch(&spec.WorkloadRef.Match, fldPath.Child("workloadRef", "match"))...)

	switch spec.TrafficType {
	case rolloutv1alpha1.InClusterTrafficType, rolloutv1alpha1.MultiClusterTrafficType:
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("trafficType"), spec.TrafficType, "unsupported traffic type"))
	}

	return allErrs
}
