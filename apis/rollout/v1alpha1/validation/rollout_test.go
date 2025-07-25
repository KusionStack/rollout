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

package validation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

func supportAllGVK(gvk schema.GroupVersionKind) bool {
	return true
}

func denyAllGVK(gvk schema.GroupVersionKind) bool {
	return false
}

var (
	validMetadata = metav1.ObjectMeta{
		Name:      "test",
		Namespace: "default",
	}

	validWorkloadRef = rolloutv1alpha1.WorkloadRef{
		APIVersion: "apps/v1",
		Kind:       "StatefulSet",
		Match: rolloutv1alpha1.ResourceMatch{
			Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
				{
					Cluster: "cluster-a",
					Name:    "test",
				},
			},
		},
	}

	validRollout = &rolloutv1alpha1.Rollout{
		ObjectMeta: validMetadata,
		Spec: rolloutv1alpha1.RolloutSpec{
			TriggerPolicy: "Auto",
			StrategyRef:   "strategy-a",
			WorkloadRef:   validWorkloadRef,
		},
	}
)

func TestValidateRollout(t *testing.T) {
	tests := []struct {
		name           string
		obj            *rolloutv1alpha1.Rollout
		isSupportedGVK SupportedGVKFunc
		wantErr        bool
	}{
		{
			name:           "valid rollout",
			obj:            validRollout,
			isSupportedGVK: supportAllGVK,
			wantErr:        false,
		},
		{
			name: "invalid trigger policy",
			obj: func() *rolloutv1alpha1.Rollout {
				ro := validRollout.DeepCopy()
				ro.Spec.TriggerPolicy = "Invalid"
				return ro
			}(),
			isSupportedGVK: supportAllGVK,
			wantErr:        true,
		},
		{
			name: "invalid strategy ref",
			obj: func() *rolloutv1alpha1.Rollout {
				ro := validRollout.DeepCopy()
				ro.Spec.StrategyRef = ""
				return ro
			}(),
			isSupportedGVK: supportAllGVK,
			wantErr:        true,
		},
		{
			name:           "invalid GVK",
			obj:            validRollout,
			isSupportedGVK: denyAllGVK,
			wantErr:        true,
		},
		{
			name: "invalid workload resource matcher",
			obj: func() *rolloutv1alpha1.Rollout {
				ro := validRollout.DeepCopy()
				ro.Spec.WorkloadRef.Match = rolloutv1alpha1.ResourceMatch{}
				return ro
			}(),
			isSupportedGVK: supportAllGVK,
			wantErr:        true,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateRollout(tt.obj, tt.isSupportedGVK)
			if tt.wantErr != (got.ToAggregate() != nil) {
				t.Errorf("ValidateRollout() = %v, wantErr %v", got, tt.wantErr)
			}
		})
	}
}
