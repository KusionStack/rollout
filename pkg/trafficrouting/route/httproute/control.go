package httproute

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	clientutil "kusionstack.io/kube-utils/client"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"kusionstack.io/rollout/pkg/trafficrouting/route"
)

var _ route.RouteController = &httpRouteControl{}

type httpRouteControl struct {
	client         client.Client
	backendrouting *rolloutv1alpha1.BackendRouting
	routeObj       *gatewayapiv1.HTTPRoute
	routeStatus    rolloutv1alpha1.BackendRouteStatus
}

func (r *httpRouteControl) GetRoute() client.Object {
	return r.routeObj
}

// Initialize implements route.RouteControl.
func (c *httpRouteControl) Initialize(ctx context.Context) error {
	from := c.backendrouting.Spec.Backend.Name
	to := c.routeStatus.Forwarding.Origin.BackendName
	typeRef := c.backendrouting.Spec.Backend.ObjectTypeRef

	_, err := clientutil.UpdateOnConflict(clusterinfo.WithCluster(ctx, c.getCluster()), c.client, c.client, c.routeObj, func(in *gatewayapiv1.HTTPRoute) error {
		if in.Annotations == nil {
			in.Annotations = make(map[string]string)
		}
		got := in.Annotations[rolloutapi.OriginRouteSpecAnnoKey]
		if len(got) == 0 {
			// backup origin spec
			origin := encodeHTTPRouteSpec(&in.Spec)
			in.Annotations[rolloutapi.OriginRouteSpecAnnoKey] = origin
		}

		// change origin backend name
		gvk := schema.FromAPIVersionAndKind(typeRef.APIVersion, typeRef.Kind)
		for i := range in.Spec.Rules {
			for j, backendRef := range in.Spec.Rules[i].BackendRefs {
				if isBackendMatches(backendRef, gvk, from) {
					// found, change the name
					in.Spec.Rules[i].BackendRefs[j].Name = gatewayapiv1.ObjectName(to)
				}
			}
		}
		return nil
	})
	return err
}

// Reset implements route.RouteControl.
func (c *httpRouteControl) Reset(ctx context.Context) error {
	_, err := clientutil.UpdateOnConflict(clusterinfo.WithCluster(ctx, c.getCluster()), c.client, c.client, c.routeObj, func(in *gatewayapiv1.HTTPRoute) error {
		if in.Annotations == nil {
			in.Annotations = make(map[string]string)
		}
		originSpec := in.Annotations[rolloutapi.OriginRouteSpecAnnoKey]
		if len(originSpec) == 0 {
			return fmt.Errorf("failed to find origin spec in HTTPRoute annotations")
		}
		origin, err := decodeHTTPRouteSpec(originSpec)
		if err != nil {
			return err
		}
		in.Spec = *origin
		delete(in.Annotations, rolloutapi.OriginRouteSpecAnnoKey)
		return nil
	})
	return err
}

// AddCanary implements route.RouteControl.
func (c *httpRouteControl) AddCanary(ctx context.Context) error {
	gvk := schema.FromAPIVersionAndKind(c.backendrouting.Spec.Backend.ObjectTypeRef.APIVersion, c.backendrouting.Spec.Backend.ObjectTypeRef.Kind)
	_, err := clientutil.UpdateOnConflict(clusterinfo.WithCluster(ctx, c.getCluster()), c.client, c.client, c.routeObj, func(in *gatewayapiv1.HTTPRoute) error {
		stable := c.routeStatus.Forwarding.Origin.BackendName
		in.Spec.Rules = addHTTPRouteRules(
			in.Spec.Rules,
			gvk,
			stable,
			c.backendrouting.Spec.ForkedBackends.Canary.Name,
			&c.backendrouting.Spec.Forwarding.HTTP.Canary.CanaryHTTPRouteRule,
		)
		return nil
	})
	return err
}

