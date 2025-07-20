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
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/trafficrouting/route"
	"kusionstack.io/rollout/pkg/utils/accessor"
)

var GVK = networkingv1.SchemeGroupVersion.WithKind("Ingress")

var _ route.Route = &routeImpl{}

type routeImpl struct {
	accessor.ObjectAccessor
}

func New() route.Route {
	return &routeImpl{
		ObjectAccessor: accessor.NewObjectAccessor(
			GVK,
			&networkingv1.Ingress{},
			&networkingv1.IngressList{},
		),
	}
}

func (r *routeImpl) GetController(client client.Client, br *rolloutv1alpha1.BackendRouting, routeObj client.Object, routeStatus rolloutv1alpha1.BackendRouteStatus) (route.RouteController, error) {
	igs, ok := routeObj.(*networkingv1.Ingress)
	if !ok {
		return nil, fmt.Errorf("input route is not networkingv1.Ingress")
	}
	return &ingressControl{
		client:         client,
		backendrouting: br,
		routeObj:       igs,
		routeStatus:    routeStatus,
	}, nil
}
