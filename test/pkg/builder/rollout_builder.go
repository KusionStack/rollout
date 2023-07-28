/*
 * Copyright 2023 The KusionStack Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package builder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/KusionStack/rollout/api/v1alpha1"
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

// StrategyName sets the rollout strategy name
func (b *RolloutBuilder) StrategyName(name string) *RolloutBuilder {
	b.strategyName = name
	return b
}

// Build returns a Rollout
func (b *RolloutBuilder) Build() *v1alpha1.Rollout {
	b.complete()

	return &v1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.name,
			Namespace: b.namespace,
		},
		Spec: v1alpha1.RolloutSpec{
			App: v1alpha1.Application{
				WorkloadRef: v1alpha1.WorkloadRef{
					TypeMeta: metav1.TypeMeta{
						// TODO: add workload type
						APIVersion: "",
						Kind:       "CollaSet",
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/name": b.appName,
						},
					},
				},
			},
			StrategyRef: b.strategyName,
			TriggerCondition: v1alpha1.TriggerCondition{
				AutoMode: &v1alpha1.AutoTriggerCondition{},
			},
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
