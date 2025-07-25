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
	"fmt"

	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	v1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

func ValidateBackendRouting(
	obj *rolloutv1alpha1.BackendRouting,
	isSupportedRoute, isSupportedBackend SupportedGVKFunc,
) field.ErrorList {
	// validate metadata
	allErrs := apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	// validate spec
	allErrs = append(allErrs, ValidateBackendRoutingSpec(&obj.Spec, field.NewPath("spec"), isSupportedRoute, isSupportedBackend)...)
	return allErrs
}

func ValidateBackendRoutingSpec(
	spec *rolloutv1alpha1.BackendRoutingSpec,
	fldPath *field.Path,
	isSupportedRoute, isSupportedBackend SupportedGVKFunc,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// check type
	switch spec.TrafficType {
	case rolloutv1alpha1.InClusterTrafficType, rolloutv1alpha1.MultiClusterTrafficType:
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("trafficType"), spec.TrafficType, "unsupported traffic type"))
	}

	// validate backend
	allErrs = append(allErrs, validateCrossClusterObjectReference(spec.Backend, fldPath.Child("backend"), isSupportedBackend)...)
	// validate routes
	allErrs = append(allErrs, ValidateBackendRoutingRoutes(spec.Routes, fldPath.Child("routes"), isSupportedRoute)...)
	// validate ForkedBackends
	allErrs = append(allErrs, ValidateForkBackend(spec.ForkedBackends, fldPath.Child("forkedBackends"))...)
	if spec.Forwarding != nil && spec.ForkedBackends == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("forkedBackends"), " forkedBackends is required when forwarding is set"))
	}
	allErrs = append(allErrs, ValidateForwording(spec.Forwarding, fldPath.Child("forwarding"))...)

	return allErrs
}

func ValidateBackendRoutingRoutes(routes []rolloutv1alpha1.CrossClusterObjectReference, fldPath *field.Path, isSupportedGVK SupportedGVKFunc) field.ErrorList {
	allErrs := field.ErrorList{}
	routeGVKNames := sets.NewString()

	for i, route := range routes {
		allErrs = append(allErrs, validateCrossClusterObjectReference(route, fldPath.Index(i), isSupportedGVK)...)
		key := fmt.Sprintf("%s/%s/%s", route.APIVersion, route.Kind, route.Name)
		if routeGVKNames.Has(key) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Index(i).Child("name"), route.Name))
		}
		routeGVKNames.Insert(key)
	}
	return allErrs
}

func validateCrossClusterObjectReference(ref rolloutv1alpha1.CrossClusterObjectReference, fldPath *field.Path, isSupportedGVK SupportedGVKFunc) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(ref.APIVersion) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("apiVersion"), ""))
	}
	if len(ref.Kind) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("kind"), ""))
	}
	if len(ref.APIVersion) > 0 && len(ref.Kind) > 0 {
		gvk := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
		if !isSupportedGVK(gvk) {
			allErrs = append(allErrs, field.Invalid(fldPath, gvk.String(), "unsupported gvk"))
		}
	}
	if len(ref.Name) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), ""))
	}
	return allErrs
}

func ValidateForkBackend(foredBackend *rolloutv1alpha1.ForkedBackends, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if foredBackend == nil {
		return allErrs
	}
	if len(foredBackend.Canary.Name) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("canary").Child("name"), ""))
	}

	allErrs = append(allErrs, v1validation.ValidateLabels(foredBackend.Canary.ExtraLabelSelector, fldPath.Child("canary").Child("extraLabelSelector"))...)

	if len(foredBackend.Stable.Name) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("stable").Child("name"), ""))
	}

	allErrs = append(allErrs, v1validation.ValidateLabels(foredBackend.Stable.ExtraLabelSelector, fldPath.Child("stable").Child("extraLabelSelector"))...)
	return allErrs
}

func ValidateForwording(forwarding *rolloutv1alpha1.BackendForwarding, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if forwarding == nil {
		return allErrs
	}
	allErrs = append(allErrs, ValidateForwardingHTTP(forwarding.HTTP, fldPath.Child("http"))...)
	return allErrs
}

func ValidateForwardingHTTP(httpSpec *rolloutv1alpha1.HTTPForwarding, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if httpSpec == nil {
		return allErrs
	}
	if httpSpec.Origin == nil && (httpSpec.Stable != nil || httpSpec.Canary != nil) {
		allErrs = append(allErrs, field.Required(fldPath.Child("origin"), "origin is required when stable or canary is set"))
		return allErrs
	}
	return allErrs
}
