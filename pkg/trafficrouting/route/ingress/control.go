// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ingress

import (
	"context"
	"strconv"
	"strings"

	"github.com/samber/lo"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	clientutil "kusionstack.io/kube-utils/client"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	"kusionstack.io/rollout/pkg/trafficrouting/route"
)

var _ route.RouteController = &ingressControl{}

type ingressControl struct {
	client         client.Client
	backendrouting *rolloutv1alpha1.BackendRouting
	routeObj       *networkingv1.Ingress
	routeStatus    rolloutv1alpha1.BackendRouteStatus
}

func (r *ingressControl) GetRoute() client.Object {
	return r.routeObj
}

func (c *ingressControl) getCluster() string {
	return c.routeStatus.Cluster
}

func (c *ingressControl) AddCanary(ctx context.Context) error {
	ingress := c.routeObj

	strategy := c.backendrouting.Spec.Forwarding.HTTP.Canary

	annosCanaryNeedCheck := map[string]string{
		AnnoCanary:                 "true",
		AnnoCanaryWeight:           "",
		AnnoCanaryHeader:           "",
		AnnoCanaryHeaderValue:      "",
		AnnoMseCanaryQuery:         "",
		AnnoMseCanaryQueryValue:    "",
		AnnoMseReqHeaderCtrlUpdate: "",
		AnnoMseReqHeaderCtrlAdd:    "",
		AnnoMseReqHeaderCtrlRemove: "",
	}

	isMseIngress := ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == MseIngressClass

	if strategy != nil {
		if len(strategy.Matches) > 0 {
			if len(strategy.Matches[0].Headers) > 0 {
				annosCanaryNeedCheck[AnnoCanaryHeader] = string(strategy.Matches[0].Headers[0].Name)
				annosCanaryNeedCheck[AnnoCanaryHeaderValue] = strategy.Matches[0].Headers[0].Value
			}
			if isMseIngress && len(strategy.Matches[0].QueryParams) > 0 {
				annosCanaryNeedCheck[AnnoMseCanaryQuery] = string(strategy.Matches[0].QueryParams[0].Name)
				annosCanaryNeedCheck[AnnoMseCanaryQueryValue] = strategy.Matches[0].QueryParams[0].Value
			}
		} else if strategy.Weight != nil {
			annosCanaryNeedCheck[AnnoCanaryWeight] = strconv.Itoa(int(*strategy.Weight))
		}

		if isMseIngress && len(strategy.Filters) > 0 {
			filter, ok := lo.Find(strategy.Filters, func(item v1.HTTPRouteFilter) bool {
				return item.RequestHeaderModifier != nil
			})
			if ok {
				annoSet := generateMultiHeadersAnno(filter.RequestHeaderModifier.Set)
				if annoSet != "" {
					annosCanaryNeedCheck[AnnoMseReqHeaderCtrlUpdate] = annoSet
				}

				annoAdd := generateMultiHeadersAnno(filter.RequestHeaderModifier.Add)
				if annoAdd != "" {
					annosCanaryNeedCheck[AnnoMseReqHeaderCtrlAdd] = annoAdd
				}

				if len(filter.RequestHeaderModifier.Remove) > 0 {
					annosCanaryNeedCheck[AnnoMseReqHeaderCtrlRemove] = strings.Join(filter.RequestHeaderModifier.Remove, ",")
				}
			}
		}
	}

	canaryIgs := &networkingv1.Ingress{}
	canaryIgs.Name = c.canaryIngressName()
	canaryIgs.Namespace = ingress.Namespace

	forked := c.backendrouting.Spec.ForkedBackends

	_, err := controllerutil.CreateOrUpdate(clusterinfo.WithCluster(ctx, c.getCluster()), c.client, canaryIgs, func() error {
		canaryIgs.Spec = ingress.Spec

		if canaryIgs.Spec.DefaultBackend != nil {
			if canaryIgs.Spec.DefaultBackend.Service != nil && canaryIgs.Spec.DefaultBackend.Service.Name == forked.Stable.Name {
				canaryIgs.Spec.DefaultBackend.Service.Name = forked.Canary.Name
			}
			if canaryIgs.Spec.DefaultBackend.Resource != nil && canaryIgs.Spec.DefaultBackend.Resource.Name == forked.Stable.Name {
				canaryIgs.Spec.DefaultBackend.Resource.Name = forked.Canary.Name
			}
		}

		for idx, rule := range canaryIgs.Spec.Rules {
			for k, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil && path.Backend.Service.Name == forked.Stable.Name {
					path.Backend.Service.Name = forked.Canary.Name
				}
				if path.Backend.Resource != nil && path.Backend.Resource.Name == forked.Stable.Name {
					path.Backend.Resource.Name = forked.Canary.Name
				}
				canaryIgs.Spec.Rules[idx].HTTP.Paths[k] = path
			}
		}

		if canaryIgs.Annotations == nil {
			canaryIgs.Annotations = make(map[string]string)
		}
		for key, value := range annosCanaryNeedCheck {
			if value != "" {
				canaryIgs.Annotations[key] = value
			}
			if value == "" {
				delete(canaryIgs.Annotations, key)
			}
		}

		if canaryIgs.Labels == nil {
			canaryIgs.Labels = make(map[string]string)
		}
		canaryIgs.Labels[rolloutapi.LabelCanaryResource] = "true"
		return nil
	})

	return err
}

