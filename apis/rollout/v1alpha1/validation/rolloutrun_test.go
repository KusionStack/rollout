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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

func newValidRollotRun() *rolloutv1alpha1.RolloutRun {
	return &rolloutv1alpha1.RolloutRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Spec: rolloutv1alpha1.RolloutRunSpec{
			TargetType: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
			},
			Webhooks: []rolloutv1alpha1.RolloutWebhook{
				{
					Name:      "webhook-1",
					HookTypes: []rolloutv1alpha1.HookType{rolloutv1alpha1.PreBatchStepHook},
					ClientConfig: rolloutv1alpha1.WebhookClientConfig{
						URL: "http://example.com",
					},
				},
				{
					Name:      "webhook-2",
					HookTypes: []rolloutv1alpha1.HookType{rolloutv1alpha1.PreBatchStepHook},
					ClientConfig: rolloutv1alpha1.WebhookClientConfig{
						URL: "http://example.com",
					},
				},
			},
			TrafficTopologyRefs: []string{
				"traffic-topology-0",
				"traffic-topology-1",
			},
			Canary: &rolloutv1alpha1.RolloutRunCanaryStrategy{
				Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Name:    "test",
							Cluster: "cluster-1",
						},
						Replicas: intstr.FromInt(1),
					},
					{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Name:    "test",
							Cluster: "cluster-2",
						},
						Replicas: intstr.FromInt(1),
					},
				},
				TemplateMetadataPatch: &rolloutv1alpha1.MetadataPatch{
					Labels: map[string]string{
						"canary": "true",
					},
				},
			},

			Batch: &rolloutv1alpha1.RolloutRunBatchStrategy{
				Batches: []rolloutv1alpha1.RolloutRunStep{
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Name:    "test",
									Cluster: "cluster-1",
								},
								Replicas: intstr.FromInt(1),
							},
						},
					},
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Name:    "test",
									Cluster: "cluster-1",
								},
								Replicas: intstr.FromString("100%"),
							},
							{
								CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
									Name:    "test",
									Cluster: "cluster-2",
								},
								Replicas: intstr.FromString("100%"),
							},
						},
					},
				},
			},
		},
	}
}

func TestValidateRolloutRun(t *testing.T) {
	validRolloutRun := newValidRollotRun()
	tests := []struct {
		name    string
		obj     *rolloutv1alpha1.RolloutRun
		wantErr bool
		errLen  int
	}{
		{
			name:    "valid RolloutRun",
			obj:     validRolloutRun,
			wantErr: false,
		},
		{
			name: "duplicated webhook name, missing hookType, invalid url",
			obj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Webhooks = append(obj.Spec.Webhooks, rolloutv1alpha1.RolloutWebhook{
					Name: "webhook-1",
					ClientConfig: rolloutv1alpha1.WebhookClientConfig{
						URL: "invalid_url",
					},
				})
				return obj
			}(),
			wantErr: true,
			errLen:  3,
		},
		{
			name: "set canary with out batch",
			obj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Batch = nil
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name: "duplicate targets, and empty target name",
			obj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Canary.Targets = append(obj.Spec.Canary.Targets,
					rolloutv1alpha1.RolloutRunStepTarget{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Name:    "test",
							Cluster: "cluster-1",
						},
						Replicas: intstr.FromInt(1),
					},
					rolloutv1alpha1.RolloutRunStepTarget{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Name:    "",
							Cluster: "",
						},
						Replicas: intstr.FromInt(1),
					},
				)
				return obj
			}(),
			wantErr: true,
			errLen:  2,
		},
		{
			name: "empty targets",
			obj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Canary.Targets = nil
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name: "invalid replicas",
			obj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Canary.Targets[0].Replicas = intstr.FromString("invalid")
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name: "empty batches",
			obj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Batch.Batches = nil
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name: "valid traffic",
			obj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Canary.Traffic = validTraffic
				obj.Spec.Batch.Batches[0].Traffic = validTraffic
				return obj
			}(),
			wantErr: false,
			errLen:  0,
		},
		{
			name: "invalid traffic",
			obj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Canary.Traffic = invalidTraffic
				obj.Spec.Batch.Batches[0].Traffic = invalidTraffic
				return obj
			}(),
			wantErr: true,
			errLen:  2,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateRolloutRun(tt.obj)
			if tt.wantErr != (got.ToAggregate() != nil) {
				t.Errorf("ValidateRolloutRun() = %v, wantErr %v", got, tt.wantErr)
			}
			if tt.wantErr && len(got) != tt.errLen {
				t.Errorf("ValidateRolloutRun() = %v, want errors count %v", got, tt.errLen)
			}
		})
	}
}

