// Copyright 2025 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package route

import (
	"encoding/json"
	"fmt"

	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetConditionExtension(routeObj client.Object) (*rolloutv1alpha1.RouteConditionExtension, error) {
	annoCond, ok := routeObj.GetAnnotations()[rolloutapi.AnnoRouteConditions]
	if !ok {
		// no route-conditions annotation measn ready
		return nil, nil
	}
	if len(annoCond) == 0 {
		return nil, fmt.Errorf("annotaions[%s] value is empty", rolloutapi.AnnoRouteConditions)
	}
	routeCond := &rolloutv1alpha1.RouteConditionExtension{}
	err := json.Unmarshal([]byte(annoCond), routeCond)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal annotations[%s] value: %w", rolloutapi.AnnoRouteConditions, err)
	}
	return routeCond, nil
}
