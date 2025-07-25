/**
 * Copyright 2025 The KusionStack Authors
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

var validBackendRouting = &rolloutv1alpha1.BackendRouting{
	ObjectMeta: validMetadata,
	Spec: rolloutv1alpha1.BackendRoutingSpec{
		TrafficType: rolloutv1alpha1.InClusterTrafficType,
		Backend: rolloutv1alpha1.CrossClusterObjectReference{
			ObjectTypeRef: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: "v1",
				Kind:       "Service",
			},
			CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
				Name: "service",
			},
		},
		Routes: []rolloutv1alpha1.CrossClusterObjectReference{
			{
				ObjectTypeRef: rolloutv1alpha1.ObjectTypeRef{
					APIVersion: "gateway.networking.k8s.io/v1",
					Kind:       "HTTPRoute",
				},
				CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
					Name: "httpRoute",
				},
			},
		},
		ForkedBackends: &rolloutv1alpha1.ForkedBackends{
			Stable: rolloutv1alpha1.ForkedBackend{
				Name: "stable",
			},
			Canary: rolloutv1alpha1.ForkedBackend{
				Name: "canary",
			},
		},
		Forwarding: &rolloutv1alpha1.BackendForwarding{
			HTTP: &rolloutv1alpha1.HTTPForwarding{
				Origin: &rolloutv1alpha1.OriginHTTPForwarding{
					BackendName: "origin",
				},
				Canary: &rolloutv1alpha1.CanaryHTTPForwarding{
					BackendName: "canary",
				},
			},
		},
	},
}

func TestValidateBackendRouting(t *testing.T) {
	tests := []struct {
		name               string
		obj                *rolloutv1alpha1.BackendRouting
		isSupportedRoute   SupportedGVKFunc
		isSupportedBackend SupportedGVKFunc
		wantErr            bool
		errNum             int
	}{
		{
			name:               "valid backend routing",
			obj:                validBackendRouting,
			isSupportedRoute:   supportAllGVK,
			isSupportedBackend: supportAllGVK,
			wantErr:            false,
		},
		{
			name:               "unsupported backend gvk",
			obj:                validBackendRouting,
			isSupportedRoute:   supportAllGVK,
			isSupportedBackend: denyAllGVK,
			wantErr:            true,
		},
		{
			name:               "unsupported route gvk",
			obj:                validBackendRouting,
			isSupportedRoute:   denyAllGVK,
			isSupportedBackend: supportAllGVK,
			wantErr:            true,
		},
		{
			name: "empty backend apiVersion, kind, name",
			obj: func() *rolloutv1alpha1.BackendRouting {
				obj := validBackendRouting.DeepCopy()
				obj.Spec.Backend.APIVersion = ""
				obj.Spec.Backend.Kind = ""
				obj.Spec.Backend.Name = ""
				return obj
			}(),
			isSupportedRoute:   supportAllGVK,
			isSupportedBackend: supportAllGVK,
			wantErr:            true,
			errNum:             3,
		},

		{
			name: "empty route apiVersion, kind, name",
			obj: func() *rolloutv1alpha1.BackendRouting {
				obj := validBackendRouting.DeepCopy()
				obj.Spec.Routes[0].APIVersion = ""
				obj.Spec.Routes[0].Kind = ""
				obj.Spec.Routes[0].Name = ""
				return obj
			}(),
			isSupportedRoute:   supportAllGVK,
			isSupportedBackend: supportAllGVK,
			wantErr:            true,
			errNum:             3,
		},
		{
			name: "duplicate route",
			obj: func() *rolloutv1alpha1.BackendRouting {
				obj := validBackendRouting.DeepCopy()
				obj.Spec.Routes = append(obj.Spec.Routes, obj.Spec.Routes[0])
				return obj
			}(),
			isSupportedRoute:   supportAllGVK,
			isSupportedBackend: supportAllGVK,
			wantErr:            true,
		},
		{
			name: "empty forked canary and stable name",
			obj: func() *rolloutv1alpha1.BackendRouting {
				obj := validBackendRouting.DeepCopy()
				obj.Spec.ForkedBackends = &rolloutv1alpha1.ForkedBackends{}
				obj.Spec.Forwarding = nil
				return obj
			}(),
			isSupportedRoute:   supportAllGVK,
			isSupportedBackend: supportAllGVK,
			wantErr:            true,
			errNum:             2,
		},
		{
			name: "ForkedBackends must not be nil when Forwarding is set",
			obj: func() *rolloutv1alpha1.BackendRouting {
				obj := validBackendRouting.DeepCopy()
				obj.Spec.ForkedBackends = nil
				return obj
			}(),
			isSupportedRoute:   supportAllGVK,
			isSupportedBackend: supportAllGVK,
			wantErr:            true,
		},
		{
			name: "forwording.http.origin must not be nil when stable or canary is set",
			obj: func() *rolloutv1alpha1.BackendRouting {
				obj := validBackendRouting.DeepCopy()
				obj.Spec.Forwarding.HTTP.Origin = nil
				return obj
			}(),
			isSupportedRoute:   supportAllGVK,
			isSupportedBackend: supportAllGVK,
			wantErr:            true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateBackendRouting(tt.obj, tt.isSupportedRoute, tt.isSupportedBackend)
			if tt.wantErr != (got.ToAggregate() != nil) {
				t.Errorf("ValidateBackendRouting() error = %v, wantErr %v", got, tt.wantErr)
			}
			if tt.wantErr && tt.errNum > 0 && len(got) != tt.errNum {
				t.Errorf("ValidateBackendRouting() = %v, want err num %v", got, tt.errNum)
			}
		})
	}
}
