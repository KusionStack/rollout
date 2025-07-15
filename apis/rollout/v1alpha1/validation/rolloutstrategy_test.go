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
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

var validTraffic = &rolloutv1alpha1.TrafficStrategy{
	HTTP: &rolloutv1alpha1.HTTPTrafficStrategy{
		Weight: ptr.To[int32](10),
		HTTPRouteRule: rolloutv1alpha1.HTTPRouteRule{
			Filters: []gatewayapiv1.HTTPRouteFilter{
				{
					RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
						Set: []gatewayapiv1.HTTPHeader{
							{
								Name:  "foo",
								Value: "bar",
							},
						},
					},
				},
			},
		},
	},
}

var invalidTraffic = &rolloutv1alpha1.TrafficStrategy{
	HTTP: &rolloutv1alpha1.HTTPTrafficStrategy{
		Weight: ptr.To[int32](10),
		HTTPRouteRule: rolloutv1alpha1.HTTPRouteRule{
			Matches: []rolloutv1alpha1.HTTPRouteMatch{
				{
					Headers: []gatewayapiv1.HTTPHeaderMatch{
						{
							Name:  "foo",
							Value: "bar",
						},
					},
				},
			},
			Filters: []gatewayapiv1.HTTPRouteFilter{
				{
					RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
						Set: []gatewayapiv1.HTTPHeader{
							{
								Name:  "foo",
								Value: "bar",
							},
						},
					},
				},
			},
		},
	},
}

func TestValidateRolloutStrategy(t *testing.T) {
	validStratgy := &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Canary: &rolloutv1alpha1.CanaryStrategy{
			Replicas: intstr.FromInt(1),
		},
		Batch: &rolloutv1alpha1.BatchStrategy{
			Batches: []rolloutv1alpha1.RolloutStep{
				{
					Replicas: intstr.FromInt(1),
				},
				{
					Replicas: intstr.FromString("100%"),
				},
			},
		},
	}

	tests := []struct {
		name    string
		obj     *rolloutv1alpha1.RolloutStrategy
		wantErr bool
		errLen  int
	}{
		{
			name:    "valid strategy",
			obj:     validStratgy,
			wantErr: false,
		},
		{
			name: "set canary with out batch",
			obj: func() *rolloutv1alpha1.RolloutStrategy {
				obj := validStratgy.DeepCopy()
				obj.Batch = nil
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name: "empty batches",
			obj: func() *rolloutv1alpha1.RolloutStrategy {
				obj := validStratgy.DeepCopy()
				obj.Batch.Batches = nil
				return obj
			}(),
			wantErr: true,
			errLen:  1,
		},
		{
			name: "valid traffic",
			obj: func() *rolloutv1alpha1.RolloutStrategy {
				obj := validStratgy.DeepCopy()
				obj.Canary.Traffic = validTraffic
				obj.Batch.Batches[0].Traffic = validTraffic
				return obj
			}(),
			wantErr: false,
			errLen:  0,
		},

		{
			name: "invalid traffic",
			obj: func() *rolloutv1alpha1.RolloutStrategy {
				obj := validStratgy.DeepCopy()
				invalidTraffic := invalidTraffic
				obj.Canary.Traffic = invalidTraffic
				obj.Batch.Batches[0].Traffic = invalidTraffic
				return obj
			}(),
			wantErr: true,
			errLen:  2,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateRolloutStrategy(tt.obj)
			if tt.wantErr != (got.ToAggregate() != nil) {
				t.Errorf("ValidateRolloutStrategy() = %v, wantErr %v", got, tt.wantErr)
			}
			if tt.wantErr && len(got) != tt.errLen {
				t.Errorf("ValidateRolloutStrategy() = %v, want errors count %v", got, tt.errLen)
			}
		})
	}
}
