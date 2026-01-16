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

package rolloutrun

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

func TestRolloutRunStatusDeepEqual(t *testing.T) {
	now := time.Now()
	mt1 := metav1.NewTime(now)
	mt2 := metav1.NewTime(now)
	a := rolloutv1alpha1.RolloutRunStatus{
		ObservedGeneration: 1,
		Phase:              "TEST",
		LastUpdateTime:     &mt1,
		BatchStatus: &rolloutv1alpha1.RolloutRunBatchStatus{
			Records: []rolloutv1alpha1.RolloutRunStepStatus{
				{
					Index:     ptr.To[int32](0),
					StartTime: &mt1,
					Webhooks: []rolloutv1alpha1.RolloutWebhookStatus{
						{
							Name:     "PreBatchOpsCheck",
							HookType: "PreBatchStepHook",
							CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
								Code:    "Processing",
								Reason:  "工单otrafficonaicloudservice-test不存在",
								Message: "工单otrafficonaicloudservice-test不存在",
							},
							StartTime: &mt1,
							State:     "Running",
						},
					},
				},
			},
		},
	}
	b := rolloutv1alpha1.RolloutRunStatus{
		ObservedGeneration: 1,
		Phase:              "TEST",
		LastUpdateTime:     &mt2,
		BatchStatus: &rolloutv1alpha1.RolloutRunBatchStatus{
			Records: []rolloutv1alpha1.RolloutRunStepStatus{
				{
					Index:     ptr.To[int32](0),
					StartTime: &mt2,
					Webhooks: []rolloutv1alpha1.RolloutWebhookStatus{
						{
							Name:     "PreBatchOpsCheck",
							HookType: "PreBatchStepHook",
							CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
								Code:    "Processing",
								Reason:  "工单otrafficonaicloudservice-test不存在",
								Message: "工单otrafficonaicloudservice-test不存在",
							},
							StartTime: &mt2,
							State:     "Running",
						},
					},
				},
			},
		},
	}

	assert.True(t, equality.Semantic.DeepEqual(a, b))
}
