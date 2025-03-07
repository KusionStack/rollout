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

package features

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"
)

const (
	// owner: @zoumo
	//
	// Allow user set one time batch stratey in rollout annotation
	OneTimeStrategy featuregate.Feature = "OneTimeStrategy"

	// owner: @youngLiuHY
	// Allow rollout class predicate when reconcile
	RolloutClassPredicate featuregate.Feature = "RolloutClassPredicate"
)

func init() {
	runtime.Must(DefaultMutableFeatureGate.Add(defaultKubernetesFeatureGates))
}

// defaultKubernetesFeatureGates consists of all known Kubernetes-specific feature keys.
// To add a new feature, define a key for it above and add it here. The features will be
// available throughout Kubernetes binaries.
var defaultKubernetesFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	OneTimeStrategy:       {Default: false, PreRelease: featuregate.Alpha},
	RolloutClassPredicate: {Default: true, PreRelease: featuregate.Alpha},
}
