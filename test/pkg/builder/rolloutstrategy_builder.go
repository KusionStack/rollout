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
	"k8s.io/apimachinery/pkg/util/intstr"

	rolloutv1alpha1 "github.com/KusionStack/rollout/api/v1alpha1"
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
		Spec: rolloutv1alpha1.RolloutStrategySpec{
			Batch: &rolloutv1alpha1.BatchStrategy{
				PauseMode: rolloutv1alpha1.PauseModeTypeFirstBatch,
				TolerationPolicy: &rolloutv1alpha1.TolerationPolicy{
					FailureThreshold: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
					WaitTimeSeconds:  2,
				},
				Analysis: &rolloutv1alpha1.AnalysisStrategy{
					Rules: []*rolloutv1alpha1.AnalysisRule{
						{
							CheckPoints:     []rolloutv1alpha1.CheckPointType{rolloutv1alpha1.CheckPointTypePreBatch, rolloutv1alpha1.CheckPointTypePostBatch},
							Provider:        rolloutv1alpha1.AnalysisProvider{Name: "provider-1", Address: "addr-1"},
							IntervalSeconds: 10,
							MaxFailureCount: 10,
						},
						{
							CheckPoints:     []rolloutv1alpha1.CheckPointType{rolloutv1alpha1.CheckPointTypePostBatch},
							Provider:        rolloutv1alpha1.AnalysisProvider{Name: "provider-2", Address: "addr-2"},
							IntervalSeconds: 10,
							MaxFailureCount: 10,
						},
					},
				},
				BatchTemplate: &rolloutv1alpha1.BatchTemplate{
					FixedBatchPolicy: &rolloutv1alpha1.FixedBatchPolicy{
						BatchCount:  5,
						Breakpoints: []int32{1, 3, 5},
						BetaBatch:   &rolloutv1alpha1.BetaBatch{},
					},
				},
			},
		},
	}
}
