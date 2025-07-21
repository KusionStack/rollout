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
	"kusionstack.io/rollout/pkg/trafficrouting/backend"
	"kusionstack.io/rollout/pkg/trafficrouting/backend/inferencepool"
	"kusionstack.io/rollout/pkg/trafficrouting/backend/service"
)

const (
	BackendRegistryName = "backend-registry"
)

var Backends = NewBackendRegistry()

type BackendRegistry interface {
	genericregistry.Registry[schema.GroupVersionKind, backend.InClusterBackend]
}

func NewBackendRegistry() BackendRegistry {
	return genericregistry.New[schema.GroupVersionKind, backend.InClusterBackend]()
}

func InitBackendRegistry(mgr manager.Manager) (bool, error) {
	Backends.Register(service.GVK, service.New())
	Backends.Register(inferencepool.GVK, inferencepool.New())
	return true, nil
}
