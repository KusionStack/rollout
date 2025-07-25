package httproute

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func newPathMatch(path string) *gatewayapiv1.HTTPPathMatch {
	return &gatewayapiv1.HTTPPathMatch{
		Type:  ptr.To(gatewayapiv1.PathMatchExact),
		Value: ptr.To(path),
	}
}

func newHeaderMatch(name, value string) gatewayapiv1.HTTPHeaderMatch {
	return gatewayapiv1.HTTPHeaderMatch{
		Type:  ptr.To(gatewayapiv1.HeaderMatchExact),
		Name:  gatewayapiv1.HTTPHeaderName(name),
		Value: value,
	}
}

func newQueryParamMatch(name, value string) gatewayapiv1.HTTPQueryParamMatch {
	return gatewayapiv1.HTTPQueryParamMatch{
		Type:  ptr.To(gatewayapiv1.QueryParamMatchExact),
		Name:  gatewayapiv1.HTTPHeaderName(name),
		Value: value,
	}
}

func Test_mergeHTTPRouteMatches(t *testing.T) {
	tests := []struct {
		name          string
		oldMatches    []gatewayapiv1.HTTPRouteMatch
		canaryMatches []gatewayapiv1.HTTPRouteMatch
		want          []gatewayapiv1.HTTPRouteMatch
	}{
		{
			// [Test Case] Scenario: When canary has path matches, they should replace the original path matches
			name: "pathMatches",
			oldMatches: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: newPathMatch("/test"),
				},
			},
			canaryMatches: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: newPathMatch("/canary-1"),
				},
				{
					Path: newPathMatch("/canary-2"),
				},
			},
			want: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: newPathMatch("/canary-1"),
				},
				{
					Path: newPathMatch("/canary-2"),
				},
			},
		},
		{
			// [Test Case] Scenario: When canary adds header matches, they should be merged with original path matches
			name: "nonPathMatches: add headers",
			oldMatches: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: newPathMatch("/test"),
				},
			},
			canaryMatches: []gatewayapiv1.HTTPRouteMatch{
				{
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						newHeaderMatch("canary", "true"),
						newHeaderMatch("uid", "test-1"),
					},
				},
				{
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						newHeaderMatch("canary", "true"),
						newHeaderMatch("uid", "test-2"),
					},
				},
			},
			want: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: newPathMatch("/test"),
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						newHeaderMatch("canary", "true"),
						newHeaderMatch("uid", "test-1"),
					},
				},
				{
					Path: newPathMatch("/test"),
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						newHeaderMatch("canary", "true"),
						newHeaderMatch("uid", "test-2"),
					},
				},
			},
		},
		{
			// [Test Case] Scenario: When canary adds query param matches, they should be merged with original path matches
			name: "nonPathMatches: add queryParams",
			oldMatches: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: newPathMatch("/test"),
				},
			},
			canaryMatches: []gatewayapiv1.HTTPRouteMatch{
				{
					QueryParams: []gatewayapiv1.HTTPQueryParamMatch{
						newQueryParamMatch("canary", "true"),
						newQueryParamMatch("uid", "test-1"),
					},
				},
				{
					QueryParams: []gatewayapiv1.HTTPQueryParamMatch{
						newQueryParamMatch("canary", "true"),
						newQueryParamMatch("uid", "test-2"),
					},
				},
			},
			want: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: newPathMatch("/test"),
					QueryParams: []gatewayapiv1.HTTPQueryParamMatch{
						newQueryParamMatch("canary", "true"),
						newQueryParamMatch("uid", "test-1"),
					},
				},
				{
					Path: newPathMatch("/test"),
					QueryParams: []gatewayapiv1.HTTPQueryParamMatch{
						newQueryParamMatch("canary", "true"),
						newQueryParamMatch("uid", "test-2"),
					},
				},
			},
		},
		{
			// [Test Case] Scenario: When canary adds both header and query param matches, they should all be merged with original path matches
			name: "nonPathMatches: add headers and queryParams",
			oldMatches: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: newPathMatch("/test"),
				},
			},
			canaryMatches: []gatewayapiv1.HTTPRouteMatch{
				{
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						newHeaderMatch("canary", "true"),
						newHeaderMatch("uid", "test-1"),
					},
					QueryParams: []gatewayapiv1.HTTPQueryParamMatch{
						newQueryParamMatch("canary", "true"),
						newQueryParamMatch("uid", "test-1"),
					},
				},
			},
			want: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: newPathMatch("/test"),
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						newHeaderMatch("canary", "true"),
						newHeaderMatch("uid", "test-1"),
					},
					QueryParams: []gatewayapiv1.HTTPQueryParamMatch{
						newQueryParamMatch("canary", "true"),
						newQueryParamMatch("uid", "test-1"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeHTTPRouteMatches(tt.oldMatches, tt.canaryMatches)
			assert.Equal(t, tt.want, got)
		})
	}
}

