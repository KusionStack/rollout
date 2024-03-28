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

package backend

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/registry"
)

type IBackend interface {
	GetBackendObject() client.Object
	// todo: discussion: maybe Fork can be replaced by Create/Delete, and using Create/Delete to check if ready or deleted
	ForkStable(stableName, controllerName string) client.Object
	ForkCanary(canaryName, controllerName string) client.Object
}

type Store interface {
	GroupVersionKind() schema.GroupVersionKind
	// NewObject returns a new instance of the backend type
	NewObject() client.Object
	// Wrap get a client.Object and returns a backend interface
	Wrap(cluster string, obj client.Object) (IBackend, error)
	// Get returns a wrapped backend interface
	Get(ctx context.Context, cluster, namespace, name string) (IBackend, error)
}

type Registry = registry.Registry[schema.GroupVersionKind, Store]

// type Registry interface {
// 	// SetupWithManger initialize registry with manager.
// 	SetupWithManger(mgr manager.Manager)
// 	// Register add a new backend store for given gvk
// 	Register(store Store)
// 	// Delete delete a backend store for given gvk
// 	Delete(gvk schema.GroupVersionKind)
// 	// If the gvk is registered and supported by all member clusters, Get returns the backend rest store.
// 	Get(gvk schema.GroupVersionKind) (Store, error)
// }
