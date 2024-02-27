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

package utils

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
)

func SliceTopologyInfoEqual(a, b []v1alpha1.TopologyInfo) bool {
	return cmp.Equal(a, b, cmpopts.SortSlices(func(a, b v1alpha1.TopologyInfo) bool {
		if a.BackendRoutingName < b.BackendRoutingName {
			return true
		} else if a.BackendRoutingName > b.BackendRoutingName {
			return false
		} else {
			if a.WorkloadRef.Name < b.WorkloadRef.Name {
				return true
			} else if a.WorkloadRef.Name > b.WorkloadRef.Name {
				return false
			} else {
				if a.WorkloadRef.Cluster < b.WorkloadRef.Cluster {
					return true
				} else {
					return false
				}
			}
		}
	}))
}
