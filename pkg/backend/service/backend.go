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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/apis/rollout"
	"kusionstack.io/rollout/pkg/backend"
)

var GVK = corev1.SchemeGroupVersion.WithKind("Service")

type serviceBackend struct {
	client client.Client
	obj    *corev1.Service
}

var _ backend.IBackend = &serviceBackend{}

func (s *serviceBackend) GetBackendObject() client.Object {
	return s.obj
}

func (s *serviceBackend) ForkCanary(canaryName, controllerName string) client.Object {
	canaryBackend := &corev1.Service{}
	canaryBackend.Name = canaryName
	canaryBackend.Namespace = s.obj.Namespace
	canaryBackend.Labels = map[string]string{
		rollout.LabelControl:   "true",
		rollout.LabelCreatedBy: controllerName,
	}
	canaryBackend.Spec.Ports = s.obj.Spec.Ports
	canaryBackend.Spec.Selector = s.obj.Spec.Selector
	if canaryBackend.Spec.Selector == nil {
		canaryBackend.Spec.Selector = make(map[string]string)
	}
	canaryBackend.Spec.Selector[rollout.LabelPodRevision] = rollout.LabelValuePodRevisionCanary
	return canaryBackend
}

func (s *serviceBackend) ForkStable(stableName, controllerName string) client.Object {
	stableBackend := &corev1.Service{}
	stableBackend.Name = stableName
	stableBackend.Namespace = s.obj.Namespace
	stableBackend.Labels = map[string]string{
		rollout.LabelControl:   "true",
		rollout.LabelCreatedBy: controllerName,
	}
	stableBackend.Spec.Ports = s.obj.Spec.Ports
	stableBackend.Spec.Selector = s.obj.Spec.Selector
	if stableBackend.Spec.Selector == nil {
		stableBackend.Spec.Selector = make(map[string]string)
	}
	stableBackend.Spec.Selector[rollout.LabelPodRevision] = rollout.LabelValuePodRevisionBase
	return stableBackend
}
