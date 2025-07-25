/**
 * Copyright 2024 The KusionStack Authors
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

package registry

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/genericregistry"
	"kusionstack.io/rollout/pkg/trafficrouting/route"
	"kusionstack.io/rollout/pkg/trafficrouting/route/httproute"
	"kusionstack.io/rollout/pkg/trafficrouting/route/ingress"
)

const (
	RouteRegistryName = "route-registry"
)

var Routes = NewRouteRegistry()

type RouteRegistry interface {
	genericregistry.Registry[schema.GroupVersionKind, route.Route]
}

func NewRouteRegistry() RouteRegistry {
	return genericregistry.New[schema.GroupVersionKind, route.Route]()
}

func InitRouteRegistry(mgr manager.Manager) (bool, error) {
	Routes.Register(ingress.GVK, ingress.New())
	Routes.Register(httproute.GVK, httproute.New())
	return true, nil
}

func IsSupportedRoute(gvk schema.GroupVersionKind) bool {
	_, err := Routes.Get(gvk)
	return err == nil
}
