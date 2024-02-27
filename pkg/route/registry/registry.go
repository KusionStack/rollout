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

package registry

import (
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/kube-utils/multicluster"

	"kusionstack.io/rollout/pkg/route"
)

type routeRegistry struct {
	routes           sync.Map // map[schema.GroupVersionKind]Store
	logger           logr.Logger
	discoveryFed     discovery.DiscoveryInterface
	discoveryMembers multicluster.PartialCachedDiscoveryInterface
}

func (r *routeRegistry) SetupWithManger(mgr manager.Manager) {
	r.logger = mgr.GetLogger().WithName("routeRegistry")
	var fedDiscovery discovery.DiscoveryInterface
	var membersDiscovery multicluster.PartialCachedDiscoveryInterface
	c := mgr.GetClient()
	client, ok := c.(multicluster.MultiClusterDiscovery)
	if ok {
		fedDiscovery = client.FedDiscoveryInterface()
		membersDiscovery = client.MembersCachedDiscoveryInterface()
	} else {
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(mgr.GetConfig()))
		fedDiscovery = discoveryClient
		membersDiscovery = discoveryClient
	}
	r.discoveryFed = fedDiscovery
	r.discoveryMembers = membersDiscovery
}

func (r *routeRegistry) Register(store route.Store) {
	fmt.Println("xxxx", store.GroupVersionKind())
	r.routes.Store(store.GroupVersionKind(), store)
}

func (r *routeRegistry) Delete(gvk schema.GroupVersionKind) {
	r.routes.Delete(gvk)
}

func (r *routeRegistry) Get(gvk schema.GroupVersionKind) (route.Store, error) {
	value, ok := r.routes.Load(gvk)
	if !ok {
		return nil, fmt.Errorf("unregistered gvk(%s) in route registry", gvk.String())
	}

	return value.(route.Store), nil
}

var _ route.Registry = &routeRegistry{}

func NewRegistry() route.Registry {
	return &routeRegistry{
		routes: sync.Map{},
	}
}
