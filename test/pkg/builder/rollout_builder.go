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
