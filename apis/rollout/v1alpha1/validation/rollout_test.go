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
	"k8s.io/apimachinery/pkg/util/intstr"
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

// Tests for inline batch strategy validation
func TestValidateRollout_InlineBatchStrategy(t *testing.T) {
	tests := []struct {
		name           string
		obj            *rolloutv1alpha1.Rollout
		isSupportedGVK SupportedGVKFunc
		wantErr        bool
		errMsg         string
	}{
		{
			name: "valid inline batch strategy",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: validMetadata,
				Spec: rolloutv1alpha1.RolloutSpec{
					TriggerPolicy: "Auto",
					WorkloadRef:   validWorkloadRef,
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-a",
											Name:    "test",
										},
										Replicas: intstr.FromString("20%"),
									},
								},
							},
						},
					},
				},
			},
			isSupportedGVK: supportAllGVK,
			wantErr:        false,
		},
		{
			name: "batch strategy with multiple batches",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: validMetadata,
				Spec: rolloutv1alpha1.RolloutSpec{
					TriggerPolicy: "Auto",
					WorkloadRef:   validWorkloadRef,
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Breakpoint: true,
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-a",
											Name:    "test",
										},
										Replicas: intstr.FromString("20%"),
									},
								},
							},
							{
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-a",
											Name:    "test",
										},
										Replicas: intstr.FromString("100%"),
									},
								},
							},
						},
					},
				},
			},
			isSupportedGVK: supportAllGVK,
			wantErr:        false,
		},
		{
			name: "batch strategy with multiple targets in one batch",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: validMetadata,
				Spec: rolloutv1alpha1.RolloutSpec{
					TriggerPolicy: "Auto",
					WorkloadRef:   validWorkloadRef,
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-a",
											Name:    "test-1",
										},
										Replicas: intstr.FromString("20%"),
									},
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-b",
											Name:    "test-2",
										},
										Replicas: intstr.FromString("30%"),
									},
								},
							},
						},
					},
				},
			},
			isSupportedGVK: supportAllGVK,
			wantErr:        false,
		},
		{
			name: "empty batch strategy - no batches",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: validMetadata,
				Spec: rolloutv1alpha1.RolloutSpec{
					TriggerPolicy: "Auto",
					WorkloadRef:   validWorkloadRef,
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{},
					},
				},
			},
			isSupportedGVK: supportAllGVK,
			wantErr:        true,
			errMsg:         "must specify at least one batch",
		},
		{
			name: "batch with empty targets",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: validMetadata,
				Spec: rolloutv1alpha1.RolloutSpec{
					TriggerPolicy: "Auto",
					WorkloadRef:   validWorkloadRef,
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{},
							},
						},
					},
				},
			},
			isSupportedGVK: supportAllGVK,
			wantErr:        true,
			errMsg:         "must specify at least one target",
		},
		{
			name: "target missing name",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: validMetadata,
				Spec: rolloutv1alpha1.RolloutSpec{
					TriggerPolicy: "Auto",
					WorkloadRef:   validWorkloadRef,
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-a",
										},
										Replicas: intstr.FromString("20%"),
									},
								},
							},
						},
					},
				},
			},
			isSupportedGVK: supportAllGVK,
			wantErr:        true,
			errMsg:         "name is required",
		},
		{
			name: "target with invalid replicas",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: validMetadata,
				Spec: rolloutv1alpha1.RolloutSpec{
					TriggerPolicy: "Auto",
					WorkloadRef:   validWorkloadRef,
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-a",
											Name:    "test",
										},
										Replicas: intstr.FromString("-1"),
									},
								},
							},
						},
					},
				},
			},
			isSupportedGVK: supportAllGVK,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateRollout(tt.obj, tt.isSupportedGVK)
			hasErr := got.ToAggregate() != nil
			if tt.wantErr != hasErr {
				t.Errorf("ValidateRollout() error = %v, wantErr %v", got.ToAggregate(), tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				errStr := got.ToAggregate().Error()
				if !containsString(errStr, tt.errMsg) {
					t.Errorf("ValidateRollout() error message = %q, should contain %q", errStr, tt.errMsg)
				}
			}
		})
	}
}

// Tests for StrategyRef and inline strategy mutual exclusion
func TestValidateRollout_StrategyMutualExclusion(t *testing.T) {
	tests := []struct {
		name           string
		obj            *rolloutv1alpha1.Rollout
		isSupportedGVK SupportedGVKFunc
		wantErr        bool
		errMsg         string
	}{
		{
			name: "StrategyRef is mutually exclusive with BatchStrategy",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: validMetadata,
				Spec: rolloutv1alpha1.RolloutSpec{
					TriggerPolicy: "Auto",
					StrategyRef:   "strategy-a",
					WorkloadRef:   validWorkloadRef,
					BatchStrategy: &rolloutv1alpha1.RolloutRunBatchStrategy{
						Batches: []rolloutv1alpha1.RolloutRunStep{
							{
								Targets: []rolloutv1alpha1.RolloutRunStepTarget{
									{
										CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
											Cluster: "cluster-a",
											Name:    "test",
										},
										Replicas: intstr.FromString("20%"),
									},
								},
							},
						},
					},
				},
			},
			isSupportedGVK: supportAllGVK,
			wantErr:        true,
			errMsg:         "strategyRef is mutually exclusive with batchStrategy",
		},
		{
			name: "neither StrategyRef nor BatchStrategy specified",
			obj: &rolloutv1alpha1.Rollout{
				ObjectMeta: validMetadata,
				Spec: rolloutv1alpha1.RolloutSpec{
					TriggerPolicy: "Auto",
					WorkloadRef:   validWorkloadRef,
				},
			},
			isSupportedGVK: supportAllGVK,
			wantErr:        true,
			errMsg:         "must specify either strategyRef, or batchStrategy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateRollout(tt.obj, tt.isSupportedGVK)
			hasErr := got.ToAggregate() != nil
			if tt.wantErr != hasErr {
				t.Errorf("ValidateRollout() error = %v, wantErr %v", got.ToAggregate(), tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				errStr := got.ToAggregate().Error()
				if !containsString(errStr, tt.errMsg) {
					t.Errorf("ValidateRollout() error message = %q, should contain %q", errStr, tt.errMsg)
				}
			}
		})
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringSub(s, substr))
}

func containsStringSub(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
