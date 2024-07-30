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
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func newTestApplyPartitionObject(total int32, updated int32) *appsv1.StatefulSet {
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

func Test_releaseControl_ApplyPartition(t *testing.T) {
	tests := []struct {
		name        string
		object      *appsv1.StatefulSet
		input       intstr.IntOrString
		checkResult func(assert assert.Assertions, object *appsv1.StatefulSet, err error)
	}{
		{
			name:   "total 10, want to update 1",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromInt(1),
			checkResult: func(assert assert.Assertions, object *appsv1.StatefulSet, err error) {
				assert.Nil(err)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate.Partition)
				partition := object.Spec.UpdateStrategy.RollingUpdate.Partition
				if assert.NotNil(partition) {
					assert.EqualValues(9, *partition)
				}
			},
		},
		{
			name:   "total 10, want to update 60%",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromString("60%"),
			checkResult: func(assert assert.Assertions, object *appsv1.StatefulSet, err error) {
				assert.Nil(err)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate.Partition)
				partition := object.Spec.UpdateStrategy.RollingUpdate.Partition
				if assert.NotNil(partition) {
					assert.EqualValues(4, *partition)
				}
			},
		},
		{
			name:   "total 10, updated 9, want to update 50%",
			object: newTestApplyPartitionObject(10, 9),
			input:  intstr.FromString("50%"),
			checkResult: func(assert assert.Assertions, object *appsv1.StatefulSet, err error) {
				assert.Nil(err)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate)
				assert.NotNil(object.Spec.UpdateStrategy.RollingUpdate.Partition)
				partition := object.Spec.UpdateStrategy.RollingUpdate.Partition
				if assert.NotNil(partition) {
					assert.EqualValues(1, *partition)
				}
			},
		},
		{
			name:   "total 10, want to update 100%",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromString("100%"),
			checkResult: func(assert assert.Assertions, object *appsv1.StatefulSet, err error) {
				assert.Nil(err)
				assert.Nil(object.Spec.UpdateStrategy.RollingUpdate)
			},
		},
		{
			name:   "total 10, want to update 11",
			object: newTestApplyPartitionObject(10, 0),
			input:  intstr.FromInt(11),
			checkResult: func(assert assert.Assertions, object *appsv1.StatefulSet, err error) {
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
