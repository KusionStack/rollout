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
//

package route

import (
	"context"

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/utils/accessor"
)

type Route interface {
	accessor.ObjectAccessor

	// GetController returns a RouteController to manage the route.
	GetController(client client.Client, br *rolloutv1alpha1.BackendRouting, route client.Object, routeStatus rolloutv1alpha1.BackendRouteStatus) (RouteController, error)
}

type RouteController interface {
	GetRoute() client.Object

	Initialize(ctx context.Context) error
	Reset(ctx context.Context) error

	AddCanary(ctx context.Context) error
	DeleteCanary(ctx context.Context) error
}
