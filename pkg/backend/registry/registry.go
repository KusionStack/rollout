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
	"kusionstack.io/kube-utils/multicluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/backend"
)

type backendRegistry struct {
	backends         sync.Map // map[schema.GroupVersionKind]Store
	logger           logr.Logger
	discoveryFed     discovery.DiscoveryInterface
	discoveryMembers multicluster.PartialCachedDiscoveryInterface
}

func (b *backendRegistry) SetupWithManger(mgr manager.Manager) {
	b.logger = mgr.GetLogger().WithName("registry").WithName("backend")
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
	b.discoveryFed = fedDiscovery
	b.discoveryMembers = membersDiscovery
}

func (b *backendRegistry) Register(store backend.Store) {
	b.backends.Store(store.GroupVersionKind(), store)
}

func (b *backendRegistry) Delete(gvk schema.GroupVersionKind) {
	b.backends.Delete(gvk)
}

func (b *backendRegistry) Get(gvk schema.GroupVersionKind) (backend.Store, error) {
	value, ok := b.backends.Load(gvk)
	if !ok {
		return nil, fmt.Errorf("unregistered gvk(%s) in backend registry", gvk.String())
	}

	return value.(backend.Store), nil
}

var _ backend.Registry = &backendRegistry{}

func NewRegistry() backend.Registry {
	return &backendRegistry{
		backends: sync.Map{},
	}
}
