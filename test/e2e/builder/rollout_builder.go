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
	"k8s.io/apimachinery/pkg/runtime/schema"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

// RolloutBuilder is a builder for Rollout
type RolloutBuilder struct {
	builder
	strategyName string
}

// NewRollout returns a Rollout builder
func NewRollout() *RolloutBuilder {
	return &RolloutBuilder{}
}

func (b *RolloutBuilder) Namespace(namespace string) *RolloutBuilder {
	b.namespace = namespace
	return b
}

// StrategyName sets the rollout strategy name
func (b *RolloutBuilder) StrategyName(name string) *RolloutBuilder {
	b.strategyName = name
	return b
}

// Build returns a Rollout
func (b *RolloutBuilder) Build(gvk schema.GroupVersionKind, labels map[string]string) *rolloutv1alpha1.Rollout {
	b.complete()

	return &rolloutv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.name,
			Namespace: b.namespace,
		},
		Spec: rolloutv1alpha1.RolloutSpec{
			TriggerPolicy: rolloutv1alpha1.ManualTriggerPolicy,
			WorkloadRef: rolloutv1alpha1.WorkloadRef{
				APIVersion: gvk.GroupVersion().String(),
				Kind:       gvk.Kind,
				Match: rolloutv1alpha1.ResourceMatch{
					Selector: &metav1.LabelSelector{
						MatchLabels: labels,
					},
				},
			},
			StrategyRef: b.strategyName,
		},
	}
}

// complete sets default values
func (b *RolloutBuilder) complete() {
	b.builder.complete()

	if b.strategyName == "" {
		b.strategyName = defaultStrategyName
	}
}
