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

package service

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/backend"
)

type SvcStore struct {
	client client.Client
}

func NewStorage(mgr manager.Manager) backend.Store {
	return &SvcStore{
		client: mgr.GetClient(),
	}
}

func (s *SvcStore) GroupVersionKind() schema.GroupVersionKind {
	return GVK
}

func (s *SvcStore) NewObject() client.Object {
	return &corev1.Service{}
}

func (s *SvcStore) NewObjectList() client.ObjectList {
	return &corev1.ServiceList{}
}

func (s *SvcStore) Wrap(cluster string, obj client.Object) (backend.IBackend, error) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return nil, fmt.Errorf("not Service")
	}
	return &serviceBackend{
		client: s.client,
		obj:    svc,
	}, nil
}

func (s *SvcStore) Get(ctx context.Context, cluster, namespace, name string) (backend.IBackend, error) {
	var svc corev1.Service
	err := s.client.Get(clusterinfo.WithCluster(ctx, cluster), types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, &svc)
	if err != nil {
		return nil, err
	}
	return s.Wrap(cluster, &svc)
}

var _ backend.Store = &SvcStore{}
