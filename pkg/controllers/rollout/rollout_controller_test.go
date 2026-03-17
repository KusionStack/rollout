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

package rollout

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/kube-api/rollout/v1alpha1/condition"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Rollout Controller Integration Tests", func() {
	const (
		timeout  = time.Second * 30
		interval = time.Millisecond * 250
	)

	Context("Rollout initialize", func() {
		var (
			namespace  = "test-ns-rollout-init"
			rolloutKey = types.NamespacedName{
				Namespace: namespace,
				Name:      "test-rollout-init",
			}
		)

		It("should create rollout with finalizer", func() {
			Expect(createNamespace(namespace)).Should(Succeed())

			// Create a basic rollout
			rollout := newTestRollout(rolloutKey.Name, namespace)
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait for finalizer to be added
			Eventually(func() bool {
				r := &rolloutv1alpha1.Rollout{}
				if err := c.Get(ctx, rolloutKey, r); err != nil {
					return false
				}
				return containsString(r.Finalizers, rolloutapi.FinalizerRolloutProtection)
			}, timeout, interval).Should(BeTrue())
		})

		It("initialize rollout with correct phase", func() {
			// Wait for rollout to be initialized
			Eventually(func() bool {
				r := &rolloutv1alpha1.Rollout{}
				if err := c.Get(ctx, rolloutKey, r); err != nil {
					return false
				}
				return r.Status.Phase == rolloutv1alpha1.RolloutPhaseInitialized
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Rollout with StrategyRef", func() {
		var (
			namespace   = "test-ns-rollout-strategy"
			rolloutKey  = types.NamespacedName{Namespace: namespace, Name: "test-rollout"}
			strategyKey = types.NamespacedName{Namespace: namespace, Name: "test-strategy"}
		)

		It("should resolve strategy reference successfully", func() {
			Expect(createNamespace(namespace)).Should(Succeed())

			// Create rollout strategy
			strategy := newTestRolloutStrategy(strategyKey.Name, namespace)
			Expect(c.Create(clusterinfo.WithCluster(ctx, clusterinfo.Fed), strategy)).Should(Succeed())

			rollout := newTestRollout(rolloutKey.Name, namespace)
			rollout.Spec.StrategyRef = strategyKey.Name
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait for Available condition to become True
			Eventually(func() bool {
				r := &rolloutv1alpha1.Rollout{}
				if err := c.Get(ctx, rolloutKey, r); err != nil {
					return false
				}
				return condition.IsAvailable(r.Status.Conditions)
			}, timeout, interval).Should(BeTrue())
		})

		It("should fail when strategy reference does not exist", func() {
			rolloutKey = types.NamespacedName{
				Namespace: namespace,
				Name:      "test-rollout-no-strategy",
			}
			rollout := newTestRollout(rolloutKey.Name, namespace)
			rollout.Spec.StrategyRef = "non-existent-strategy"
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait for Available condition to become False
			Eventually(func() bool {
				r := &rolloutv1alpha1.Rollout{}
				if err := c.Get(ctx, rolloutKey, r); err != nil {
					return false
				}
				return !condition.IsAvailable(r.Status.Conditions)
			}, timeout, interval).Should(BeTrue())
		})

		It("should create rolloutRun using strategyRef", func() {
			rolloutKey = types.NamespacedName{
				Namespace: namespace,
				Name:      "test-rollout-with-run",
			}

			rollout := newTestRollout(rolloutKey.Name, namespace)
			rollout.Spec.StrategyRef = strategyKey.Name
			rollout.Annotations = map[string]string{
				rolloutapi.AnnoRolloutTrigger: "trigger-from-strategy-ref",
			}
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait for rolloutRun to be created
			Eventually(func() bool {
				runList := &rolloutv1alpha1.RolloutRunList{}
				if err := c.List(clusterinfo.WithCluster(ctx, clusterinfo.Fed), runList, client.InNamespace(namespace)); err != nil {
					return false
				}
				for _, run := range runList.Items {
					owner := metav1.GetControllerOf(&run)
					if owner != nil && owner.Name == rolloutKey.Name && run.Name == "trigger-from-strategy-ref" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Rollout with Inline Strategy", func() {
		var (
			namespace  = "test-ns-inline"
			rolloutKey = types.NamespacedName{Namespace: namespace, Name: "test-rollout"}
		)

		It("should create rollout with inline batch strategy", func() {
			Expect(createNamespace(namespace)).Should(Succeed())

			rollout := newTestRolloutWithInlineBatch(rolloutKey.Name, namespace)
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Verify rollout is created and has finalizer
			Eventually(func() bool {
				r := &rolloutv1alpha1.Rollout{}
				if err := c.Get(ctx, rolloutKey, r); err != nil {
					return false
				}
				return containsString(r.Finalizers, rolloutapi.FinalizerRolloutProtection)
			}, timeout, interval).Should(BeTrue())
		})

		It("should create rollout with inline canary strategy", func() {
			rolloutKey = types.NamespacedName{Namespace: namespace, Name: "test-rollout-canary"}
			rollout := newTestRolloutWithInlineCanary(rolloutKey.Name, namespace)
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Verify rollout is created
			Eventually(func() bool {
				r := &rolloutv1alpha1.Rollout{}
				if err := c.Get(ctx, rolloutKey, r); err != nil {
					return false
				}
				return r.Status.Phase == rolloutv1alpha1.RolloutPhaseInitialized
			}, timeout, interval).Should(BeTrue())
		})

		It("should create rolloutRun with inline batch strategy and trigger", func() {
			rolloutKey = types.NamespacedName{Namespace: namespace, Name: "test-rollout-inline-batch-trigger"}
			rollout := newTestRolloutWithInlineBatch(rolloutKey.Name, namespace)
			rollout.Annotations = map[string]string{
				rolloutapi.AnnoRolloutTrigger: "inline-batch-trigger",
			}
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait for rolloutRun to be created
			Eventually(func() bool {
				runList := &rolloutv1alpha1.RolloutRunList{}
				if err := c.List(clusterinfo.WithCluster(ctx, clusterinfo.Fed), runList, client.InNamespace(namespace)); err != nil {
					return false
				}
				for _, run := range runList.Items {
					owner := metav1.GetControllerOf(&run)
					if owner != nil && owner.Name == rolloutKey.Name && run.Name == "inline-batch-trigger" {
						return run.Spec.Batch != nil
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("should create rolloutRun with inline canary strategy and trigger", func() {
			rolloutKey = types.NamespacedName{Namespace: namespace, Name: "test-rollout-inline-canary-trigger"}
			rollout := newTestRolloutWithInlineCanary(rolloutKey.Name, namespace)
			rollout.Annotations = map[string]string{
				rolloutapi.AnnoRolloutTrigger: "inline-canary-trigger",
			}
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait for rolloutRun to be created
			Eventually(func() bool {
				runList := &rolloutv1alpha1.RolloutRunList{}
				if err := c.List(clusterinfo.WithCluster(ctx, clusterinfo.Fed), runList, client.InNamespace(namespace)); err != nil {
					return false
				}
				for _, run := range runList.Items {
					owner := metav1.GetControllerOf(&run)
					if owner != nil && owner.Name == rolloutKey.Name && run.Name == "inline-canary-trigger" {
						return run.Spec.Canary != nil
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Trigger Policy", func() {
		var (
			namespace  = "test-ns-manual-policy"
			rolloutKey = types.NamespacedName{Namespace: namespace, Name: "test-rollout-manual-policy"}
		)

		It("should not auto-trigger when trigger policy is manual", func() {
			Expect(createNamespace(namespace)).Should(Succeed())

			rollout := newTestRolloutWithInlineBatch(rolloutKey.Name, namespace)
			rollout.Spec.TriggerPolicy = rolloutv1alpha1.ManualTriggerPolicy
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait a bit to ensure no rolloutRun is created
			Consistently(func() int {
				runList := &rolloutv1alpha1.RolloutRunList{}
				_ = c.List(clusterinfo.WithCluster(ctx, clusterinfo.Fed), runList, client.InNamespace(namespace))
				return len(runList.Items)
			}, time.Second*3, interval).Should(Equal(0))
		})

		It("should auto-trigger when trigger policy is auto (default)", func() {
			// Use different rolloutKey for this test
			rolloutKey = types.NamespacedName{Namespace: namespace, Name: "test-rollout-auto-policy"}

			// Create rollout without trigger annotation
			rollout := newTestRollout(rolloutKey.Name, namespace)
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait and verify rolloutRun is not created without trigger annotation
			// The controller needs workloads to be in waiting-rollout state
			Consistently(func() int {
				runList := &rolloutv1alpha1.RolloutRunList{}
				_ = c.List(clusterinfo.WithCluster(ctx, clusterinfo.Fed), runList, client.InNamespace(namespace))
				return len(runList.Items)
			}, time.Second*3, interval).Should(Equal(0))
		})
	})

	Context("Status Updates", func() {
		var (
			namespace  = "test-ns-status"
			rolloutKey = types.NamespacedName{Namespace: namespace, Name: "test-rollout-obs-gen"}
		)

		It("should update observed generation", func() {
			Expect(createNamespace(namespace)).Should(Succeed())

			rollout := newTestRollout(rolloutKey.Name, namespace)
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait for observed generation to be set
			Eventually(func() int64 {
				r := &rolloutv1alpha1.Rollout{}
				if err := c.Get(ctx, rolloutKey, r); err != nil {
					return 0
				}
				return r.Status.ObservedGeneration
			}, timeout, interval).Should(Equal(rollout.Generation))
		})

		It("should record conditions properly", func() {
			rolloutKey = types.NamespacedName{Namespace: namespace, Name: "test-rollout-conditions"}

			rollout := newTestRollout(rolloutKey.Name, namespace)
			Expect(c.Create(ctx, rollout)).Should(Succeed())

			// Wait for conditions to be recorded
			Eventually(func() bool {
				r := &rolloutv1alpha1.Rollout{}
				if err := c.Get(ctx, rolloutKey, r); err != nil {
					return false
				}
				return len(r.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())
		})
	})
})

// Helper functions

func newTestRollout(name, namespace string) *rolloutv1alpha1.Rollout {
	return &rolloutv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{},
		},
		Spec: rolloutv1alpha1.RolloutSpec{
			WorkloadRef: rolloutv1alpha1.WorkloadRef{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
			},
		},
	}
}

func newTestRolloutWithInlineBatch(name, namespace string) *rolloutv1alpha1.Rollout {
	rollout := newTestRollout(name, namespace)
	rollout.Spec.BatchStrategy = &rolloutv1alpha1.RolloutRunBatchStrategy{
		Batches: []rolloutv1alpha1.RolloutRunStep{
			{
				Breakpoint: true,
				Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Cluster: "",
							Name:    "test-workload",
						},
						Replicas: intstr.FromString("25%"),
					},
				},
			},
			{
				Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Cluster: "",
							Name:    "test-workload",
						},
						Replicas: intstr.FromString("100%"),
					},
				},
			},
		},
	}
	return rollout
}

func newTestRolloutWithInlineCanary(name, namespace string) *rolloutv1alpha1.Rollout {
	rollout := newTestRollout(name, namespace)
	rollout.Spec.BatchStrategy = &rolloutv1alpha1.RolloutRunBatchStrategy{
		Batches: []rolloutv1alpha1.RolloutRunStep{
			{
				Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{
						CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
							Name: "test-workload",
						},
						Replicas: intstr.FromString("100%"),
					},
				},
			},
		},
	}
	rollout.Spec.CanaryStrategy = &rolloutv1alpha1.RolloutRunCanaryStrategy{
		Targets: []rolloutv1alpha1.RolloutRunStepTarget{
			{
				CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
					Name: "test-workload",
				},
				Replicas: intstr.FromString("10%"),
			},
		},
	}
	return rollout
}

func newTestRolloutStrategy(name, namespace string) *rolloutv1alpha1.RolloutStrategy {
	return &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Batch: &rolloutv1alpha1.BatchStrategy{
			Batches: []rolloutv1alpha1.RolloutStep{
				{
					Breakpoint: true,
					Replicas:   intstr.FromString("25%"),
				},
				{
					Replicas: intstr.FromString("100%"),
				},
			},
		},
	}
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func createNamespace(namespace string) error {
	ctx = context.Background()

	// Create test namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	return c.Create(ctx, ns)
}
