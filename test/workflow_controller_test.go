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

package test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/KusionStack/rollout/api/v1alpha1"
)

const (
	WorkflowNamePrefix = "testflow"
	WorkflowNamespace  = "default"
)

var _ = Describe("Workflow controller", func() {

	Describe("Creating a Workflow", func() {
		var ctx = context.Background()
		var flow *v1alpha1.Workflow

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, flow)).Should(Succeed())
		})

		When("Creating an empty Workflow", func() {
			BeforeEach(func() {
				flow = &v1alpha1.Workflow{
					TypeMeta: metav1.TypeMeta{
						APIVersion: v1alpha1.GroupVersion.String(),
						Kind:       "Workflow",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      name(WorkflowNamePrefix),
						Namespace: WorkflowNamespace,
					},
					Spec: v1alpha1.WorkflowSpec{
						// Add fields here
					},
				}
			})
			It("Should create successfully", func() {
				// TODO: use DeferCleanup to delete the Workflow, which depends on Ginkgo V2
				By("By deleting the Workflow")
				Expect(k8sClient.Delete(ctx, flow)).Should(Succeed())
			})
		})

		When("Creating a Pending Workflow", func() {
			BeforeEach(func() {
				flow = &v1alpha1.Workflow{
					TypeMeta: metav1.TypeMeta{
						APIVersion: v1alpha1.GroupVersion.String(),
						Kind:       "Workflow",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      name(WorkflowNamePrefix),
						Namespace: WorkflowNamespace,
					},
					Spec: v1alpha1.WorkflowSpec{
						Status: v1alpha1.WorkflowSpecStatusPending,
						Params: v1alpha1.Params{},
						Tasks:  nil,
					},
				}
			})
			It("Should in Pending status", func() {
				Eventually(func() bool {
					Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
					return flow.Status.IsPending()
				}).Should(BeTrue())
				By("By deleting the Workflow")
				Expect(k8sClient.Delete(ctx, flow)).Should(Succeed())
			})
		})

		Describe("Creating a Workflow with Echo Tasks", func() {
			When("Task is in Succeeded status", func() {
				BeforeEach(func() {
					flow = workflowWithEchoTasks("Succeeded")
				})
				It("Should in Succeeded status", func() {
					Eventually(func() bool {
						Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
						return flow.Status.IsSucceeded()
					}).Should(BeTrue())
					By("By deleting the Workflow")
					Expect(k8sClient.Delete(ctx, flow)).Should(Succeed())
				})
			})

			When("Task is in Paused status", func() {
				BeforeEach(func() {
					flow = workflowWithEchoTasks("Paused")
				})
				It("Should in Paused status", func() {
					Eventually(func() bool {
						Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
						return flow.Status.IsPaused()
					}).Should(BeTrue())
					By("By deleting the Workflow")
					Expect(k8sClient.Delete(ctx, flow)).Should(Succeed())
				})
			})
		})
	})

	Describe("Canceling a Workflow", func() {
		var ctx = context.Background()
		var flow *v1alpha1.Workflow

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, flow)).Should(Succeed())
		})

		When("Workflow is paused", func() {
			BeforeEach(func() {
				flow = workflowWithEchoTasks("Paused")
			})
			It("Can be cancelled successfully", func() {
				By("Checking the workflow is in Paused status, task is in Paused status")
				Eventually(func() bool {
					Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
					return flow.Status.IsPaused()
				}).Should(BeTrue())

				By("Canceling the Workflow")
				flow.Spec.Status = v1alpha1.WorkflowSpecStatusCancelled
				Expect(k8sClient.Update(ctx, flow)).Should(Succeed())

				By("Checking the workflow is in Cancelled status, task is in Cancelled status")
				Eventually(func() bool {
					Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
					return flow.Status.IsCancelled()
				}, 10, 1).Should(BeTrue())

				By("By deleting the Workflow")
				Expect(k8sClient.Delete(ctx, flow)).Should(Succeed())

			})
		})

		When("Workflow is pending", func() {
			BeforeEach(func() {
				flow = workflowWithEchoTasks("")
				flow.Spec.Status = v1alpha1.WorkflowSpecStatusPending
			})
			It("Can be cancelled successfully", func() {
				By("Checking the workflow is in Pending status")
				Eventually(func() bool {
					Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
					return flow.Status.IsPending()
				}).Should(BeTrue())

				By("Canceling the Workflow")
				flow.Spec.Status = v1alpha1.WorkflowSpecStatusCancelled
				Expect(k8sClient.Update(ctx, flow)).Should(Succeed())

				By("Checking the workflow is in Cancelled status")
				Eventually(func() bool {
					Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
					return flow.Status.IsCancelled()
				}).Should(BeTrue())
			})
		})

		When("Has multiple Running tasks", func() {
			BeforeEach(func() {
				// 3 Running tasks, 0 is Running, 1 is Pending, 2 is Pending
				flow = workflowWithEchoTasks("Running", "Running", "Running")
			})

			It("Can be cancelled successfully", func() {
				By("Checking the workflow is in Running status, task is in Running status")
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
					g.Expect(flow.Status.IsRunning()).Should(BeTrue())
					g.Expect(len(flow.Status.Tasks)).Should(Equal(3))
					g.Expect(flow.Status.Tasks[0].IsRunning()).Should(BeTrue())
					g.Expect(flow.Status.Tasks[1].SuccessCondition).Should(BeNil())
					g.Expect(flow.Status.Tasks[2].SuccessCondition).Should(BeNil())
				}, 10, 1).Should(Succeed())

				By("Canceling the Workflow")
				flow.Spec.Status = v1alpha1.WorkflowSpecStatusCancelled
				Expect(k8sClient.Update(ctx, flow)).Should(Succeed())

				By("Checking the workflow is in Cancelled status, task is in Cancelled status")
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
					g.Expect(flow.Status.IsCancelled()).Should(BeTrue())
					g.Expect(len(flow.Status.Tasks)).Should(Equal(3))
					g.Expect(flow.Status.Tasks[0].IsCancelled()).Should(BeTrue())
					g.Expect(flow.Status.Tasks[1].SuccessCondition).Should(BeNil())
					g.Expect(flow.Status.Tasks[2].SuccessCondition).Should(BeNil())
				}, 10, 1).Should(Succeed())

				By("By deleting the Workflow")
				Expect(k8sClient.Delete(ctx, flow)).Should(Succeed())
			})
		})

		When("Has multiple Succeeded tasks", func() {
			BeforeEach(func() {
				// 3 Running tasks, 0 is Succeeded, 1 is Running, 2 is Pending
				flow = workflowWithEchoTasks("Succeeded", "Running", "Running")
			})

			It("Can be cancelled successfully", func() {
				By("Checking the workflow is in Running status, task is in Running status")
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
					g.Expect(flow.Status.IsRunning()).Should(BeTrue())
					g.Expect(len(flow.Status.Tasks)).Should(Equal(3))
					g.Expect(flow.Status.Tasks[0].IsSucceeded()).Should(BeTrue())
					g.Expect(flow.Status.Tasks[1].IsRunning()).Should(BeTrue())
					g.Expect(flow.Status.Tasks[2].SuccessCondition).Should(BeNil())
				}, 10, 1).Should(Succeed())

				By("Canceling the Workflow")
				flow.Spec.Status = v1alpha1.WorkflowSpecStatusCancelled
				Expect(k8sClient.Update(ctx, flow)).Should(Succeed())

				By("Checking the workflow is in Cancelled status, task is in Cancelled status")
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, GetNamespacedName(flow.Name, flow.Namespace), flow)).Should(Succeed())
					g.Expect(flow.Status.IsCancelled()).Should(BeTrue())
					g.Expect(len(flow.Status.Tasks)).Should(Equal(3))
					g.Expect(flow.Status.Tasks[0].IsSucceeded()).Should(BeTrue())
					g.Expect(flow.Status.Tasks[1].IsCancelled()).Should(BeTrue())
					g.Expect(flow.Status.Tasks[2].SuccessCondition).Should(BeNil())
				}, 10, 1).Should(Succeed())

				By("By deleting the Workflow")
				Expect(k8sClient.Delete(ctx, flow)).Should(Succeed())
			})
		})
	})
})