// DeleteCanary implements route.RouteControl.
func (c *httpRouteControl) DeleteCanary(ctx context.Context) error {
	gvk := schema.FromAPIVersionAndKind(c.backendrouting.Spec.Backend.ObjectTypeRef.APIVersion, c.backendrouting.Spec.Backend.ObjectTypeRef.Kind)
	_, err := clientutil.UpdateOnConflict(clusterinfo.WithCluster(ctx, c.getCluster()), c.client, c.client, c.routeObj, func(in *gatewayapiv1.HTTPRoute) error {
		in.Spec.Rules = deleteBackendRefRules(
			in.Spec.Rules,
			gvk,
			c.routeStatus.Forwarding.Origin.BackendName,
			c.backendrouting.Spec.ForkedBackends.Canary.Name,
		)
		return nil
	})
	return err
}

func (c *httpRouteControl) getCluster() string {
	return c.routeStatus.Cluster
}

func encodeHTTPRouteSpec(in *gatewayapiv1.HTTPRouteSpec) string {
	data, _ := json.Marshal(in)
	return string(data)
}

func decodeHTTPRouteSpec(in string) (*gatewayapiv1.HTTPRouteSpec, error) {
	spec := &gatewayapiv1.HTTPRouteSpec{}
	err := json.Unmarshal([]byte(in), spec)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

func addHTTPRouteRules(in []gatewayapiv1.HTTPRouteRule, gvk schema.GroupVersionKind, targetBackend, canaryBackend string, newRule *rolloutv1alpha1.CanaryHTTPRouteRule) []gatewayapiv1.HTTPRouteRule {
	// pre check: if canary backendRef already exists, skip it
	for i := range in {
		rule := &in[i]
		_, canaryBackendRef := findBackendRef(rule, gvk, canaryBackend)
		if canaryBackendRef != nil {
			return in
		}
	}

	if newRule.Weight != nil {
		return addWeightedBackendRefs(in, gvk, targetBackend, canaryBackend, newRule)
	}
	return addMatchesBackendRefs(in, gvk, targetBackend, canaryBackend, newRule)
}

func addMatchesBackendRefs(oldRules []gatewayapiv1.HTTPRouteRule, gvk schema.GroupVersionKind, targetBackend, canaryBackend string, newRule *rolloutv1alpha1.CanaryHTTPRouteRule) []gatewayapiv1.HTTPRouteRule {
	if newRule.Matches == nil {
		return oldRules
	}

	outputRules := make([]gatewayapiv1.HTTPRouteRule, 0)
	canaryRules := make([]gatewayapiv1.HTTPRouteRule, 0)

	newMatches := []gatewayapiv1.HTTPRouteMatch{}
	for _, match := range newRule.Matches {
		newMatches = append(newMatches, gatewayapiv1.HTTPRouteMatch{
			Path:        match.Path,
			Headers:     match.Headers,
			QueryParams: match.QueryParams,
		})
	}

	for i := range oldRules {
		rule := &oldRules[i]

		_, canaryBackendRef := findBackendRef(rule, gvk, canaryBackend)
		if canaryBackendRef != nil {
			// canary backendRef already exists, skip it
			continue
		}

		index, targetBackendRef := findBackendRef(rule, gvk, targetBackend)
		if targetBackendRef == nil {
			continue
		}

		canaryRule := rule.DeepCopy()
		canaryRule.BackendRefs[index].Name = gatewayapiv1.ObjectName(canaryBackend)
		canaryRule.Matches = mergeHTTPRouteMatches(canaryRule.Matches, newMatches)
		canaryRule.Filters = mergeHTTPRouteFilter(canaryRule.Filters, newRule.Filters)
		canaryRules = append(canaryRules, *canaryRule)
	}

	outputRules = append(outputRules, oldRules...)
	for i := range canaryRules {
		canaryRules[i].Name = ptr.To(gatewayapiv1.SectionName(getCanaryRuleName(i)))
		outputRules = append(outputRules, canaryRules[i])
	}
	return outputRules
}

func addWeightedBackendRefs(oldRules []gatewayapiv1.HTTPRouteRule, gvk schema.GroupVersionKind, targetBackend, canaryBackend string, newRule *rolloutv1alpha1.CanaryHTTPRouteRule) []gatewayapiv1.HTTPRouteRule {
	if newRule.Weight == nil {
		return oldRules
	}

	outputRules := make([]gatewayapiv1.HTTPRouteRule, 0)
	for i := range oldRules {
		rule := &oldRules[i]
		_, targetBackendRef := findBackendRef(rule, gvk, targetBackend)
		if targetBackendRef == nil {
			outputRules = append(outputRules, *rule)
			continue
		}
		_, canaryBackendRef := findBackendRef(rule, gvk, canaryBackend)
		if canaryBackendRef == nil {
			canaryBackendRef = targetBackendRef.DeepCopy()
		}

		canaryBackendRef.Name = gatewayapiv1.ObjectName(canaryBackend)
		canaryBackendRef.Weight = newRule.Weight
		canaryBackendRef.Filters = mergeHTTPRouteFilter(canaryBackendRef.Filters, newRule.Filters)

		targetBackendRef.Weight = ptr.To(100 - *canaryBackendRef.Weight)

		setBackendRef(rule, gvk, *targetBackendRef)
		setBackendRef(rule, gvk, *canaryBackendRef)
		outputRules = append(outputRules, *rule)
	}
	return outputRules
}

func deleteBackendRefRules(oldRules []gatewayapiv1.HTTPRouteRule, gvk schema.GroupVersionKind, targetBackend, canaryBackend string) []gatewayapiv1.HTTPRouteRule {
	outputRules := make([]gatewayapiv1.HTTPRouteRule, 0)
	for i := range oldRules {
		_, canaryBackendRef := findBackendRef(&oldRules[i], gvk, canaryBackend)
		if canaryBackendRef == nil {
			// no canary backend ref found, skip it
			outputRules = append(outputRules, oldRules[i])
			continue
		}
		rule := oldRules[i].DeepCopy()
		// delete canary backendRef
		filterOutBackendRef(rule, gvk, canaryBackend)
		// find target backendRef and reset weight to 1
		index, targetBackend := findBackendRef(rule, gvk, targetBackend)
		if targetBackend != nil {
			rule.BackendRefs[index].Weight = ptr.To[int32](1)
		}
		if len(rule.BackendRefs) == 0 {
			// no backendRef, skip
			continue
		}
		outputRules = append(outputRules, *rule)
	}
	return outputRules
}

func filterOutBackendRef(rule *gatewayapiv1.HTTPRouteRule, gvk schema.GroupVersionKind, name string) {
	newBackends := lo.Reject(rule.BackendRefs, func(ref gatewayapiv1.HTTPBackendRef, _ int) bool {
		return isBackendMatches(ref, gvk, name)
	})
	rule.BackendRefs = newBackends
}

func findBackendRef(rule *gatewayapiv1.HTTPRouteRule, gvk schema.GroupVersionKind, name string) (int, *gatewayapiv1.HTTPBackendRef) {
	for i, backendRef := range rule.BackendRefs {
		if isBackendMatches(backendRef, gvk, name) {
			return i, &backendRef
		}
	}
	return -1, nil
}

func setBackendRef(rule *gatewayapiv1.HTTPRouteRule, gvk schema.GroupVersionKind, ref gatewayapiv1.HTTPBackendRef) {
	index, backendRef := findBackendRef(rule, gvk, string(ref.Name))
	if backendRef == nil {
		rule.BackendRefs = append(rule.BackendRefs, ref)
		return
	}
	rule.BackendRefs[index] = ref
}

func isBackendMatches(backendRef gatewayapiv1.HTTPBackendRef, gvk schema.GroupVersionKind, name string) bool {
	return gvk.Group == string(ptr.Deref(backendRef.Group, "")) &&
		gvk.Kind == string(ptr.Deref(backendRef.Kind, "Service")) &&
		name == string(backendRef.Name)
}

func mergeHTTPRouteMatches(oldMatches, canaryMatches []gatewayapiv1.HTTPRouteMatch) []gatewayapiv1.HTTPRouteMatch {
	if len(oldMatches) == 0 {
		return canaryMatches
	}

	canaryPathMatches := lo.Filter(canaryMatches, func(match gatewayapiv1.HTTPRouteMatch, _ int) bool {
		return match.Path != nil
	})
	canarnNonPathMatches := lo.Filter(canaryMatches, func(match gatewayapiv1.HTTPRouteMatch, _ int) bool {
		return match.Path == nil
	})
	// remain all canary path matches
	newMatches := []gatewayapiv1.HTTPRouteMatch{}
	newMatches = append(newMatches, canaryPathMatches...)
	for i := range oldMatches {
		for j := range canarnNonPathMatches {
			// add headers and query params matches to old match
			if len(canarnNonPathMatches[j].Headers) == 0 && len(canarnNonPathMatches[j].QueryParams) == 0 {
				continue
			}
			oldMatch := oldMatches[i].DeepCopy()
			oldMatch.Headers = append(oldMatch.Headers, canarnNonPathMatches[j].Headers...)
			oldMatch.QueryParams = append(oldMatch.QueryParams, canarnNonPathMatches[j].QueryParams...)
			newMatches = append(newMatches, *oldMatch)
		}
	}
	return newMatches
}

func mergeHTTPRouteFilter(oldFilters, canaryFilters []gatewayapiv1.HTTPRouteFilter) []gatewayapiv1.HTTPRouteFilter {
	if len(oldFilters) == 0 {
		return canaryFilters
	}

	newFilters := []gatewayapiv1.HTTPRouteFilter{}
	newFilters = append(newFilters, oldFilters...)
	// HTTPRouteFilters validation rules
	//
	// +kubebuilder:validation:XValidation:message="May specify either httpRouteFilterRequestRedirect or httpRouteFilterRequestRewrite, but not both",rule="!(self.exists(f, f.type == 'RequestRedirect') && self.exists(f, f.type == 'URLRewrite'))"
	// +kubebuilder:validation:XValidation:message="RequestHeaderModifier filter cannot be repeated",rule="self.filter(f, f.type == 'RequestHeaderModifier').size() <= 1"
	// +kubebuilder:validation:XValidation:message="ResponseHeaderModifier filter cannot be repeated",rule="self.filter(f, f.type == 'ResponseHeaderModifier').size() <= 1"
	// +kubebuilder:validation:XValidation:message="RequestRedirect filter cannot be repeated",rule="self.filter(f, f.type == 'RequestRedirect').size() <= 1"
	// +kubebuilder:validation:XValidation:message="URLRewrite filter cannot be repeated",rule="self.filter(f, f.type == 'URLRewrite').size() <= 1"
	for _, canaryFiler := range canaryFilters {
		// request header modifier can not be repeated
		if canaryFiler.RequestHeaderModifier != nil {
			newFilters = lo.Reject(newFilters, func(filter gatewayapiv1.HTTPRouteFilter, _ int) bool {
				return filter.RequestHeaderModifier != nil
			})
		}
		// response header modifier can not be repeated
		if canaryFiler.ResponseHeaderModifier != nil {
			newFilters = lo.Reject(newFilters, func(filter gatewayapiv1.HTTPRouteFilter, _ int) bool {
				return filter.ResponseHeaderModifier != nil
			})
		}
		// request redirect can not be repeated
		// RequestRedirect can not set URLRewrite
		if canaryFiler.RequestRedirect != nil {
			newFilters = lo.Reject(newFilters, func(filter gatewayapiv1.HTTPRouteFilter, _ int) bool {
				return filter.RequestRedirect != nil || filter.URLRewrite != nil
			})
		}
		// URLRewrite can not be repeated
		// URLRewrite can not set with RequestRedirect
		if canaryFiler.URLRewrite != nil {
			newFilters = lo.Reject(newFilters, func(filter gatewayapiv1.HTTPRouteFilter, _ int) bool {
				return filter.URLRewrite != nil || filter.RequestRedirect != nil
			})
		}
		newFilters = append(newFilters, canaryFiler)
	}

	return newFilters
}

func getCanaryRuleName(i int) string {
	return fmt.Sprintf("%d.canary.rollout.kusionstack.io", i)
}