func newRequestHeaderModifierFilter(name, value string) gatewayapiv1.HTTPRouteFilter {
	return gatewayapiv1.HTTPRouteFilter{
		Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
		RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
			Set: []gatewayapiv1.HTTPHeader{
				{Name: gatewayapiv1.HTTPHeaderName(name), Value: value},
			},
		},
	}
}

func newResponseHeaderModifierFilter(name, value string) gatewayapiv1.HTTPRouteFilter {
	return gatewayapiv1.HTTPRouteFilter{
		Type: gatewayapiv1.HTTPRouteFilterResponseHeaderModifier,
		ResponseHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
			Set: []gatewayapiv1.HTTPHeader{
				{Name: gatewayapiv1.HTTPHeaderName(name), Value: value},
			},
		},
	}
}

func newRequestRedirectFilter(host string) gatewayapiv1.HTTPRouteFilter {
	return gatewayapiv1.HTTPRouteFilter{
		Type: gatewayapiv1.HTTPRouteFilterRequestRedirect,
		RequestRedirect: &gatewayapiv1.HTTPRequestRedirectFilter{
			Hostname: ptr.To(gatewayapiv1.PreciseHostname(host)),
		},
	}
}

func newURLRewriteFilter(host string) gatewayapiv1.HTTPRouteFilter {
	return gatewayapiv1.HTTPRouteFilter{
		Type: gatewayapiv1.HTTPRouteFilterURLRewrite,
		URLRewrite: &gatewayapiv1.HTTPURLRewriteFilter{
			Hostname: ptr.To(gatewayapiv1.PreciseHostname(host)),
		},
	}
}

