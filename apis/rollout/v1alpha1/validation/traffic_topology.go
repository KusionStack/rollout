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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

func ValidateTrafficTopology(
	obj *rolloutv1alpha1.TrafficTopology,
	isSupportedWorkload, isSupportedRoute, isSupportedBackend SupportedGVKFunc,
) field.ErrorList {
	// check metadata
	allErrs := apimachineryvalidation.ValidateObjectMeta(&obj.ObjectMeta, true, apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))
	// check spec
	allErrs = append(allErrs, ValidateTrafficTopologySpec(&obj.Spec, field.NewPath("spec"), isSupportedWorkload, isSupportedRoute, isSupportedBackend)...)
	return allErrs
}

func ValidateTrafficTopologySpec(
	spec *rolloutv1alpha1.TrafficTopologySpec,
	fldPath *field.Path,
	isSupportedWorkload, isSupportedRoute, isSupportedBackend SupportedGVKFunc,
) field.ErrorList {
	allErrs := field.ErrorList{}

	// check workloadRef
	allErrs = append(allErrs, ValidateWorkloadRef(&spec.WorkloadRef, fldPath.Child("workloadRef"), isSupportedWorkload)...)

	// check type
	switch spec.TrafficType {
	case rolloutv1alpha1.InClusterTrafficType, rolloutv1alpha1.MultiClusterTrafficType:
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("trafficType"), spec.TrafficType, "unsupported traffic type"))
	}

	// check backend
	allErrs = append(allErrs, ValidateBackendRef(spec.Backend, fldPath.Child("backend"), isSupportedBackend)...)
	// check routes
	allErrs = append(allErrs, ValidateRoutes(spec.Routes, fldPath.Child("routes"), isSupportedRoute)...)

	return allErrs
}

func ValidateBackendRef(ref rolloutv1alpha1.BackendRef, fldPath *field.Path, isSupportedBackend SupportedGVKFunc) field.ErrorList {
	allErrs := field.ErrorList{}

	gvk := schema.FromAPIVersionAndKind(ptr.Deref(ref.APIVersion, "v1"), ptr.Deref(ref.Kind, "Service"))
	if !isSupportedBackend(gvk) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), ref, "unsupported backend gvk"))
	}
	if len(ref.Name) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), ""))
	}
	return allErrs
}

func ValidateRoutes(routes []rolloutv1alpha1.RouteRef, fldPath *field.Path, isSupportedRoute SupportedGVKFunc) field.ErrorList {
	allErrs := field.ErrorList{}

	routeGVKNames := sets.NewString()
	for i, route := range routes {
		gvk := schema.FromAPIVersionAndKind(ptr.Deref(route.APIVersion, "gateway.networking.k8s.io/v1"), ptr.Deref(route.Kind, "HTTPRoute"))
		key := fmt.Sprintf("%s, Name=%s", gvk.String(), route.Name)
		// check supported
		if !isSupportedRoute(gvk) {
			allErrs = append(allErrs, field.Invalid(fldPath.Index(i).Child("kind"), route, "unsupported route gvk"))
		}
		if len(route.Name) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Index(i).Child("name"), ""))
		}
		// check duplicate
		if routeGVKNames.Has(key) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Index(i).Child("name"), route.Name))
		}
		routeGVKNames.Insert(key)
	}
	return allErrs
}