func TestValidateRolloutRunUpdate(t *testing.T) {
	validRolloutRun := newValidRollotRun()
	tests := []struct {
		name    string
		newObj  *rolloutv1alpha1.RolloutRun
		oldObj  *rolloutv1alpha1.RolloutRun
		wantErr bool
		errLen  int
	}{
		{
			name:   "valid update",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Canary.Targets[0].Replicas = intstr.FromInt(2)
				return obj
			}(),
			wantErr: false,
		},
		{
			name:   "immutable targetType, webhooks, trafficTopologyRefs",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				// change target type
				obj.Spec.TargetType = rolloutv1alpha1.ObjectTypeRef{
					APIVersion: "apps.kusionstack.io",
					Kind:       "CollaSet",
				}
				// change webhooks
				obj.Spec.Webhooks[0].Name = "webhook-xx"
				// change traffic topology refs
				obj.Spec.TrafficTopologyRefs = nil
				return obj
			}(),
			wantErr: true,
			errLen:  3,
		},
		{
			name:   "delete canary",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Canary = nil
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name:   "mutate canary before running",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Canary.Targets[0].Replicas = intstr.FromInt(2)
				obj.Spec.Canary.Traffic = &rolloutv1alpha1.TrafficStrategy{
					HTTP: &rolloutv1alpha1.HTTPTrafficStrategy{
						Weight: ptr.To[int32](10),
					},
				}
				return obj
			}(),
			wantErr: false,
		},
		{
			name:   "mutate canary after running",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: rolloutv1alpha1.RolloutStepRunning,
				}
				obj.Spec.Canary.Targets[0].Replicas = intstr.FromInt(2)
				obj.Spec.Canary.TemplateMetadataPatch.Labels["canary"] = "false"
				return obj
			}(),
			wantErr: true,
			errLen:  2,
		},
		{
			name:   "delete batch",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Spec.Batch = nil
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name:   "mutate batch before running",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rolloutv1alpha1.RolloutStepPending,
					},
				}
				obj.Spec.Batch.Batches[0].Targets[0].Replicas = intstr.FromInt(2)
				obj.Spec.Batch.Batches[0].Traffic = &rolloutv1alpha1.TrafficStrategy{
					HTTP: &rolloutv1alpha1.HTTPTrafficStrategy{
						Weight: ptr.To[int32](10),
					},
				}
				return obj
			}(),
			wantErr: false,
		},
		{
			name:   "shorten batches when running",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1,
						CurrentBatchState: rolloutv1alpha1.RolloutStepPending,
					},
				}
				obj.Spec.Batch.Batches = obj.Spec.Batch.Batches[:1]
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name:   "mutate batch after running",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rolloutv1alpha1.RolloutStepRunning,
					},
				}
				obj.Spec.Batch.Batches[0].Breakpoint = true
				obj.Spec.Batch.Batches[0].Targets[0].Replicas = intstr.FromInt(2)
				obj.Spec.Batch.Batches[0].Traffic = &rolloutv1alpha1.TrafficStrategy{
					HTTP: &rolloutv1alpha1.HTTPTrafficStrategy{
						Weight: ptr.To[int32](10),
					},
				}
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name:   "mutate pending batch's breakpoint",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rolloutv1alpha1.RolloutStepPending,
					},
				}
				obj.Spec.Batch.Batches[0].Breakpoint = true
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name:   "mutate replicas in current batch after running",
			oldObj: validRolloutRun,
			newObj: func() *rolloutv1alpha1.RolloutRun {
				obj := validRolloutRun.DeepCopy()
				obj.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rolloutv1alpha1.RolloutStepPending,
					},
				}
				obj.Spec.Batch.Batches[0].Targets[0].Replicas = intstr.FromInt(10)
				return obj
			}(),
			wantErr: false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateRolloutRunUpdate(tt.newObj, tt.oldObj)
			if tt.wantErr != (len(got) != 0) || len(got) != tt.errLen {
				t.Errorf("ValidateRolloutRunUpdate() = %v, error count %v, wantErr %v, wantErrLen %v", got, len(got), tt.wantErr, tt.errLen)
			}
		})
	}
}