func GetNamespacedName(name string, namespace string) client.ObjectKey {
	return client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
}

// workflowWithEchoTasks generate a workflow with echo tasks
func workflowWithEchoTasks(taskStatuses ...string) *v1alpha1.Workflow {
	flowName := name(WorkflowNamePrefix)

	var tasks []v1alpha1.WorkflowTask
	for i, taskStatus := range taskStatuses {
		taskName := fmt.Sprintf("%s-task-%d", flowName, i)
		tasks = append(tasks, v1alpha1.WorkflowTask{
			Name: taskName,
			TaskSpec: v1alpha1.TaskSpec{
				Echo: &v1alpha1.EchoTask{
					Message: "hello from " + taskName,
				},
				Params: v1alpha1.Params{
					{
						Name: "expectStatus",
						Value: v1alpha1.ParamValue{
							Type:        v1alpha1.ParamValueTypeString,
							StringValue: taskStatus,
						},
					},
				},
			},
		})
	}

	for i := 1; i < len(tasks); i++ {
		tasks[i].RunAfter = []string{tasks[i-1].Name}
	}

	return &v1alpha1.Workflow{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "Workflow",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      flowName,
			Namespace: WorkflowNamespace,
		},
		Spec: v1alpha1.WorkflowSpec{
			Tasks: tasks,
		},
	}
}

// name generate workflow name with prefix and timestamp of millisecond
func name(prefix string) string {
	return prefix + "-" + uuid.NewString()
}
