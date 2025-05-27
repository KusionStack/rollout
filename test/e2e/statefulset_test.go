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
	"encoding/json"
	"fmt"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/webhook/probe/http"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload/statefulset"
	"kusionstack.io/rollout/test/e2e/builder"
)

var _ = Describe("StatefulSet", func() {
	ctx := context.Background()

	var ts *httptest.Server
	sts := &appsv1.StatefulSet{}
	rollout := &rolloutv1alpha1.Rollout{}
	rolloutRun := &rolloutv1alpha1.RolloutRun{}
	var strategy *rolloutv1alpha1.RolloutStrategy

	BeforeEach(func() {
		// prepare http server
		ts = http.NewTestHTTPServer()

		// Prepare Sts
		sts = builder.NewStatefulSet().Namespace(e2eNamespace).Build()
		{
			By(fmt.Sprintf("PrepareSts: create sts %s/%s", sts.Namespace, sts.Name))
			Expect(k8sClient.Create(ctx, sts)).Should(Succeed())

			Eventually(func() bool {
				tmpSts := &appsv1.StatefulSet{}
				if err := k8sClient.Get(ctx, GetNamespacedName(sts.Name, sts.Namespace), tmpSts); err != nil {
					By(fmt.Sprintf("PrepareSts: Get sts %s/%s err %v", sts.Namespace, sts.Name, err))
					return false
				}

				if tmpSts.Status.ObservedGeneration != tmpSts.Generation {
					By(fmt.Sprintf("PrepareSts: checkGeneration %d/%d", tmpSts.Status.ObservedGeneration, tmpSts.Generation))
					return false
				}

				if val, err := json.Marshal(tmpSts.Status); err == nil {
					fmt.Printf("tempSts: %s \n", string(val))
				}

				if tmpSts.Status.ReadyReplicas != *sts.Spec.Replicas {
					By(fmt.Sprintf("PrepareSts: Replicas %d/%d", tmpSts.Status.ReadyReplicas, *sts.Spec.Replicas))
					return false
				}

				mergeEnv := mergeEnvVar(
					sts.Spec.Template.Spec.Containers[0].Env,
					corev1.EnvVar{Name: "Foo", Value: "Bar"},
				)
				tmpSts.Spec.Template.Spec.Containers[0].Env = mergeEnv
				tmpSts.Spec.UpdateStrategy.RollingUpdate.Partition = sts.Spec.Replicas
				if err := k8sClient.Update(ctx, tmpSts); err != nil {
					By("Update sts error")
					return false
				}

				return true
			}, "120s", "1s").Should(BeTrue())

			Eventually(func() bool {
				tmpSts := &appsv1.StatefulSet{}
				if err := k8sClient.Get(ctx, GetNamespacedName(sts.Name, sts.Namespace), tmpSts); err != nil {
					return false
				}

				if tmpSts.Status.UpdatedReplicas != 0 {
					return false
				}

				if tmpSts.Status.ObservedGeneration != tmpSts.Generation {
					return false
				}

				sts = tmpSts

				return true
			}, "60s", "1s").Should(BeTrue())
		}

		// Prepare Rollout && Strategy
		{
			strategy = builder.NewRolloutStrategy().Namespace(e2eNamespace).Build(ts)
			Expect(k8sClient.Create(ctx, strategy)).Should(Succeed())
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, GetNamespacedName(strategy.Name, strategy.Namespace), strategy); err != nil {
					return false
				}
				return true
			}, "60s", "1s").Should(BeTrue())

			rollout = builder.NewRollout().Namespace(e2eNamespace).StrategyName(strategy.Name).Build(statefulset.GVK, map[string]string{
				"app.kubernetes.io/name":     sts.Labels["app.kubernetes.io/name"],
				"app.kubernetes.io/instance": sts.Labels["app.kubernetes.io/instance"],
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
			}, "60s", "1s").Should(BeTrue())
		}
	})

	AfterEach(func() {
		defer ts.Close()

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
		}, "600s", "5s").Should(BeTrue())
	})

	It("Happy Path", func() {
		// trigger update
		{
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rollout), rollout); err != nil {
					return false
				}

				if len(rollout.Annotations) == 0 {
					rollout.Annotations = make(map[string]string)
				}
				rollout.Annotations["rollout.kusionstack.io/trigger"] = ""
				if err := k8sClient.Update(ctx, rollout); err != nil {
					return false
				}

				By("HappyPath: trigger rollout")

				return true
			}, "60s", "1s").Should(BeTrue())

			Eventually(func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rollout), rollout); err != nil {
					return false
				}

				if len(rollout.Status.RolloutID) == 0 {
					return false
				}
				if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Status.RolloutID, rollout.Namespace), rolloutRun); err != nil {
					return false
				}

				currentPhase := rolloutRun.Status.Phase
				currentBatchIndex := rolloutRun.Status.BatchStatus.CurrentBatchIndex
				currentBatchState := rolloutRun.Status.BatchStatus.CurrentBatchState

				if rolloutRun.Status.BatchStatus != nil &&
					currentPhase == rolloutv1alpha1.RolloutRunPhasePaused &&
					currentBatchIndex == 0 &&
					currentBatchState == rolloutv1alpha1.RolloutStepPreBatchStepHook {
					By("HappyPath: ensure trigger rolloutRun")
					return true
				}

				return false
			}, "60s", "1s").Should(BeTrue())
		}

		// first batch
		{
			By("HappyPath: start first batch")

			_, err := utils.UpdateOnConflict(ctx, k8sClient, k8sClient, rolloutRun, func() error {
				if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Status.RolloutID, rollout.Namespace), rolloutRun); err != nil {
					return err
				}

				if rolloutRun.Annotations == nil {
					rolloutRun.Annotations = make(map[string]string)
				}
				rolloutRun.Annotations[rolloutapi.AnnoManualCommandKey] = rolloutapi.AnnoManualCommandContinue
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Status.RolloutID, rollout.Namespace), rolloutRun); err != nil {
					return false
				}

				currentPhase := rolloutRun.Status.Phase
				currentBatchIndex := rolloutRun.Status.BatchStatus.CurrentBatchIndex
				currentBatchState := rolloutRun.Status.BatchStatus.CurrentBatchState

				if !(currentPhase == rolloutv1alpha1.RolloutRunPhasePaused &&
					currentBatchIndex == 1 &&
					currentBatchState == rolloutv1alpha1.RolloutStepPreBatchStepHook) {
					return false
				}

				if err := k8sClient.Get(ctx, GetNamespacedName(sts.Name, sts.Namespace), sts); err != nil {
					By(fmt.Sprintf("HappyPath: first batch get sts %s/%s err %v", sts.Namespace, sts.Name, err))
					return false
				}

				if sts.Status.UpdatedReplicas != 1 {
					By(fmt.Sprintf("HappyPath: first batch sts %s/%s UpdatedReplicas %d  not match", sts.Namespace, sts.Name, sts.Status.UpdatedReplicas))
					return false
				}

				if *sts.Spec.UpdateStrategy.RollingUpdate.Partition != 4 {
					By(fmt.Sprintf("HappyPath: first batch sts %s/%s Partition %d  not match", sts.Namespace, sts.Name, *sts.Spec.UpdateStrategy.RollingUpdate.Partition))
					return false
				}

				return true
			}, "60s", "1s").Should(BeTrue())
		}

		// second batch
		{
			By("HappyPath: start second batch")

			_, err := utils.UpdateOnConflict(ctx, k8sClient, k8sClient, rolloutRun, func() error {
				if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Status.RolloutID, rollout.Namespace), rolloutRun); err != nil {
					return err
				}

				if rolloutRun.Annotations == nil {
					rolloutRun.Annotations = make(map[string]string)
				}

				rolloutRun.Annotations[rolloutapi.AnnoManualCommandKey] = rolloutapi.AnnoManualCommandContinue

				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Status.RolloutID, rollout.Namespace), rolloutRun); err != nil {
					return false
				}

				if rolloutRun.Status.BatchStatus.Records[0].State != rolloutv1alpha1.RolloutStepSucceeded {
					// still waiting
					return false
				}

				currentPhase := rolloutRun.Status.Phase
				currentBatchIndex := rolloutRun.Status.BatchStatus.CurrentBatchIndex
				currentBatchState := rolloutRun.Status.BatchStatus.CurrentBatchState

				if !(currentPhase == rolloutv1alpha1.RolloutRunPhasePaused &&
					currentBatchIndex == 2 &&
					currentBatchState == rolloutv1alpha1.RolloutStepPreBatchStepHook) {
					return false
				}

				if err := k8sClient.Get(ctx, GetNamespacedName(sts.Name, sts.Namespace), sts); err != nil {
					By(fmt.Sprintf("HappyPath: second batch get sts %s/%s err %v", sts.Namespace, sts.Name, err))
					return false
				}

				if sts.Status.UpdatedReplicas != 2 {
					By(fmt.Sprintf("HappyPath: second batch sts %s/%s UpdatedReplicas %d  not match", sts.Namespace, sts.Name, sts.Status.UpdatedReplicas))
					return false
				}

				if *sts.Spec.UpdateStrategy.RollingUpdate.Partition != 3 {
					By(fmt.Sprintf("HappyPath: second batch sts %s/%s Partition %d  not match", sts.Namespace, sts.Name, *sts.Spec.UpdateStrategy.RollingUpdate.Partition))
					return false
				}

				return true
			}, "60s", "1s").Should(BeTrue())
		}

		// third batch
		{
			By("HappyPath: start third batch")

			_, err := utils.UpdateOnConflict(ctx, k8sClient, k8sClient, rolloutRun, func() error {
				if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Status.RolloutID, rollout.Namespace), rolloutRun); err != nil {
					return err
				}

				if rolloutRun.Annotations == nil {
					rolloutRun.Annotations = make(map[string]string)
				}
				rolloutRun.Annotations[rolloutapi.AnnoManualCommandKey] = rolloutapi.AnnoManualCommandContinue

				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Status.RolloutID, rollout.Namespace), rolloutRun); err != nil {
					return false
				}

				currentBatchIndex := rolloutRun.Status.BatchStatus.CurrentBatchIndex
				currentBatchState := rolloutRun.Status.BatchStatus.CurrentBatchState
				if currentBatchIndex != 2 ||
					currentBatchState != rolloutv1alpha1.RolloutStepSucceeded ||
					rolloutRun.Status.BatchStatus.Records[2].State != rolloutv1alpha1.RolloutStepSucceeded {
					By(fmt.Sprintf("HappyPath: third batch idx=%d, state=%s", currentBatchIndex, currentBatchState))
					return false
				}

				if err := k8sClient.Get(ctx, GetNamespacedName(sts.Name, sts.Namespace), sts); err != nil {
					By(fmt.Sprintf("HappyPath: third batch get sts %s/%s err %v", sts.Namespace, sts.Name, err))
					return false
				}

				if sts.Status.UpdatedReplicas != 5 {
					By(fmt.Sprintf("HappyPath: third batch sts %s/%s UpdatedReplicas %d  not match", sts.Namespace, sts.Name, sts.Status.UpdatedReplicas))
					return false
				}

				if sts.Spec.UpdateStrategy.RollingUpdate != nil && *sts.Spec.UpdateStrategy.RollingUpdate.Partition != 0 {
					By(fmt.Sprintf("HappyPath: third batch sts %s/%s Partition %d  not match", sts.Namespace, sts.Name, *sts.Spec.UpdateStrategy.RollingUpdate.Partition))
					return false
				}

				return true
			}, "60s", "1s").Should(BeTrue())
		}

		// rollout
		{
			By("HappyPath: check rollout")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Name, rollout.Namespace), rollout); err != nil {
					By(fmt.Sprintf("HappyPath: check rollout get rollout %s/%s error %v", rollout.Namespace, rollout.Name, err))
					return false
				}

				if rollout.Status.Phase != rolloutv1alpha1.RolloutPhaseInitialized {
					By(fmt.Sprintf("HappyPath: check rollout get rollout %s/%s phase %s", rollout.Namespace, rollout.Name, rollout.Status.Phase))
					return false
				}

				if err := k8sClient.Get(ctx, GetNamespacedName(rollout.Status.RolloutID, rollout.Namespace), rolloutRun); err != nil {
					By(fmt.Sprintf("HappyPath: check rollout get rolloutRun %s/%s error %v", rolloutRun.Namespace, rolloutRun.Name, err))
					return false
				}

				if rolloutRun.Status.Phase != rolloutv1alpha1.RolloutRunPhaseSucceeded {
					By(fmt.Sprintf("HappyPath: check rollout get rolloutRun %s/%s phase %s", rolloutRun.Namespace, rolloutRun.Name, rolloutRun.Status.Phase))
					return false
				}

				logf.Log.WithName("e2e-test").Info("show final rolloutRun status", "rolloutRun", rolloutRun)
				return true
			}, "60s", "1s").Should(BeTrue())

		}
	})
})
