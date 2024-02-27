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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	rsFrameController "kusionstack.io/resourceconsist/pkg/frame/controller"
	"kusionstack.io/rollout/pkg/controllers/workloadregistry"
)

const ControllerName = "traffic-topology-controller"

//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=traffictopologies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=traffictopologies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=traffictopologies/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=backendroutings,verbs=get;list;watch;create;update;patch;delete

func InitFunc(mgr manager.Manager) (bool, error) {
	err := rsFrameController.AddToMgr(mgr, NewTPControllerAdapter(mgr.GetClient(), workloadregistry.DefaultRegistry))
	if err != nil {
		return false, err
	}
	return true, nil
}
