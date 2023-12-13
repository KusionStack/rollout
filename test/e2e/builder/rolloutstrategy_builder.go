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

package builder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

const defaultStrategyName = DefaultName

// RolloutStrategyBuilder is a builder for RolloutStrategy
type RolloutStrategyBuilder struct {
	builder
}

// NewRolloutStrategy returns a RolloutStrategy builder
func NewRolloutStrategy() *RolloutStrategyBuilder {
	return &RolloutStrategyBuilder{}
}

// Build returns a RolloutStrategy
func (b *RolloutStrategyBuilder) Build() *rolloutv1alpha1.RolloutStrategy {
	b.complete()

	return &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.name,
			Namespace: b.namespace,
		},
		Batch: &rolloutv1alpha1.BatchStrategy{
			Toleration: &rolloutv1alpha1.TolerationStrategy{
				WorkloadFailureThreshold: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
				InitialDelaySeconds:      2,
			},
			Batches: []rolloutv1alpha1.RolloutStep{
				{
					Replicas: intstr.FromInt(1),
					Pause:    pointer.Bool(true),
				},
				{
					Replicas: intstr.FromString("50%"),
					Pause:    pointer.Bool(true),
				},
				{
					Replicas: intstr.FromString("100%"),
				},
			},
		},
	}
}
