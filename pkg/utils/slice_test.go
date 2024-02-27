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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
)

func Test_SliceTopologyInfoEqual(t *testing.T) {
	a := []v1alpha1.TopologyInfo{
		{
			BackendRoutingName: "br1",
			WorkloadRef: v1alpha1.CrossClusterObjectNameReference{
				Name:    "workload1",
				Cluster: "cluster1",
			},
		},
		{
			BackendRoutingName: "br2",
			WorkloadRef: v1alpha1.CrossClusterObjectNameReference{
				Name:    "workload2",
				Cluster: "cluster2",
			},
		},
		{
			BackendRoutingName: "br2",
			WorkloadRef: v1alpha1.CrossClusterObjectNameReference{
				Name:    "workload3",
				Cluster: "cluster3",
			},
		},
		{
			BackendRoutingName: "br2",
			WorkloadRef: v1alpha1.CrossClusterObjectNameReference{
				Name:    "workload3",
				Cluster: "cluster4",
			},
		},
	}

	b := []v1alpha1.TopologyInfo{
		{
			BackendRoutingName: "br2",
			WorkloadRef: v1alpha1.CrossClusterObjectNameReference{
				Name:    "workload3",
				Cluster: "cluster4",
			},
		},
		{
			BackendRoutingName: "br2",
			WorkloadRef: v1alpha1.CrossClusterObjectNameReference{
				Name:    "workload2",
				Cluster: "cluster2",
			},
		},
		{
			BackendRoutingName: "br2",
			WorkloadRef: v1alpha1.CrossClusterObjectNameReference{
				Name:    "workload3",
				Cluster: "cluster3",
			},
		},
		{
			BackendRoutingName: "br1",
			WorkloadRef: v1alpha1.CrossClusterObjectNameReference{
				Name:    "workload1",
				Cluster: "cluster1",
			},
		},
	}

	assert.True(t, SliceTopologyInfoEqual(a, b))

	b[3].WorkloadRef.Cluster = "cluster"
	assert.False(t, SliceTopologyInfoEqual(a, b))
}

func Test_SliceEqualWithoutOrder(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"a", "c", "b"}

	less := func(x, y string) bool { return x < y }
	assert.True(t, cmp.Equal(a, b, cmpopts.SortSlices(less)))

	a = append(a, "a")
	b = append(b, "a")
	assert.True(t, cmp.Equal(a, b, cmpopts.SortSlices(less)))

	a = append(a, "aa")
	b = append(b, "a")
	assert.False(t, cmp.Equal(a, b, cmpopts.SortSlices(less)))

	type Item struct {
		A string
		B string
	}
	structA := []Item{
		{
			A: "a",
			B: "b",
		},
		{
			A: "aa",
			B: "bb",
		},
	}

	structB := []Item{
		{
			A: "aa",
			B: "bb",
		},
		{
			A: "a",
			B: "b",
		},
	}

	lessStruct := func(x, y Item) bool {
		if x.A < y.A {
			return true
		} else if x.A > y.A {
			return false
		} else {
			if x.B < y.B {
				return true
			} else {
				return false
			}
		}
	}

	assert.True(t, cmp.Equal(structA, structB, cmpopts.SortSlices(lessStruct)))

	structB[0] = Item{
		B: "bb",
		A: "aa",
	}

	assert.True(t, cmp.Equal(structA, structB, cmpopts.SortSlices(lessStruct)))

	structB[0] = Item{
		B: "bb",
		A: "a",
	}
	assert.False(t, cmp.Equal(structA, structB, cmpopts.SortSlices(lessStruct)))
}
