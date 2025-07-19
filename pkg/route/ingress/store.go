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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/route"
	"kusionstack.io/rollout/pkg/utils/accessor"
)

type IgsStore struct {
	accessor.ObjectAccessor
}

func NewStorage() route.Route {
	return &IgsStore{
		ObjectAccessor: accessor.NewObjectAccessor(
			GVK,
			&networkingv1.Ingress{},
			&networkingv1.IngressList{},
		),
	}
}

func (i *IgsStore) Wrap(client client.Client, cluster string, route client.Object) (route.RouteControl, error) {
	igs, ok := route.(*networkingv1.Ingress)
	if !ok {
		return nil, fmt.Errorf("not Ingress")
	}
	return &ingressRoute{
		client:  client,
		obj:     igs,
		cluster: cluster,
	}, nil
}

var _ route.Route = &IgsStore{}