func TestMergeHTTPRouteFilter(t *testing.T) {
	tests := []struct {
		name          string
		oldFilters    []gatewayapiv1.HTTPRouteFilter
		canaryFilters []gatewayapiv1.HTTPRouteFilter
		expected      []gatewayapiv1.HTTPRouteFilter
	}{
		{
			// [Test Case] Scenario: Empty inputs should return empty result
			name:          "empty inputs",
			oldFilters:    []gatewayapiv1.HTTPRouteFilter{},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{},
			expected:      []gatewayapiv1.HTTPRouteFilter{},
		},
		{
			// [Test Case] Scenario: Only old filters should be preserved when no canary filters exist
			name: "only old filters",
			oldFilters: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
			},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{},
			expected: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
			},
		},
		{
			// [Test Case] Scenario: Only canary filters should be preserved when no old filters exist
			name:       "only canary filters",
			oldFilters: []gatewayapiv1.HTTPRouteFilter{},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Canary-Header", "canary-value"),
				newResponseHeaderModifierFilter("X-Canary-Response", "canary-response"),
			},
			expected: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Canary-Header", "canary-value"),
				newResponseHeaderModifierFilter("X-Canary-Response", "canary-response"),
			},
		},
		{
			// [Test Case] Scenario: Conflicting RequestHeaderModifier should prefer canary version
			name: "conflict RequestHeaderModifier keeps canary",
			oldFilters: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
			},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Canary-Header", "canary-value"),
			},
			expected: []gatewayapiv1.HTTPRouteFilter{
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
				newRequestHeaderModifierFilter("X-Canary-Header", "canary-value"),
			},
		},
		{
			// [Test Case] Scenario: Conflicting ResponseHeaderModifier should prefer canary version
			name: "conflict ResponseHeaderModifier keeps canary",
			oldFilters: []gatewayapiv1.HTTPRouteFilter{
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
			},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{
				newResponseHeaderModifierFilter("X-Canary-Response", "canary-response"),
			},
			expected: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
				newResponseHeaderModifierFilter("X-Canary-Response", "canary-response"),
			},
		},
		{
			// [Test Case] Scenario: Conflicting RequestRedirect should prefer canary version
			name: "conflict RequestRedirect keeps canary",
			oldFilters: []gatewayapiv1.HTTPRouteFilter{
				newRequestRedirectFilter("old.example.com"),
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
			},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{
				newRequestRedirectFilter("canary.example.com"),
			},
			expected: []gatewayapiv1.HTTPRouteFilter{
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
				newRequestRedirectFilter("canary.example.com"),
			},
		},
		{
			// [Test Case] Scenario: Conflicting URLRewrite should prefer canary version
			name: "conflict URLRewrite keeps canary",
			oldFilters: []gatewayapiv1.HTTPRouteFilter{
				newURLRewriteFilter("old.rewrite.com"),
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
			},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{
				newURLRewriteFilter("canary.rewrite.com"),
			},
			expected: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
				newURLRewriteFilter("canary.rewrite.com"),
			},
		},
		{
			// [Test Case] Scenario: Mutual exclusion between RequestRedirect and URLRewrite should prefer canary URLRewrite
			name: "mutual exclusion RequestRedirect vs URLRewrite keeps canary URLRewrite",
			oldFilters: []gatewayapiv1.HTTPRouteFilter{
				newRequestRedirectFilter("old.example.com"),
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
			},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{
				newURLRewriteFilter("canary.rewrite.com"),
			},
			expected: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
				newURLRewriteFilter("canary.rewrite.com"),
			},
		},
		{
			// [Test Case] Scenario: Mutual exclusion between URLRewrite and RequestRedirect should prefer canary RequestRedirect
			name: "mutual exclusion URLRewrite vs RequestRedirect keeps canary RequestRedirect",
			oldFilters: []gatewayapiv1.HTTPRouteFilter{
				newURLRewriteFilter("old.rewrite.com"),
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
			},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{
				newRequestRedirectFilter("canary.example.com"),
			},
			expected: []gatewayapiv1.HTTPRouteFilter{
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
				newRequestRedirectFilter("canary.example.com"),
			},
		},
		{
			// [Test Case] Scenario: Mixed filter types with conflicts should properly merge with canary preferences
			name: "mixed filter types with conflicts",
			oldFilters: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
				newResponseHeaderModifierFilter("X-Old-Response", "old-response"),
				newRequestRedirectFilter("old.example.com"),
			},
			canaryFilters: []gatewayapiv1.HTTPRouteFilter{
				newResponseHeaderModifierFilter("X-Canary-Response", "canary-response"),
				newURLRewriteFilter("canary.rewrite.com"),
			},
			expected: []gatewayapiv1.HTTPRouteFilter{
				newRequestHeaderModifierFilter("X-Old-Header", "old-value"),
				newResponseHeaderModifierFilter("X-Canary-Response", "canary-response"),
				newURLRewriteFilter("canary.rewrite.com"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeHTTPRouteFilter(tt.oldFilters, tt.canaryFilters)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func newBackendRef(gvk schema.GroupVersionKind, name gatewayapiv1.ObjectName, weight *int32) gatewayapiv1.HTTPBackendRef {
	return gatewayapiv1.HTTPBackendRef{
		BackendRef: gatewayapiv1.BackendRef{
			BackendObjectReference: gatewayapiv1.BackendObjectReference{
				Group: ptr.To(gatewayapiv1.Group(gvk.Group)),
				Kind:  ptr.To(gatewayapiv1.Kind(gvk.Kind)),
				Name:  name,
			},
			Weight: weight,
		},
	}
}

func TestAddHTTPRouteRules(t *testing.T) {
	gvk := corev1.SchemeGroupVersion.WithKind("Service")

	targetBackendName := gatewayapiv1.ObjectName("target-backend")
	canaryBackendName := gatewayapiv1.ObjectName("canary-backend")

	tests := []struct {
		name     string
		input    []gatewayapiv1.HTTPRouteRule
		gvk      schema.GroupVersionKind
		target   gatewayapiv1.ObjectName
		canary   gatewayapiv1.ObjectName
		newRule  *rolloutv1alpha1.CanaryHTTPRouteRule
		expected []gatewayapiv1.HTTPRouteRule
	}{
		{
			// [Test Case] Scenario: When canary backendRef already exists, return original rules unchanged
			name: "canary backend exists",
			input: []gatewayapiv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, canaryBackendName, nil),
					},
				},
			},
			gvk:    gvk,
			target: targetBackendName,
			canary: canaryBackendName,
			newRule: &rolloutv1alpha1.CanaryHTTPRouteRule{
				Weight: ptr.To(int32(20)),
			},
			expected: []gatewayapiv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, canaryBackendName, nil),
					},
				},
			},
		},
		{
			// [Test Case] Scenario: When newRule has weight, add weighted backend refs
			name: "add weighted backend refs",
			input: []gatewayapiv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, targetBackendName, nil),
					},
				},
			},
			gvk:    gvk,
			target: targetBackendName,
			canary: canaryBackendName,
			newRule: &rolloutv1alpha1.CanaryHTTPRouteRule{
				Weight: ptr.To(int32(20)),
			},
			expected: []gatewayapiv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, targetBackendName, ptr.To(int32(80))),
						newBackendRef(gvk, canaryBackendName, ptr.To(int32(20))),
					},
				},
			},
		},
		{
			// [Test Case] Scenario: When newRule has matches, add matches backend refs
			name: "add matches backend refs",
			input: []gatewayapiv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, targetBackendName, nil),
					},
				},
			},
			gvk:    gvk,
			target: targetBackendName,
			canary: canaryBackendName,
			newRule: &rolloutv1alpha1.CanaryHTTPRouteRule{
				HTTPRouteRule: rolloutv1alpha1.HTTPRouteRule{
					Matches: []rolloutv1alpha1.HTTPRouteMatch{
						{
							Headers: []gatewayapiv1.HTTPHeaderMatch{
								{
									Name:  "Canary",
									Value: "true",
								},
							},
						},
					},
				},
			},
			expected: []gatewayapiv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, targetBackendName, nil),
					},
				},
				{
					Name: ptr.To(gatewayapiv1.SectionName(getCanaryRuleName(0))),
					Matches: []gatewayapiv1.HTTPRouteMatch{
						{
							Headers: []gatewayapiv1.HTTPHeaderMatch{
								{
									Name:  "Canary",
									Value: "true",
								},
							},
						},
					},
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, canaryBackendName, nil),
					},
				},
			},
		},
		{
			// [Test Case] Scenario: skip existing canary backend
			name: "skip existing canary backend",
			input: []gatewayapiv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, targetBackendName, nil),
						newBackendRef(gvk, canaryBackendName, ptr.To(int32(30))),
					},
				},
			},
			gvk:    gvk,
			target: targetBackendName,
			canary: canaryBackendName,
			newRule: &rolloutv1alpha1.CanaryHTTPRouteRule{
				Weight: ptr.To(int32(1)),
			},
			expected: []gatewayapiv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, targetBackendName, nil),
						newBackendRef(gvk, canaryBackendName, ptr.To(int32(30))),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addHTTPRouteRules(tt.input, tt.gvk, string(tt.target), string(tt.canary), tt.newRule)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_deleteBackendRefRules(t *testing.T) {
	gvk := corev1.SchemeGroupVersion.WithKind("Service")
	targetBackend := gatewayapiv1.ObjectName("target-backend")
	canaryBackend := gatewayapiv1.ObjectName("canary-backend")

	tests := []struct {
		name          string
		oldRules      []gatewayapiv1.HTTPRouteRule
		gvk           schema.GroupVersionKind
		targetBackend gatewayapiv1.ObjectName
		canaryBackend gatewayapiv1.ObjectName
		want          []gatewayapiv1.HTTPRouteRule
	}{
		{
			// [Test Case] Scenario: Remove canary backend
			name: "remove canary backends",
			oldRules: []gatewayapiv1.HTTPRouteRule{{
				BackendRefs: []gatewayapiv1.HTTPBackendRef{
					newBackendRef(gvk, targetBackend, ptr.To(int32(80))),
					newBackendRef(gvk, canaryBackend, ptr.To(int32(20))),
				},
			}},
			gvk:           gvk,
			targetBackend: targetBackend,
			canaryBackend: canaryBackend,
			want: []gatewayapiv1.HTTPRouteRule{{
				BackendRefs: []gatewayapiv1.HTTPBackendRef{
					newBackendRef(gvk, targetBackend, ptr.To(int32(1))),
				},
			}},
		},
		{
			// [Test Case] Scenario: No backends to remove should return original rules
			name: "no backends to remove",
			oldRules: []gatewayapiv1.HTTPRouteRule{{
				BackendRefs: []gatewayapiv1.HTTPBackendRef{
					newBackendRef(gvk, targetBackend, nil),
				},
			}},
			gvk:           gvk,
			targetBackend: targetBackend,
			canaryBackend: canaryBackend,
			want: []gatewayapiv1.HTTPRouteRule{{
				BackendRefs: []gatewayapiv1.HTTPBackendRef{
					newBackendRef(gvk, targetBackend, nil),
				},
			}},
		},
		{
			name: "delete canary rule",
			oldRules: []gatewayapiv1.HTTPRouteRule{
				{
					Matches: []gatewayapiv1.HTTPRouteMatch{
						{
							Path: newPathMatch("/"),
						},
					},
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, targetBackend, nil),
					},
				},
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, canaryBackend, nil),
					},
				},
			},
			gvk:           gvk,
			targetBackend: targetBackend,
			canaryBackend: canaryBackend,
			want: []gatewayapiv1.HTTPRouteRule{
				{
					Matches: []gatewayapiv1.HTTPRouteMatch{
						{
							Path: newPathMatch("/"),
						},
					},
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						newBackendRef(gvk, targetBackend, nil),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deleteBackendRefRules(tt.oldRules, tt.gvk, string(tt.targetBackend), string(tt.canaryBackend))
			assert.Equal(t, tt.want, got)
		})
	}
}
