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

package e2e

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload/statefulset"
	"kusionstack.io/rollout/test/e2e/builder"
)

func mergeEnvVar(original []corev1.EnvVar, add corev1.EnvVar) []corev1.EnvVar {
	newEnvs := make([]corev1.EnvVar, 0)
	for _, env := range original {
		if add.Name == env.Name {
			continue
		}
		newEnvs = append(newEnvs, env)
	}
	newEnvs = append(newEnvs, add)
	return newEnvs
}

var _ = Describe("StatefulSet", func() {
	ctx := context.Background()

	var sts = &v1.StatefulSet{}
	var rollout = &rolloutv1alpha1.Rollout{}
	var strategy *rolloutv1alpha1.RolloutStrategy

	BeforeEach(func() {
		// Prepare
		sts = builder.NewStatefulSet().Build()
		Expect(k8sClient.Create(ctx, sts)).Should(Succeed())

		Eventually(func() bool {
			if err := k8sClient.Get(ctx, GetNamespacedName(sts.Name, sts.Namespace), sts); err != nil {
				return false
			}
			return true
		}, 5, 1).Should(BeTrue())

		strategy = builder.NewRolloutStrategy().Build()
		Expect(k8sClient.Create(ctx, strategy)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, GetNamespacedName(strategy.Name, strategy.Namespace), strategy); err != nil {
				return false
			}
			return true
		}, 5, 1).Should(BeTrue())

		rollout = builder.NewRollout().StrategyName(strategy.Name).Build(statefulset.GVK, map[string]string{
			"app.kubernetes.io/name": sts.Spec.Template.Labels["app.kubernetes.io/name"],
		})
		Expect(k8sClient.Create(ctx, rollout)).Should(Succeed())
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Name, rollout.Namespace), rollout); err != nil {
				return false
			}
			if rollout.Status.ObservedGeneration != rollout.Generation {
				return false
			}
			return true
		}, 5, 1).Should(BeTrue())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, sts)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, rollout)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, strategy)).Should(Succeed())
		Eventually(func() bool {
			clean := true
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sts), sts)
			if !apierrors.IsNotFound(err) {
				clean = false
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(rollout), rollout)
			if !apierrors.IsNotFound(err) {
				clean = false
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(strategy), strategy)
			if !apierrors.IsNotFound(err) {
				clean = false
			}
			return clean
		}, 5, 1).Should(BeTrue())
	})

	It("Update StatefulSet template to trigger rollout", func() {
		// Trigger Update
		_, err := utils.UpdateOnConflict(ctx, k8sClient, k8sClient, sts, func() error {
			mergeEnv := mergeEnvVar(
				sts.Spec.Template.Spec.Containers[0].Env,
				corev1.EnvVar{Name: "Foo", Value: "Bar"},
			)
			// pause update
			sts.Spec.UpdateStrategy = v1.StatefulSetUpdateStrategy{
				Type:          v1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &v1.RollingUpdateStatefulSetStrategy{Partition: pointer.Int32(*sts.Spec.Replicas)},
			}
			sts.Spec.Template.Spec.Containers[0].Env = mergeEnv
			return nil
		})

		Expect(err).NotTo(HaveOccurred())

		// wait for rollout & rolloutRun
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rollout), rollout); err != nil {
				return false
			}
			if len(rollout.Status.RolloutID) == 0 {
				return false
			}
			run := &rolloutv1alpha1.RolloutRun{}
			if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Status.RolloutID, rollout.Namespace), run); err != nil {
				return false
			}
			return true
		}, 10, 1).Should(BeTrue())

		// Beta && Step 1
		By("start Beta")
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, GetNamespacedName(sts.Name, sts.Namespace), sts); err != nil {
				return false
			}
			if sts.Status.UpdatedReplicas == 1 {
				return true
			}
			return false
		}, 60, 1).Should(BeTrue())

		// Step 1
		By("start Step 1")
		// resume to continue
		_, err = utils.UpdateOnConflict(ctx, k8sClient, k8sClient, rollout, func() error {
			if rollout.Annotations == nil {
				rollout.Annotations = make(map[string]string)
			}
			rollout.Annotations[rolloutapi.AnnoManualCommandKey] = rolloutapi.AnnoManualCommandResume
			return nil
		})

		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rollout), rollout); err != nil {
				return false
			}

			// wait for batch 1 finished and paused at batch 2
			if rollout.Status.BatchStatus.CurrentBatchIndex != 2 || rollout.Status.BatchStatus.CurrentBatchState != rolloutv1alpha1.RolloutStepStatePaused {
				return false
			}

			if err := k8sClient.Get(ctx, GetNamespacedName(sts.Name, sts.Namespace), sts); err != nil {
				return false
			}
			if sts.Status.UpdatedReplicas == 5 {
				return true
			}
			return false
		}, 120, 1).Should(BeTrue())

		// Step 2
		By("start step 2")
		_, err = utils.UpdateOnConflict(ctx, k8sClient, k8sClient, rollout, func() error {
			if rollout.Annotations == nil {
				rollout.Annotations = make(map[string]string)
			}
			rollout.Annotations[rolloutapi.AnnoManualCommandKey] = rolloutapi.AnnoManualCommandResume
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			if err := k8sClient.Get(ctx, GetNamespacedName(sts.Name, sts.Namespace), sts); err != nil {
				return false
			}
			if sts.Status.CurrentRevision == sts.Status.UpdateRevision &&
				sts.Status.UpdatedReplicas == sts.Status.Replicas {
				return true
			}
			return false
		}, 60, 1).Should(BeTrue())
	})
})
