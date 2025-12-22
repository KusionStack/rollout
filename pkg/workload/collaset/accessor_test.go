/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package collaset

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"

	"kusionstack.io/rollout/pkg/workload"
)

type accessorTestSuite struct {
	suite.Suite
}

func TestAccessorTestSuite(t *testing.T) {
	suite.Run(t, new(accessorTestSuite))
}

func (s *accessorTestSuite) Test_getStatus() {
	tests := []struct {
		name     string
		obj      *operatingv1alpha1.CollaSet
		expected workload.InfoStatus
	}{
		{
			name: "normal case with all fields populated",
			obj: &operatingv1alpha1.CollaSet{
				Spec: operatingv1alpha1.CollaSetSpec{
					Replicas: ptr.To[int32](10),
				},
				Status: operatingv1alpha1.CollaSetStatus{
					CurrentRevision:          "revision-v1",
					UpdatedRevision:          "revision-v2",
					ObservedGeneration:       5,
					Replicas:                 10,
					ReadyReplicas:            9,
					AvailableReplicas:        8,
					UpdatedReplicas:          6,
					UpdatedReadyReplicas:     5,
					UpdatedAvailableReplicas: 4,
				},
			},
			expected: workload.InfoStatus{
				StableRevision:           "revision-v1",
				UpdatedRevision:          "revision-v2",
				ObservedGeneration:       5,
				DesiredReplicas:          10,
				ObservedReplicas:         10,
				ReadyReplicas:            9,
				AvailableReplicas:        8,
				UpdatedReplicas:          6,
				UpdatedReadyReplicas:     5,
				UpdatedAvailableReplicas: 4,
			},
		},
		{
			name: "zero values for all fields",
			obj: &operatingv1alpha1.CollaSet{
				Spec: operatingv1alpha1.CollaSetSpec{
					Replicas: ptr.To[int32](0),
				},
				Status: operatingv1alpha1.CollaSetStatus{
					CurrentRevision:          "",
					UpdatedRevision:          "",
					ObservedGeneration:       0,
					Replicas:                 0,
					ReadyReplicas:            0,
					AvailableReplicas:        0,
					UpdatedReplicas:          0,
					UpdatedReadyReplicas:     0,
					UpdatedAvailableReplicas: 0,
				},
			},
			expected: workload.InfoStatus{
				StableRevision:           "",
				UpdatedRevision:          "",
				ObservedGeneration:       0,
				DesiredReplicas:          0,
				ObservedReplicas:         0,
				ReadyReplicas:            0,
				AvailableReplicas:        0,
				UpdatedReplicas:          0,
				UpdatedReadyReplicas:     0,
				UpdatedAvailableReplicas: 0,
			},
		},
		{
			name: "nil replicas pointer should default to 0",
			obj: &operatingv1alpha1.CollaSet{
				Spec: operatingv1alpha1.CollaSetSpec{
					Replicas: nil,
				},
				Status: operatingv1alpha1.CollaSetStatus{
					CurrentRevision:          "rev1",
					UpdatedRevision:          "rev2",
					ObservedGeneration:       1,
					Replicas:                 3,
					ReadyReplicas:            3,
					AvailableReplicas:        3,
					UpdatedReplicas:          2,
					UpdatedReadyReplicas:     2,
					UpdatedAvailableReplicas: 2,
				},
			},
			expected: workload.InfoStatus{
				StableRevision:           "rev1",
				UpdatedRevision:          "rev2",
				ObservedGeneration:       1,
				DesiredReplicas:          0,
				ObservedReplicas:         3,
				ReadyReplicas:            3,
				AvailableReplicas:        3,
				UpdatedReplicas:          2,
				UpdatedReadyReplicas:     2,
				UpdatedAvailableReplicas: 2,
			},
		},
		{
			name: "partial fields populated",
			obj: &operatingv1alpha1.CollaSet{
				Spec: operatingv1alpha1.CollaSetSpec{
					Replicas: ptr.To[int32](5),
				},
				Status: operatingv1alpha1.CollaSetStatus{
					CurrentRevision:   "stable-rev",
					UpdatedRevision:   "updated-rev",
					Replicas:          5,
					ReadyReplicas:     5,
					AvailableReplicas: 4,
					// Other fields intentionally left at zero values
				},
			},
			expected: workload.InfoStatus{
				StableRevision:           "stable-rev",
				UpdatedRevision:          "updated-rev",
				ObservedGeneration:       0,
				DesiredReplicas:          5,
				ObservedReplicas:         5,
				ReadyReplicas:            5,
				AvailableReplicas:        4,
				UpdatedReplicas:          0,
				UpdatedReadyReplicas:     0,
				UpdatedAvailableReplicas: 0,
			},
		},
		{
			name: "large values",
			obj: &operatingv1alpha1.CollaSet{
				Spec: operatingv1alpha1.CollaSetSpec{
					Replicas: ptr.To[int32](1000),
				},
				Status: operatingv1alpha1.CollaSetStatus{
					CurrentRevision:          "revision-999",
					UpdatedRevision:          "revision-1000",
					ObservedGeneration:       9999,
					Replicas:                 1000,
					ReadyReplicas:            999,
					AvailableReplicas:        950,
					UpdatedReplicas:          800,
					UpdatedReadyReplicas:     750,
					UpdatedAvailableReplicas: 700,
				},
			},
			expected: workload.InfoStatus{
				StableRevision:           "revision-999",
				UpdatedRevision:          "revision-1000",
				ObservedGeneration:       9999,
				DesiredReplicas:          1000,
				ObservedReplicas:         1000,
				ReadyReplicas:            999,
				AvailableReplicas:        950,
				UpdatedReplicas:          800,
				UpdatedReadyReplicas:     750,
				UpdatedAvailableReplicas: 700,
			},
		},
	}

	accessor := &accessorImpl{}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := accessor.getStatus(tt.obj)
			s.Equal(tt.expected, result)
		})
	}
}
