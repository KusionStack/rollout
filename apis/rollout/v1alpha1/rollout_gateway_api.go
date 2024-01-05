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

type HTTPRouteFilter struct {
	// RequestHeaderModifier defines a schema for a filter that modifies request
	// headers.
	//
	// Support: Core
	//
	// +optional
	RequestHeaderModifier *gatewayapiv1.HTTPHeaderFilter `json:"requestHeaderModifier,omitempty"`
}

type HTTPRouteRule struct {
	// Matches define conditions used for matching the incoming HTTP requests to canary service.
	Matches []HTTPRouteMatch `json:"matches,omitempty"`
	// Filter defines a filter for the canary service.
	Filters []HTTPRouteFilter `json:"filters,omitempty"`
}
