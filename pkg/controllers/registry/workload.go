/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package registry

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/collaset"
	"kusionstack.io/rollout/pkg/workload/statefulset"
)

const (
	WorkloadRegistryName = "workload-registry"
)

var Workloads = workload.NewRegistry()

func InitWorkloadRegistry(mgr manager.Manager) (bool, error) {
	Workloads.Register(collaset.GVK, collaset.New())
	Workloads.Register(statefulset.GVK, statefulset.New())
	return true, nil
}

func IsSupportedWorkload(gvk schema.GroupVersionKind) bool {
	_, err := Workloads.Get(gvk)
	return err == nil
}
