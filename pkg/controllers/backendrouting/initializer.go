/**
 * Copyright 2023 The KusionStack Authors
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

package backendrouting

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/backend"
	"kusionstack.io/rollout/pkg/controllers/backendregistry"
	"kusionstack.io/rollout/pkg/controllers/routeregistry"
	"kusionstack.io/rollout/pkg/route"
)

func InitFunc(mgr manager.Manager) (bool, error) {
	return initFunc(mgr, backendregistry.DefaultRegistry, routeregistry.DefaultRegistry)
}

func initFunc(mgr manager.Manager, backendRegistry backend.Registry, routeRegistry route.Registry) (bool, error) {
	err := AddToMgr(mgr, backendregistry.DefaultRegistry, routeregistry.DefaultRegistry)
	if err != nil {
		return false, err
	}
	return true, nil
}
