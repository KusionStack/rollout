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
)

type IBackend interface {
	GetBackendObject() client.Object
	// todo: discussion: maybe Fork can be replaced by Create/Delete, and using Create/Delete to check if ready or deleted
	ForkStable(stableName string) client.Object
	ForkCanary(canaryName string) client.Object
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
