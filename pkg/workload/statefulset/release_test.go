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

package statefulset

import (
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func newTestApplyPartitionObject(total, updated int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(total),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: ptr.To(total - updated),
				},
			},
		},
	}
}

type releaseControlTestSuite struct {
	suite.Suite
}

func (s *releaseControlTestSuite) Test_ApplyPartition() {
	tests := []struct {
		name        string
		object      *appsv1.StatefulSet
		input       intstr.IntOrString
		checkResult func(object *appsv1.StatefulSet, err error)
	}{
		{
			name:   "total 10, want to update 1",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromInt(1),
			checkResult: func(object *appsv1.StatefulSet, err error) {
				s.Require().NoError(err)
				s.Require().NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				if s.NotNil(object.Spec.UpdateStrategy.RollingUpdate.Partition) {
					s.EqualValues(9, *object.Spec.UpdateStrategy.RollingUpdate.Partition)
				}
			},
		},
		{
			name:   "total 10, want to update 60%",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromString("60%"),
			checkResult: func(object *appsv1.StatefulSet, err error) {
				s.Require().NoError(err)
				s.Require().NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				if s.NotNil(object.Spec.UpdateStrategy.RollingUpdate.Partition) {
					s.EqualValues(4, *object.Spec.UpdateStrategy.RollingUpdate.Partition)
				}
			},
		},
		{
			name:   "total 10, updated 9, want to update 50%",
			object: newTestApplyPartitionObject(10, 9),
			input:  intstr.FromString("50%"),
			checkResult: func(object *appsv1.StatefulSet, err error) {
				s.Require().NoError(err)
				s.Require().NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				if s.NotNil(object.Spec.UpdateStrategy.RollingUpdate.Partition) {
					s.EqualValues(1, *object.Spec.UpdateStrategy.RollingUpdate.Partition)
				}
			},
		},
		{
			name:   "total 10, want to update 100%",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromString("100%"),
			checkResult: func(object *appsv1.StatefulSet, err error) {
				s.Require().NoError(err)
				s.Nil(object.Spec.UpdateStrategy.RollingUpdate)
			},
		},
		{
			name:   "total 10, want to update 11",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromInt(11),
			checkResult: func(object *appsv1.StatefulSet, err error) {
				s.Require().NoError(err)
				s.Nil(object.Spec.UpdateStrategy.RollingUpdate)
			},
		},
		{
			name: "should not change spec",
			object: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](10),
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						RollingUpdate: nil,
					},
				},
			},
			input: intstr.FromInt(10),
			checkResult: func(object *appsv1.StatefulSet, err error) {
				s.Require().NoError(err)
				s.Nil(object.Spec.UpdateStrategy.RollingUpdate)
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		s.Run(tt.name, func() {
			c := &accessorImpl{}
			err := c.ApplyPartition(tt.object, tt.input)
			tt.checkResult(tt.object, err)
		})
	}
}
