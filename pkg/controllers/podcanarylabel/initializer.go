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

package podcanarylabel

import (
	"kusionstack.io/kube-utils/controller/initializer"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/controllers/registry"
)

func InitFunc(mgr manager.Manager) (bool, error) {
	return initFunc(mgr, registry.Workloads)
}

func InitFuncWith(registry registry.WorkloadRegistry) initializer.InitFunc {
	return func(m manager.Manager) (enabled bool, err error) {
		return initFunc(m, registry)
	}
}

func initFunc(mgr manager.Manager, registry registry.WorkloadRegistry) (bool, error) {
	err := NewPodCanaryReconciler(mgr, registry).SetupWithManager(mgr)
	if err != nil {
		return false, err
	}
	return true, nil
}
