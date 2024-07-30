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

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
)

func newTestApplyPartitionObject(total int32, updated int32) *operatingv1alpha1.CollaSet {
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

func Test_releaseControl_ApplyPartition(t *testing.T) {
	tests := []struct {
		name        string
		object      *operatingv1alpha1.CollaSet
		input       intstr.IntOrString
		checkResult func(assert assert.Assertions, object *operatingv1alpha1.CollaSet, err error)
	}{
		{
			name:   "total 10, want to update 1",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromInt(1),
			checkResult: func(assert assert.Assertions, object *operatingv1alpha1.CollaSet, err error) {
				assert.Nil(err)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition)
				partition := object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition
				if assert.NotNil(partition) {
					assert.EqualValues(9, *partition)
				}
			},
		},
		{
			name:   "total 10, want to update 60%",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromString("60%"),
			checkResult: func(assert assert.Assertions, object *operatingv1alpha1.CollaSet, err error) {
				assert.Nil(err)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition)
				partition := object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition
				if assert.NotNil(partition) {
					assert.EqualValues(4, *partition)
				}
			},
		},
		{
			name:   "total 10, updated 9, want to update 50%",
			object: newTestApplyPartitionObject(10, 9),
			input:  intstr.FromString("50%"),
			checkResult: func(assert assert.Assertions, object *operatingv1alpha1.CollaSet, err error) {
				assert.Nil(err)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition)
				partition := object.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition
				if assert.NotNil(partition) {
					assert.EqualValues(1, *partition)
				}
			},
		},
		{
			name:   "total 10, want to update 100%",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromString("100%"),
			checkResult: func(assert assert.Assertions, object *operatingv1alpha1.CollaSet, err error) {
				assert.Nil(err)
				assert.Nil(object.Spec.UpdateStrategy.RollingUpdate)
			},
		},
		{
			name:   "total 10, want to update 11",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromInt(11),
			checkResult: func(assert assert.Assertions, object *operatingv1alpha1.CollaSet, err error) {
				assert.Nil(err)
				assert.Nil(object.Spec.UpdateStrategy.RollingUpdate)
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			c := &accessorImpl{}
			err := c.ApplyPartition(tt.object, tt.input)
			tt.checkResult(*assert.New(t), tt.object, err)
		})
	}
}
