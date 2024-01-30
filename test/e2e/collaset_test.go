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
	"fmt"
	"net/http/httptest"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/webhook/probe/http"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload/collaset"
	"kusionstack.io/rollout/test/e2e/builder"
)

var _ = Describe("CollaSet", func() {
	ctx := context.Background()

	var ts *httptest.Server
	var cls = &operatingv1alpha1.CollaSet{}
	var rollout = &rolloutv1alpha1.Rollout{}
	var rolloutRun = &rolloutv1alpha1.RolloutRun{}
	var strategy *rolloutv1alpha1.RolloutStrategy

	BeforeEach(func() {
		// prepare http server
		ts = http.NewTestHTTPServer()

		// Prepare Cls
		cls = builder.NewCollsetBuilder().Namespace(e2eNamespace).Build()
		{
			By(fmt.Sprintf("PrepareCls: create cls %s/%s", cls.Namespace, cls.Name))
			Expect(k8sClient.Create(ctx, cls)).Should(Succeed())

			Eventually(func() bool {
				tmpCls := &operatingv1alpha1.CollaSet{}
				if err := k8sClient.Get(ctx, GetNamespacedName(cls.Name, cls.Namespace), tmpCls); err != nil {
					By(fmt.Sprintf("PrepareCls: Get cls %s/%s err %v", cls.Namespace, cls.Name, err))
					return false
				}

				if tmpCls.Status.ObservedGeneration != tmpCls.Generation {
					By(fmt.Sprintf("PrepareCls: checkGeneration %d/%d", tmpCls.Status.ObservedGeneration, tmpCls.Generation))
					return false
				}

				if tmpCls.Status.AvailableReplicas != *cls.Spec.Replicas {
					By(fmt.Sprintf("PrepareCls: Replicas %d/%d", tmpCls.Status.AvailableReplicas, *cls.Spec.Replicas))
					return false
				}

				mergeEnv := mergeEnvVar(
					cls.Spec.Template.Spec.Containers[0].Env,
					corev1.EnvVar{Name: "Foo", Value: "Bar"},
				)
				tmpCls.Spec.Template.Spec.Containers[0].Env = mergeEnv
				tmpCls.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition = ptr.To(int32(0))
				if err := k8sClient.Update(ctx, tmpCls); err != nil {
					By("Update cls error")
					return false
				}

				return true
			}, "60s", "1s").Should(BeTrue())

			Eventually(func() bool {
				tmpCls := &operatingv1alpha1.CollaSet{}
				if err := k8sClient.Get(ctx, GetNamespacedName(cls.Name, cls.Namespace), tmpCls); err != nil {
					return false
				}

				if tmpCls.Status.UpdatedReplicas != 0 {
					return false
				}

				if tmpCls.Status.ObservedGeneration != tmpCls.Generation {
					return false
				}

				cls = tmpCls

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

			rollout = builder.NewRollout().Namespace(e2eNamespace).StrategyName(strategy.Name).Build(collaset.GVK, map[string]string{
				"app.kubernetes.io/name":     cls.Labels["app.kubernetes.io/name"],
				"app.kubernetes.io/instance": cls.Labels["app.kubernetes.io/instance"],
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

		Expect(k8sClient.Delete(ctx, cls)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, rollout)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, strategy)).Should(Succeed())
		Eventually(func() bool {
			clean := true
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cls), cls)
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
				rollout.Annotations["rollout.kusionstack.io/trigger"] = strconv.FormatInt(time.Now().Unix(), 10)
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

				if rolloutRun.Status.BatchStatus == nil ||
					rolloutRun.Status.Phase != rolloutv1alpha1.RolloutRunPhaseProgressing {
					return false
				}

				if rolloutRun.Status.BatchStatus.RolloutBatchStatus.CurrentBatchIndex != 0 ||
					rolloutRun.Status.BatchStatus.RolloutBatchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePaused {
					return false
				}

				By("HappyPath: ensure trigger rolloutRun")

				return true
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

				currentBatchIndex := rolloutRun.Status.BatchStatus.CurrentBatchIndex
				currentBatchState := rolloutRun.Status.BatchStatus.CurrentBatchState
				if currentBatchIndex != 1 ||
					currentBatchState != rolloutv1alpha1.BatchStepStatePaused ||
					rolloutRun.Status.BatchStatus.Records[0].State != rolloutv1alpha1.BatchStepStateSucceeded {
					By(fmt.Sprintf("HappyPath: first batch idx=%d, state=%s", currentBatchIndex, currentBatchState))
					return false
				}

				if err := k8sClient.Get(ctx, GetNamespacedName(cls.Name, cls.Namespace), cls); err != nil {
					By(fmt.Sprintf("HappyPath: first batch get cls %s/%s err %v", cls.Namespace, cls.Name, err))
					return false
				}

				if cls.Status.UpdatedReplicas != 1 {
					By(fmt.Sprintf("HappyPath: first batch cls %s/%s UpdatedReplicas %d  not match", cls.Namespace, cls.Name, cls.Status.UpdatedReplicas))
					return false
				}

				if *cls.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition != 1 {
					By(fmt.Sprintf("HappyPath: first batch cls %s/%s Partition %d  not match", cls.Namespace, cls.Name, *cls.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition))
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

				currentBatchIndex := rolloutRun.Status.BatchStatus.CurrentBatchIndex
				currentBatchState := rolloutRun.Status.BatchStatus.CurrentBatchState
				if currentBatchIndex != 2 ||
					currentBatchState != rolloutv1alpha1.BatchStepStatePaused ||
					rolloutRun.Status.BatchStatus.Records[1].State != rolloutv1alpha1.BatchStepStateSucceeded {
					By(fmt.Sprintf("HappyPath: second batch idx=%d, state=%s", currentBatchIndex, currentBatchState))
					return false
				}

				if err := k8sClient.Get(ctx, GetNamespacedName(cls.Name, cls.Namespace), cls); err != nil {
					By(fmt.Sprintf("HappyPath: second batch get cls %s/%s err %v", cls.Namespace, cls.Name, err))
					return false
				}

				if cls.Status.UpdatedReplicas != 2 {
					By(fmt.Sprintf("HappyPath: second batch cls %s/%s UpdatedReplicas %d  not match", cls.Namespace, cls.Name, cls.Status.UpdatedReplicas))
					return false
				}

				if *cls.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition != 2 {
					By(fmt.Sprintf("HappyPath: second batch cls %s/%s Partition %d  not match", cls.Namespace, cls.Name, *cls.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition))
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
					currentBatchState != rolloutv1alpha1.BatchStepStateSucceeded ||
					rolloutRun.Status.BatchStatus.Records[2].State != rolloutv1alpha1.BatchStepStateSucceeded {
					By(fmt.Sprintf("HappyPath: third batch idx=%d, state=%s", currentBatchIndex, currentBatchState))
					return false
				}

				if err := k8sClient.Get(ctx, GetNamespacedName(cls.Name, cls.Namespace), cls); err != nil {
					By(fmt.Sprintf("HappyPath: third batch get cls %s/%s err %v", cls.Namespace, cls.Name, err))
					return false
				}

				if cls.Status.UpdatedReplicas != 5 {
					By(fmt.Sprintf("HappyPath: third batch cls %s/%s UpdatedReplicas %d  not match", cls.Namespace, cls.Name, cls.Status.UpdatedReplicas))
					return false
				}

				if *cls.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition != 5 {
					By(fmt.Sprintf("HappyPath: third batch cls %s/%s Partition %d  not match", cls.Namespace, cls.Name, *cls.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition))
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

				return true
			}, "60s", "1s").Should(BeTrue())
		}
	})
})
