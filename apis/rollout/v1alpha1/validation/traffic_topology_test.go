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

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

var validTrafficTopology = &rolloutv1alpha1.TrafficTopology{
	ObjectMeta: validMetadata,
	Spec: rolloutv1alpha1.TrafficTopologySpec{
		WorkloadRef: validWorkloadRef,
		TrafficType: rolloutv1alpha1.InClusterTrafficType,
		Backend: rolloutv1alpha1.BackendRef{
			Name: "service",
		},
		Routes: []rolloutv1alpha1.RouteRef{
			{
				Name: "httpRoute",
			},
		},
	},
}

func TestValidateTrafficTopology(t *testing.T) {
	tests := []struct {
		name                string
		obj                 *rolloutv1alpha1.TrafficTopology
		isSupportedWorkload SupportedGVKFunc
		isSupportedRoute    SupportedGVKFunc
		isSupportedBackend  SupportedGVKFunc
		wantErr             bool
	}{
		{
			name:                "valid traffic topology",
			obj:                 validTrafficTopology,
			isSupportedWorkload: supportAllGVK,
			isSupportedRoute:    supportAllGVK,
			isSupportedBackend:  supportAllGVK,
			wantErr:             false,
		},
		{
			name:                "unsupported workload",
			obj:                 validTrafficTopology,
			isSupportedWorkload: denyAllGVK,
			isSupportedRoute:    supportAllGVK,
			isSupportedBackend:  supportAllGVK,
			wantErr:             true,
		},
		{
			name:                "unsupported route",
			obj:                 validTrafficTopology,
			isSupportedWorkload: supportAllGVK,
			isSupportedRoute:    denyAllGVK,
			isSupportedBackend:  supportAllGVK,
			wantErr:             true,
		},
		{
			name:                "unsupported backend",
			obj:                 validTrafficTopology,
			isSupportedWorkload: supportAllGVK,
			isSupportedRoute:    supportAllGVK,
			isSupportedBackend:  denyAllGVK,
			wantErr:             true,
		},
		{
			name: "duplicate route",
			obj: func() *rolloutv1alpha1.TrafficTopology {
				ttopo := validTrafficTopology.DeepCopy()
				ttopo.Spec.Routes = append(ttopo.Spec.Routes, ttopo.Spec.Routes[0])
				return ttopo
			}(),
			isSupportedWorkload: supportAllGVK,
			isSupportedRoute:    supportAllGVK,
			isSupportedBackend:  denyAllGVK,
			wantErr:             true,
		},
		{
			name: "empty backend name",
			obj: func() *rolloutv1alpha1.TrafficTopology {
				ttopo := validTrafficTopology.DeepCopy()
				ttopo.Spec.Backend.Name = ""
				return ttopo
			}(),
			isSupportedWorkload: supportAllGVK,
			isSupportedRoute:    supportAllGVK,
			isSupportedBackend:  supportAllGVK,
			wantErr:             true,
		},
		{
			name: "empty route name",
			obj: func() *rolloutv1alpha1.TrafficTopology {
				ttopo := validTrafficTopology.DeepCopy()
				ttopo.Spec.Routes[0].Name = ""
				return ttopo
			}(),
			isSupportedWorkload: supportAllGVK,
			isSupportedRoute:    supportAllGVK,
			isSupportedBackend:  supportAllGVK,
			wantErr:             true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateTrafficTopology(tt.obj, tt.isSupportedWorkload, tt.isSupportedRoute, tt.isSupportedBackend)
			if tt.wantErr != (got.ToAggregate() != nil) {
				t.Errorf("ValidateTrafficTopology() error = %v, wantErr %v", got, tt.wantErr)
			}
		})
	}
}
