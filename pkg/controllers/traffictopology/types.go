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

package traffictopology

import (
	"kusionstack.io/kube-api/rollout/v1alpha1"
	rsFrameController "kusionstack.io/resourceconsist/pkg/frame/controller"
)

var _ rsFrameController.IEmployer = TPEmployer{}

type TPEmployer struct {
	BackendRoutingName string
	BackendRouting     v1alpha1.BackendRouting
	Workloads          []v1alpha1.CrossClusterObjectNameReference
}

type TREmployerStatues struct {
	BackendRouting v1alpha1.BackendRouting
	Workloads      []v1alpha1.CrossClusterObjectNameReference
}

func (b TPEmployer) GetEmployerId() string {
	return b.BackendRoutingName
}

func (b TPEmployer) GetEmployerName() string {
	return b.BackendRoutingName
}

func (b TPEmployer) GetEmployerStatuses() interface{} {
	return TREmployerStatues{
		BackendRouting: b.BackendRouting,
		Workloads:      b.Workloads,
	}
}

// EmployerEqual only compare name now
// TODO compare spec, but spec might be changed by BackendRouting controller
func (b TPEmployer) EmployerEqual(employer rsFrameController.IEmployer) (bool, error) {
	return b.BackendRoutingName == employer.GetEmployerId(), nil
}
