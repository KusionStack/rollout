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

// HeaderMatchType specifies the semantics of how HTTP header values should be
// compared. Valid HeaderMatchType values are:
//
// * "Exact"
// * "RegularExpression"
//
// +kubebuilder:validation:Enum=Exact;RegularExpression
type HeaderMatchType string

// HeaderMatchType constants.
const (
	HeaderMatchExact             HeaderMatchType = "Exact"
	HeaderMatchRegularExpression HeaderMatchType = "RegularExpression"
)

// HTTPHeaderName is the name of an HTTP header.
//
// Valid values include:
//
// * "Authorization"
// * "Set-Cookie"
//
// Invalid values include:
//
//   - ":method" - ":" is an invalid character. This means that HTTP/2 pseudo
//     headers are not currently supported by this type.
//   - "/invalid" - "/" is an invalid character
//
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=256
// +kubebuilder:validation:Pattern=`^[A-Za-z0-9!#$%&'*+\-.^_\x60|~]+$`
type HTTPHeaderName string

// HTTPHeaderMatch describes how to select a HTTP route by matching HTTP request
// headers.
type HTTPHeaderMatch struct {
	// Type specifies how to match against the value of the header.
	//
	// Support: Core (Exact)
	//
	// Support: Custom (RegularExpression)
	//
	// Since RegularExpression HeaderMatchType has custom conformance,
	// implementations can support POSIX, PCRE or any other dialects of regular
	// expressions. Please read the implementation's documentation to determine
	// the supported dialect.
	//
	// +optional
	// +kubebuilder:default=Exact
	Type *HeaderMatchType `json:"type,omitempty"`

	// Name is the name of the HTTP Header to be matched. Name matching MUST be
	// case insensitive. (See https://tools.ietf.org/html/rfc7230#section-3.2).
	//
	// If multiple entries specify equivalent header names, only the first
	// entry with an equivalent name MUST be considered for a match. Subsequent
	// entries with an equivalent header name MUST be ignored. Due to the
	// case-insensitivity of header names, "foo" and "Foo" are considered
	// equivalent.
	//
	// When a header is repeated in an HTTP request, it is
	// implementation-specific behavior as to how this is represented.
	// Generally, proxies should follow the guidance from the RFC:
	// https://www.rfc-editor.org/rfc/rfc7230.html#section-3.2.2 regarding
	// processing a repeated header, with special handling for "Set-Cookie".
	Name HTTPHeaderName `json:"name"`

	// Value is the value of HTTP Header to be matched.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Value string `json:"value"`
}

// HTTPHeaderFilter defines a filter that modifies the headers of an HTTP
// request or response. Only one action for a given header name is permitted.
// Filters specifying multiple actions of the same or different type for any one
// header name are invalid and will be rejected by the webhook if installed.
// Configuration to set or add multiple values for a header must use RFC 7230
// header value formatting, separating each value with a comma.
type HTTPHeaderFilter struct {
	// Set overwrites the request with the given header (name, value)
	// before the action.
	//
	// Input:
	//   GET /foo HTTP/1.1
	//   my-header: foo
	//
	// Config:
	//   set:
	//   - name: "my-header"
	//     value: "bar"
	//
	// Output:
	//   GET /foo HTTP/1.1
	//   my-header: bar
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	Set []HTTPHeader `json:"set,omitempty"`

	// Add adds the given header(s) (name, value) to the request
	// before the action. It appends to any existing values associated
	// with the header name.
	//
	// Input:
	//   GET /foo HTTP/1.1
	//   my-header: foo
	//
	// Config:
	//   add:
	//   - name: "my-header"
	//     value: "bar,baz"
	//
	// Output:
	//   GET /foo HTTP/1.1
	//   my-header: foo,bar,baz
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	Add []HTTPHeader `json:"add,omitempty"`

	// Remove the given header(s) from the HTTP request before the action. The
	// value of Remove is a list of HTTP header names. Note that the header
	// names are case-insensitive (see
	// https://datatracker.ietf.org/doc/html/rfc2616#section-4.2).
	//
	// Input:
	//   GET /foo HTTP/1.1
	//   my-header1: foo
	//   my-header2: bar
	//   my-header3: baz
	//
	// Config:
	//   remove: ["my-header1", "my-header3"]
	//
	// Output:
	//   GET /foo HTTP/1.1
	//   my-header2: bar
	//
	// +optional
	// +kubebuilder:validation:MaxItems=16
	Remove []string `json:"remove,omitempty"`
}

// HTTPHeader represents an HTTP Header name and value as defined by RFC 7230.
type HTTPHeader struct {
	// Name is the name of the HTTP Header to be matched. Name matching MUST be
	// case insensitive. (See https://tools.ietf.org/html/rfc7230#section-3.2).
	//
	// If multiple entries specify equivalent header names, the first entry with
	// an equivalent name MUST be considered for a match. Subsequent entries
	// with an equivalent header name MUST be ignored. Due to the
	// case-insensitivity of header names, "foo" and "Foo" are considered
	// equivalent.
	Name HTTPHeaderName `json:"name"`

	// Value is the value of HTTP Header to be matched.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Value string `json:"value"`
}
