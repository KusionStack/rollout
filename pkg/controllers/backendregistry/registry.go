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

package backendregistry

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/backend/registry"
	"kusionstack.io/rollout/pkg/backend/service"
)

const (
	InitializerName = "__internal_backend_registry"
)

var (
	DefaultRegistry = registry.NewRegistry()
)

func InitFunc(mgr manager.Manager) (bool, error) {
	logger := mgr.GetLogger().WithName("init")
	logger.Info("initialize default backend registry")
	DefaultRegistry.Register(service.NewStorage(mgr))
	DefaultRegistry.SetupWithManger(mgr)
	return true, nil
}
