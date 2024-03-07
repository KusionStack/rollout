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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/route/ingress"
	"kusionstack.io/rollout/pkg/route/registry"
)

const (
	RouteRegistryName = "route-registry"
)

var Routes = registry.NewRegistry()

func InitRouteRegistry(mgr manager.Manager) (bool, error) {
	Routes.Register(ingress.NewStorage(mgr))
	Routes.SetupWithManger(mgr)
	return true, nil
}
