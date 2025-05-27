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
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
)

func newTestApplyPartitionObject(total, updated int32) *operatingv1alpha1.CollaSet {
	return &operatingv1alpha1.CollaSet{
		Spec: operatingv1alpha1.CollaSetSpec{
			Replicas: &total,
			UpdateStrategy: operatingv1alpha1.UpdateStrategy{
				RollingUpdate: &operatingv1alpha1.RollingUpdateCollaSetStrategy{
					ByPartition: &operatingv1alpha1.ByPartition{
						Partition: ptr.To(total - updated),
					},
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
		object      *operatingv1alpha1.CollaSet
		input       intstr.IntOrString
		checkResult func(object *operatingv1alpha1.CollaSet, err error)
	}{
		{
			name:   "total 10, want to update 1",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromInt(1),
			checkResult: func(object *operatingv1alpha1.CollaSet, err error) {
				s.Require().NoError(err)
				s.Require().NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				s.Require().NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition)
				if s.NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition) {
					s.EqualValues(9, *object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition)
				}
			},
		},
		{
			name:   "total 10, want to update 60%",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromString("60%"),
			checkResult: func(object *operatingv1alpha1.CollaSet, err error) {
				s.Require().NoError(err)
				s.Require().NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				s.Require().NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition)
				if s.NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition) {
					s.EqualValues(4, *object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition)
				}
			},
		},
		{
			name:   "total 10, updated 9, want to update 50%",
			object: newTestApplyPartitionObject(10, 9),
			input:  intstr.FromString("50%"),
			checkResult: func(object *operatingv1alpha1.CollaSet, err error) {
				s.Require().NoError(err)
				s.Require().NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				s.Require().NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition)
				if s.NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition) {
					s.EqualValues(1, *object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition)
				}
			},
		},
		{
			name:   "total 10, want to update 100%",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromString("100%"),
			checkResult: func(object *operatingv1alpha1.CollaSet, err error) {
				s.Require().NoError(err)
				s.Nil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition)
			},
		},
		{
			name:   "total 10, want to update 11",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromInt(11),
			checkResult: func(object *operatingv1alpha1.CollaSet, err error) {
				s.Require().NoError(err)
				s.Nil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition)
			},
		},
		{
			name: "should not change spec 1",
			object: &operatingv1alpha1.CollaSet{
				Spec: operatingv1alpha1.CollaSetSpec{
					Replicas: ptr.To[int32](10),
					UpdateStrategy: operatingv1alpha1.UpdateStrategy{
						RollingUpdate: nil,
					},
				},
			},
			input: intstr.FromInt(10),
			checkResult: func(object *operatingv1alpha1.CollaSet, err error) {
				s.Require().NoError(err)
				s.Nil(object.Spec.UpdateStrategy.RollingUpdate)
			},
		},
		{
			name: "should not change spec 2",
			object: &operatingv1alpha1.CollaSet{
				Spec: operatingv1alpha1.CollaSetSpec{
					Replicas: ptr.To[int32](10),
					UpdateStrategy: operatingv1alpha1.UpdateStrategy{
						RollingUpdate: &operatingv1alpha1.RollingUpdateCollaSetStrategy{
							ByPartition: nil,
						},
					},
				},
			},
			input: intstr.FromInt(10),
			checkResult: func(object *operatingv1alpha1.CollaSet, err error) {
				s.Require().NoError(err)
				if s.NotNil(object.Spec.UpdateStrategy.RollingUpdate) {
					s.Nil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition)
				}
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
