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
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/trafficrouting/backend"
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

func (s *accessorImpl) Fork(origin client.Object, config rolloutv1alpha1.ForkedBackend) client.Object {
	obj := origin.(*corev1.Service)
	forkedObj := &corev1.Service{}
	// fork metadata
	forkedObj.ObjectMeta = backend.ForkObjectMeta(obj, config.Name)
	// fork spec
	forkedObj.Spec.Ports = obj.Spec.Ports
	forkedObj.Spec.Type = obj.Spec.Type
	// merge selector
	forkedObj.Spec.Selector = lo.Assign(obj.Spec.Selector, config.ExtraLabelSelector)
	return forkedObj
}
