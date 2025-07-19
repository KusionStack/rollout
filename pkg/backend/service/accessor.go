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
	"maps"

	corev1 "k8s.io/api/core/v1"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/backend"
	"kusionstack.io/rollout/pkg/utils/accessor"
)

var GVK = corev1.SchemeGroupVersion.WithKind("Service")

var _ backend.InClusterBackend = &accessorImpl{}

type accessorImpl struct {
	accessor.ObjectAccessor
}

func New() backend.InClusterBackend {
	return &accessorImpl{
		ObjectAccessor: accessor.NewObjectAccessor(GVK, &corev1.Service{}, &corev1.ServiceList{}),
	}
}

func (s *accessorImpl) Fork(original client.Object, config rolloutv1alpha1.ForkedBackend) client.Object {
	obj := original.(*corev1.Service)
	forkedbackend := &corev1.Service{}
	forkedbackend.Name = config.Name
	forkedbackend.Namespace = obj.Namespace
	forkedbackend.Spec.Ports = obj.Spec.Ports
	forkedbackend.Spec.Type = obj.Spec.Type
	forkedbackend.Spec.Selector = obj.Spec.Selector
	if forkedbackend.Spec.Selector == nil {
		forkedbackend.Spec.Selector = make(map[string]string)
	}
	maps.Copy(forkedbackend.Spec.Selector, config.ExtraLabelSelector)
	if forkedbackend.Labels == nil {
		forkedbackend.Labels = make(map[string]string)
	}
	maps.Copy(forkedbackend.Labels, config.ExtraLabelSelector)
	forkedbackend.Labels[rolloutapi.LabelTemporaryResource] = "true"
	return forkedbackend
}
