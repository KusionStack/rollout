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

	appsv1 "k8s.io/api/apps/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	v1 "sigs.k8s.io/gateway-api/apis/v1"

	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/route"
)

var GVK = appsv1.SchemeGroupVersion.WithKind("Ingress")

type ingressRoute struct {
	client  client.Client
	obj     *networkingv1.Ingress
	cluster string
}

func (i *ingressRoute) GetRouteObject() client.Object {
	return i.obj
}

func (i *ingressRoute) AddCanaryRoute(ctx context.Context, strategy v1alpha1.TrafficStrategy) error {
	igs := i.obj

	annosCanaryNeedCheck := map[string]string{
		AnnoCanaryWeight:           "",
		AnnoCanaryHeader:           "",
		AnnoCanaryHeaderValue:      "",
		AnnoMseCanaryQuery:         "",
		AnnoMseCanaryQueryValue:    "",
		AnnoMseReqHeaderCtrlUpdate: "",
		AnnoMseReqHeaderCtrlAdd:    "",
		AnnoMseReqHeaderCtrlRemove: "",
	}
	if strategy.Weight != nil {
		annosCanaryNeedCheck[AnnoCanaryWeight] = strconv.Itoa(int(*strategy.Weight))
	}

	if strategy.HTTPRule != nil {
		if len(strategy.HTTPRule.Matches) > 0 {
			if len(strategy.HTTPRule.Matches[0].Headers) > 0 {
				annosCanaryNeedCheck[AnnoCanaryHeader] = string(strategy.HTTPRule.Matches[0].Headers[0].Name)
				annosCanaryNeedCheck[AnnoCanaryHeaderValue] = strategy.HTTPRule.Matches[0].Headers[0].Value
			}
			if len(strategy.HTTPRule.Matches[0].QueryParams) > 0 {
				annosCanaryNeedCheck[AnnoMseCanaryQuery] = string(strategy.HTTPRule.Matches[0].QueryParams[0].Name)
				annosCanaryNeedCheck[AnnoMseCanaryQueryValue] = strategy.HTTPRule.Matches[0].QueryParams[0].Value
			}
		}
		if strategy.HTTPRule.Filter.RequestHeaderModifier != nil {
			annoSet := generateMultiHeadersAnno(strategy.HTTPRule.Filter.RequestHeaderModifier.Set)
			if annoSet != "" {
				annosCanaryNeedCheck[AnnoMseReqHeaderCtrlUpdate] = annoSet
			}

			annoAdd := generateMultiHeadersAnno(strategy.HTTPRule.Filter.RequestHeaderModifier.Add)
			if annoAdd != "" {
				annosCanaryNeedCheck[AnnoMseReqHeaderCtrlAdd] = annoAdd
			}

			if len(strategy.HTTPRule.Filter.RequestHeaderModifier.Remove) > 0 {
				annosCanaryNeedCheck[AnnoMseReqHeaderCtrlRemove] = strings.Join(strategy.HTTPRule.Filter.RequestHeaderModifier.Remove, ",")
			}
		}
	}

	canaryIgs := &networkingv1.Ingress{}
	canaryIgs.Name = igs.Name + "-canary"
	canaryIgs.Namespace = igs.Namespace

	_, err := controllerutil.CreateOrUpdate(clusterinfo.WithCluster(ctx, i.cluster), i.client, canaryIgs, func() error {
		canaryIgs.Spec = igs.Spec
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
		return nil
	})

	return err
}

func (i *ingressRoute) RemoveCanaryRoute(ctx context.Context) error {
	canaryIgsName := i.obj.Name + "-canary"
	canaryIgs := &networkingv1.Ingress{}
	err := i.client.Get(clusterinfo.WithCluster(ctx, i.cluster), types.NamespacedName{
		Namespace: i.obj.Namespace,
		Name:      canaryIgsName,
	}, canaryIgs)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		return nil
	}
	return i.client.Delete(clusterinfo.WithCluster(ctx, i.cluster), canaryIgs)
}

func (i *ingressRoute) ChangeBackend(ctx context.Context, detail route.BackendChangeDetail) error {
	igs := i.obj
	needChange := false

	if igs.Spec.DefaultBackend != nil {
		if detail.Kind == "Service" {
			if igs.Spec.DefaultBackend.Service != nil && igs.Spec.DefaultBackend.Service.Name == detail.Src {
				igs.Spec.DefaultBackend.Service.Name = detail.Dst
				needChange = true
			}
		} else {
			if igs.Spec.DefaultBackend.Resource != nil && igs.Spec.DefaultBackend.Resource.Kind == detail.Kind &&
				igs.Spec.DefaultBackend.Resource.Name == detail.Src {
				igs.Spec.DefaultBackend.Resource.Name = detail.Dst
				needChange = true
			}
		}
	}

	for idx, rule := range igs.Spec.Rules {
		for k, path := range rule.HTTP.Paths {
			if detail.Kind == "Service" {
				if path.Backend.Service != nil && path.Backend.Service.Name == detail.Src {
					path.Backend.Service.Name = detail.Dst
					igs.Spec.Rules[idx].HTTP.Paths[k] = path
					needChange = true
				}
			} else {
				if path.Backend.Resource != nil && path.Backend.Resource.Kind == detail.Kind &&
					path.Backend.Resource.Name == detail.Src {
					path.Backend.Resource.Name = detail.Dst
					igs.Spec.Rules[idx].HTTP.Paths[k] = path
					needChange = true
				}
			}
		}
	}

	if needChange {
		return i.client.Update(clusterinfo.WithCluster(ctx, i.cluster), igs)
	}

	return nil
}

var _ route.IRoute = &ingressRoute{}

func generateMultiHeadersAnno(headers []v1.HTTPHeader) string {
	if headers == nil || len(headers) == 0 {
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
