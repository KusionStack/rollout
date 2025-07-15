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

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/kube-api/rollout/v1alpha1"
)

type BackendChangeDetail struct {
	Src        string
	Dst        string
	Kind       string
	ApiVersion string
}

type IRoute interface {
	GetRouteObject() client.Object
	AddCanaryRoute(ctx context.Context, forwarding *v1alpha1.BackendForwarding) error
	RemoveCanaryRoute(ctx context.Context) error
	ChangeBackend(ctx context.Context, detail BackendChangeDetail) error
}

type Store interface {
	GroupVersionKind() schema.GroupVersionKind
	// NewObject returns a new instance of the route type
	NewObject() client.Object
	// Wrap get a client.Object and returns a route interface
	Wrap(cluster string, obj client.Object) (IRoute, error)
	// Get returns a wrapped route interface
	Get(ctx context.Context, cluster, namespace, name string) (IRoute, error)
}
