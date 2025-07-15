/**
 * Copyright 2023 The KusionStack Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

import (
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type HTTPRouteMatch struct {
	// Path specifies a HTTP request path matcher.
	//
	// +optional
	Path *gatewayapiv1.HTTPPathMatch `json:"path,omitempty"`

	// Headers specifies HTTP request header matchers. Multiple match values are
	// ANDed together, meaning, a request must match all the specified headers
	// to select the route.
	//
	// +listType=map
	// +listMapKey=name
	// +optional
	// +kubebuilder:validation:MaxItems=16
	Headers []gatewayapiv1.HTTPHeaderMatch `json:"headers,omitempty"`
	// QueryParams specifies HTTP query parameter matchers. Multiple match
	// values are ANDed together, meaning, a request must match all the
	// specified query parameters to select the route.
	//
	// Support: Extended
	//
	// +listType=map
	// +listMapKey=name
	// +optional
	// +kubebuilder:validation:MaxItems=16
	QueryParams []gatewayapiv1.HTTPQueryParamMatch `json:"queryParams,omitempty"`
}

type HTTPRouteRule struct {
	// Matches define conditions used for matching the rule against incoming
	// HTTP requests. Each match is independent, i.e. this rule will be matched
	// if **any** one of the matches is satisfied.
	//
	// For example, take the following matches configuration:
	//
	// ```
	// matches:
	// - path:
	//     value: "/foo"
	//   headers:
	//   - name: "version"
	//     value: "v2"
	// - path:
	//     value: "/v2/foo"
	// ```
	//
	// For a request to match against this rule, a request must satisfy
	// EITHER of the two conditions:
	//
	// - path prefixed with `/foo` AND contains the header `version: v2`
	// - path prefix of `/v2/foo`
	//
	// See the documentation for HTTPRouteMatch on how to specify multiple
	// match conditions that should be ANDed together.
	//
	// If no matches are specified, the default is a prefix
	// path match on "/", which has the effect of matching every
	// HTTP request.
	//
	// Proxy or Load Balancer routing configuration generated from HTTPRoutes
	// MUST prioritize matches based on the following criteria, continuing on
	// ties. Across all rules specified on applicable Routes, precedence must be
	// given to the match having:
	//
	// * "Exact" path match.
	// * "Prefix" path match with largest number of characters.
	// * Method match.
	// * Largest number of header matches.
	// * Largest number of query param matches.
	//
	// Note: The precedence of RegularExpression path matches are implementation-specific.
	//
	// If ties still exist across multiple Routes, matching precedence MUST be
	// determined in order of the following criteria, continuing on ties:
	//
	// * The oldest Route based on creation timestamp.
	// * The Route appearing first in alphabetical order by
	//   "{namespace}/{name}".
	//
	// If ties still exist within an HTTPRoute, matching precedence MUST be granted
	// to the FIRST matching rule (in list order) with a match meeting the above
	// criteria.
	//
	// When no rules matching a request have been successfully attached to the
	// parent a request is coming from, a HTTP 404 status code MUST be returned.
	//
	// +optional
	// +kubebuilder:validation:MaxItems=8
	Matches []HTTPRouteMatch `json:"matches,omitempty"`
	// Filters define the filters that are applied to requests that match
	// this rule.
	//
	// The effects of ordering of multiple behaviors are currently unspecified.
	// This can change in the future based on feedback during the alpha stage.
	//
	// Conformance-levels at this level are defined based on the type of filter:
	//
	// - ALL core filters MUST be supported by all implementations.
	// - Implementers are encouraged to support extended filters.
	// - Implementation-specific custom filters have no API guarantees across
	//   implementations.
	//
	// Specifying the same filter multiple times is not supported unless explicitly
	// indicated in the filter.
	//
	// All filters are expected to be compatible with each other except for the
	// URLRewrite and RequestRedirect filters, which may not be combined. If an
	// implementation can not support other combinations of filters, they must clearly
	// document that limitation. In cases where incompatible or unsupported
	// filters are specified and cause the `Accepted` condition to be set to status
	// `False`, implementations may use the `IncompatibleFilters` reason to specify
	// this configuration error.
	//
	// Support: Core
	//
	// +optional
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:message="May specify either httpRouteFilterRequestRedirect or httpRouteFilterRequestRewrite, but not both",rule="!(self.exists(f, f.type == 'RequestRedirect') && self.exists(f, f.type == 'URLRewrite'))"
	// +kubebuilder:validation:XValidation:message="RequestHeaderModifier filter cannot be repeated",rule="self.filter(f, f.type == 'RequestHeaderModifier').size() <= 1"
	// +kubebuilder:validation:XValidation:message="ResponseHeaderModifier filter cannot be repeated",rule="self.filter(f, f.type == 'ResponseHeaderModifier').size() <= 1"
	// +kubebuilder:validation:XValidation:message="RequestRedirect filter cannot be repeated",rule="self.filter(f, f.type == 'RequestRedirect').size() <= 1"
	// +kubebuilder:validation:XValidation:message="URLRewrite filter cannot be repeated",rule="self.filter(f, f.type == 'URLRewrite').size() <= 1"
	Filters []gatewayapiv1.HTTPRouteFilter `json:"filters,omitempty"`
}

// type BaseHTTPRouteRule struct {
// 	// Matches define conditions used for matching the rule against incoming
// 	// HTTP requests. Each match is independent, i.e. this rule will be matched
// 	// if **any** one of the matches is satisfied.
// 	//
// 	// For example, take the following matches configuration:
// 	//
// 	// ```
// 	// matches:
// 	// - path:
// 	//     value: "/foo"
// 	//   headers:
// 	//   - name: "version"
// 	//     value: "v2"
// 	// - path:
// 	//     value: "/v2/foo"
// 	// ```
// 	//
// 	// For a request to match against this rule, a request must satisfy
// 	// EITHER of the two conditions:
// 	//
// 	// - path prefixed with `/foo` AND contains the header `version: v2`
// 	// - path prefix of `/v2/foo`
// 	//
// 	// See the documentation for HTTPRouteMatch on how to specify multiple
// 	// match conditions that should be ANDed together.
// 	//
// 	// If no matches are specified, the default is a prefix
// 	// path match on "/", which has the effect of matching every
// 	// HTTP request.
// 	//
// 	// Proxy or Load Balancer routing configuration generated from HTTPRoutes
// 	// MUST prioritize matches based on the following criteria, continuing on
// 	// ties. Across all rules specified on applicable Routes, precedence must be
// 	// given to the match having:
// 	//
// 	// * "Exact" path match.
// 	// * "Prefix" path match with largest number of characters.
// 	// * Method match.
// 	// * Largest number of header matches.
// 	// * Largest number of query param matches.
// 	//
// 	// Note: The precedence of RegularExpression path matches are implementation-specific.
// 	//
// 	// If ties still exist across multiple Routes, matching precedence MUST be
// 	// determined in order of the following criteria, continuing on ties:
// 	//
// 	// * The oldest Route based on creation timestamp.
// 	// * The Route appearing first in alphabetical order by
// 	//   "{namespace}/{name}".
// 	//
// 	// If ties still exist within an HTTPRoute, matching precedence MUST be granted
// 	// to the FIRST matching rule (in list order) with a match meeting the above
// 	// criteria.
// 	//
// 	// When no rules matching a request have been successfully attached to the
// 	// parent a request is coming from, a HTTP 404 status code MUST be returned.
// 	//
// 	// +optional
// 	// +kubebuilder:validation:MaxItems=8
// 	Matches []HTTPRouteMatch `json:"matches,omitempty"`
// 	// Filters define the filters that are applied to requests that match
// 	// this rule.
// 	//
// 	// The effects of ordering of multiple behaviors are currently unspecified.
// 	// This can change in the future based on feedback during the alpha stage.
// 	//
// 	// Conformance-levels at this level are defined based on the type of filter:
// 	//
// 	// - ALL core filters MUST be supported by all implementations.
// 	// - Implementers are encouraged to support extended filters.
// 	// - Implementation-specific custom filters have no API guarantees across
// 	//   implementations.
// 	//
// 	// Specifying the same filter multiple times is not supported unless explicitly
// 	// indicated in the filter.
// 	//
// 	// All filters are expected to be compatible with each other except for the
// 	// URLRewrite and RequestRedirect filters, which may not be combined. If an
// 	// implementation can not support other combinations of filters, they must clearly
// 	// document that limitation. In cases where incompatible or unsupported
// 	// filters are specified and cause the `Accepted` condition to be set to status
// 	// `False`, implementations may use the `IncompatibleFilters` reason to specify
// 	// this configuration error.
// 	//
// 	// Support: Core
// 	//
// 	// +optional
// 	// +kubebuilder:validation:MaxItems=16
// 	// +kubebuilder:validation:XValidation:message="May specify either httpRouteFilterRequestRedirect or httpRouteFilterRequestRewrite, but not both",rule="!(self.exists(f, f.type == 'RequestRedirect') && self.exists(f, f.type == 'URLRewrite'))"
// 	// +kubebuilder:validation:XValidation:message="RequestHeaderModifier filter cannot be repeated",rule="self.filter(f, f.type == 'RequestHeaderModifier').size() <= 1"
// 	// +kubebuilder:validation:XValidation:message="ResponseHeaderModifier filter cannot be repeated",rule="self.filter(f, f.type == 'ResponseHeaderModifier').size() <= 1"
// 	// +kubebuilder:validation:XValidation:message="RequestRedirect filter cannot be repeated",rule="self.filter(f, f.type == 'RequestRedirect').size() <= 1"
// 	// +kubebuilder:validation:XValidation:message="URLRewrite filter cannot be repeated",rule="self.filter(f, f.type == 'URLRewrite').size() <= 1"
// 	Filters []gatewayapiv1.HTTPRouteFilter `json:"filters,omitempty"`
// }