func (c *ingressControl) canaryIngressName() string {
	return c.routeObj.Name + "-canary"
}

func (c *ingressControl) DeleteCanary(ctx context.Context) error {
	canaryIgsName := c.canaryIngressName()
	canaryIgs := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.routeObj.Namespace,
			Name:      canaryIgsName,
		},
	}
	err := c.client.Delete(clusterinfo.WithCluster(ctx, c.getCluster()), canaryIgs)
	return client.IgnoreNotFound(err)
}

func (c *ingressControl) changeCanaryBackend(kind, from, to string) func(ingress *networkingv1.Ingress) error {
	return func(ingress *networkingv1.Ingress) error {
		if ingress.Spec.DefaultBackend != nil {
			backend := ingress.Spec.DefaultBackend
			switch kind {
			case "Service":
				if backend.Service != nil && backend.Service.Name == from {
					backend.Service.Name = to
				}
			default:
				if backend.Resource != nil && backend.Resource.Name == from {
					backend.Resource.Name = to
				}
			}
		}

		for _, rule := range ingress.Spec.Rules {
			for k := range rule.HTTP.Paths {
				backend := rule.HTTP.Paths[k].Backend
				switch kind {
				case "Service":
					if backend.Service != nil && backend.Service.Name == from {
						backend.Service.Name = to
					}
				default:
					if backend.Resource != nil && backend.Resource.Name == from {
						backend.Resource.Name = to
					}
				}
			}
		}
		return nil
	}
}

func (c *ingressControl) Initialize(ctx context.Context) error {
	to := c.routeStatus.Forwarding.Origin.BackendName
	modify := c.changeCanaryBackend(c.backendrouting.Spec.Backend.Kind, c.backendrouting.Spec.Backend.Name, to)
	_, err := clientutil.UpdateOnConflict(clusterinfo.WithCluster(ctx, c.getCluster()), c.client, c.client, c.routeObj, modify)
	return err
}

func (c *ingressControl) Reset(ctx context.Context) error {
	from := c.routeStatus.Forwarding.Origin.BackendName
	modify := c.changeCanaryBackend(c.backendrouting.Spec.Backend.Kind, from, c.backendrouting.Spec.Backend.Name)
	_, err := clientutil.UpdateOnConflict(clusterinfo.WithCluster(ctx, c.getCluster()), c.client, c.client, c.routeObj, modify)
	return err
}

func generateMultiHeadersAnno(headers []v1.HTTPHeader) string {
	if len(headers) == 0 {
		return ""
	}
	if len(headers) == 1 {
		return string(headers[0].Name) + " " + headers[0].Value
	}
	headersSli := make([]string, len(headers))
	for k, header := range headers {
		headersSli[k] = string(header.Name) + " " + header.Value
	}
	return "|\n" + strings.Join(headersSli, "\n")
}
