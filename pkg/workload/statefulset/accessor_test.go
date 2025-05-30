// Copyright 2025 The KusionStack Authors
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

package statefulset

import (
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"

	"kusionstack.io/rollout/pkg/workload"
)

type accessorTestSuite struct {
	suite.Suite
}

func (s *accessorTestSuite) Test_getStatus() {
	tests := []struct {
		name   string
		object *appsv1.StatefulSet
		want   workload.InfoStatus
	}{
		{
			name: "normal",
			object: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](10),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					CurrentRevision:    "v1",
					UpdateRevision:     "v1",
					Replicas:           10,
					CurrentReplicas:    4,
					UpdatedReplicas:    4,
					ReadyReplicas:      9,
					AvailableReplicas:  9,
				},
			},
			want: workload.InfoStatus{
				Replicas:                 10,
				ObservedGeneration:       1,
				StableRevision:           "v1",
				UpdatedRevision:          "v1",
				UpdatedReplicas:          4,
				UpdatedReadyReplicas:     4,
				UpdatedAvailableReplicas: 4,
			},
		},
		{
			name: "upgrading",
			object: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](10),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					CurrentRevision:    "v1",
					UpdateRevision:     "v2",
					Replicas:           10,
					CurrentReplicas:    4,
					UpdatedReplicas:    3,
					ReadyReplicas:      9,
					AvailableReplicas:  9,
				},
			},
			want: workload.InfoStatus{
				Replicas:                 10,
				ObservedGeneration:       1,
				StableRevision:           "v1",
				UpdatedRevision:          "v2",
				UpdatedReplicas:          3,
				UpdatedReadyReplicas:     3,
				UpdatedAvailableReplicas: 3,
			},
		},

		{
			name: "upgrading 2",
			object: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](10),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					CurrentRevision:    "v1",
					UpdateRevision:     "v2",
					Replicas:           10,
					CurrentReplicas:    4,
					UpdatedReplicas:    6,
					ReadyReplicas:      9,
					AvailableReplicas:  8,
				},
			},
			want: workload.InfoStatus{
				Replicas:                 10,
				ObservedGeneration:       1,
				StableRevision:           "v1",
				UpdatedRevision:          "v2",
				UpdatedReplicas:          6,
				UpdatedReadyReplicas:     5,
				UpdatedAvailableReplicas: 4,
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		s.Run(tt.name, func() {
			c := &accessorImpl{}
			got := c.getStatus(tt.object)
			s.Equal(tt.want, got)
		})
	}
}
