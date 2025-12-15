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

package traffictopology

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("traffic-topology-controller", func() {
	It("fed adds 2 clusters", func() {
		for i := 1; i < 3; i++ {
			name := fmt.Sprintf("cluster%d", i)

			var replicas int32 = 1
			err := fedClient.Create(ctx, &appsv1.Deployment{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "default",
					Name:      name,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &v1.LabelSelector{
						MatchLabels: map[string]string{
							"cluster": name,
						},
					},
					Replicas: &replicas,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{
								"cluster": name,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  name,
									Image: "kennethreitz/httpbin",
								},
							},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
		}

		var deployments appsv1.DeploymentList
		err := fedClient.List(ctx, &deployments)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployments.Items).To(HaveLen(2))
	})

	Context("InCluster TrafficTopology", func() {
		tp0 := &v1alpha1.TrafficTopology{
			TypeMeta: v1.TypeMeta{
				Kind:       "TrafficTopology",
				APIVersion: "rollout.kusionstack.io/v1alpha1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "tp-controller-ut-tp0",
				Namespace: "default",
			},
			Spec: v1alpha1.TrafficTopologySpec{
				WorkloadRef: v1alpha1.WorkloadRef{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Match: v1alpha1.ResourceMatch{
						Selector: &v1.LabelSelector{
							MatchLabels: map[string]string{
								"app":       "tp-controller-ut-sts",
								"component": "tp-controller-ut-comp",
							},
						},
					},
				},
				TrafficType: v1alpha1.InClusterTrafficType,
				Backend: v1alpha1.BackendRef{
					Name: "tp-controller-ut-comp-backend",
				},
				Routes: []v1alpha1.RouteRef{
					{
						Name: "tp-controller-ut-comp-route0",
					},
				},
			},
		}

		It("no workloads selected", func() {
			err := fedClient.Create(clusterinfo.WithCluster(ctx, clusterinfo.Fed), tp0)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				// wait for traffic topology reconcile
				temp := &v1alpha1.TrafficTopology{}
				fedClient.Get(context.Background(), client.ObjectKeyFromObject(tp0), temp)
				return temp.Status.ObservedGeneration == temp.Generation
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				tpTmp := &v1alpha1.TrafficTopology{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Name:      tp0.Name,
					Namespace: tp0.Namespace,
				}, tpTmp)
				if err != nil {
					return false
				}
				if tpTmp.Status.ObservedGeneration != tpTmp.Generation {
					return false
				}
				for _, condition := range tpTmp.Status.Conditions {
					if condition.Type == v1alpha1.TrafficTopologyConditionReady && condition.Status == v1.ConditionFalse {
						return true
					}
				}
				return false
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		It("2 local cluster has workloads selected by traffictopology", func() {
			var replicas int32 = 1
			stsCluster1 := &appsv1.StatefulSet{
				ObjectMeta: v1.ObjectMeta{
					Name:      "tp-ut-sts1",
					Namespace: "default",
					Labels: map[string]string{
						"app":       "tp-controller-ut-sts",
						"component": "tp-controller-ut-comp",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Selector: &v1.LabelSelector{
						MatchLabels: map[string]string{
							"app":       "tp-controller-ut-sts",
							"component": "tp-controller-ut-comp",
						},
					},
					Replicas: &replicas,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{
								"app":       "tp-controller-ut-sts",
								"component": "tp-controller-ut-comp",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "tp-ut-sts1",
									Image: "kennethreitz/httpbin",
								},
							},
						},
					},
				},
			}
			err := clusterClient1.Create(ctx, stsCluster1)
			Expect(err).ShouldNot(HaveOccurred())

			stsCluster2 := &appsv1.StatefulSet{
				ObjectMeta: v1.ObjectMeta{
					Name:      "tp-ut-sts2",
					Namespace: "default",
					Labels: map[string]string{
						"app":       "tp-controller-ut-sts",
						"component": "tp-controller-ut-comp",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Selector: &v1.LabelSelector{
						MatchLabels: map[string]string{
							"app":       "tp-controller-ut-sts",
							"component": "tp-controller-ut-comp",
						},
					},
					Replicas: &replicas,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{
								"app":       "tp-controller-ut-sts",
								"component": "tp-controller-ut-comp",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "tp-ut-sts2",
									Image: "kennethreitz/httpbin",
								},
							},
						},
					},
				},
			}
			err = clusterClient2.Create(ctx, stsCluster2)
			Expect(err).ShouldNot(HaveOccurred())

			// trigger traffic topology reconcile
			tpTmp := &v1alpha1.TrafficTopology{}
			err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
				Name:      tp0.Name,
				Namespace: tp0.Namespace,
			}, tpTmp)
			Expect(err).ShouldNot(HaveOccurred())
			if tpTmp.Labels == nil {
				tpTmp.Labels = make(map[string]string)
			}
			tpTmp.Labels["trigger"] = "x"
			err = fedClient.Update(clusterinfo.WithCluster(ctx, clusterinfo.Fed), tpTmp)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				tpTmp = &v1alpha1.TrafficTopology{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Name:      tp0.Name,
					Namespace: tp0.Namespace,
				}, tpTmp)
				if err != nil {
					return false
				}
				for _, condition := range tpTmp.Status.Conditions {
					if condition.Type == v1alpha1.TrafficTopologyConditionReady && condition.Status == v1.ConditionTrue {
						return true
					}
				}
				return false
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp := &v1alpha1.BackendRouting{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Namespace: "default",
					Name:      tpTmp.Name + "-ics-cluster1",
				}, brTmp)
				if err != nil {
					return false
				}
				if brTmp.Spec.TrafficType == v1alpha1.InClusterTrafficType && brTmp.Spec.Backend.Cluster == "cluster1" &&
					brTmp.Spec.Backend.Kind == "Service" && brTmp.Spec.Backend.Name == "tp-controller-ut-comp-backend" &&
					len(brTmp.Spec.Routes) == 1 && brTmp.Spec.Routes[0].Kind == "HTTPRoute" &&
					brTmp.Spec.Routes[0].Name == "tp-controller-ut-comp-route0" && brTmp.Spec.Routes[0].Cluster == "cluster1" {
					return true
				}
				return false
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp := &v1alpha1.BackendRouting{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Namespace: "default",
					Name:      tpTmp.Name + "-ics-cluster2",
				}, brTmp)
				if err != nil {
					return false
				}
				if brTmp.Spec.TrafficType == v1alpha1.InClusterTrafficType && brTmp.Spec.Backend.Cluster == "cluster2" &&
					brTmp.Spec.Backend.Kind == "Service" && brTmp.Spec.Backend.Name == "tp-controller-ut-comp-backend" &&
					len(brTmp.Spec.Routes) == 1 && brTmp.Spec.Routes[0].Kind == "HTTPRoute" &&
					brTmp.Spec.Routes[0].Name == "tp-controller-ut-comp-route0" && brTmp.Spec.Routes[0].Cluster == "cluster2" {
					return true
				}
				return false
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		It("cluster2's workload deleted", func() {
			stsCluster2 := &appsv1.StatefulSet{
				ObjectMeta: v1.ObjectMeta{
					Name:      "tp-ut-sts2",
					Namespace: "default",
					Labels: map[string]string{
						"app":       "tp-controller-ut-sts",
						"component": "tp-controller-ut-comp",
					},
				},
			}
			err := clusterClient2.Delete(ctx, stsCluster2)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				stsTmp := &appsv1.StatefulSet{}
				err = clusterClient2.Get(ctx, types.NamespacedName{
					Namespace: "default",
					Name:      "tp-ut-sts2",
				}, stsTmp)
				return errors.IsNotFound(err)
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			// trigger traffic topology reconcile
			tpTmp := &v1alpha1.TrafficTopology{}
			err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
				Name:      tp0.Name,
				Namespace: tp0.Namespace,
			}, tpTmp)
			if tpTmp.Labels == nil {
				tpTmp.Labels = make(map[string]string)
			}
			tpTmp.Labels["trigger"] = "xx"
			err = fedClient.Update(clusterinfo.WithCluster(ctx, clusterinfo.Fed), tpTmp)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				tpTmp := &v1alpha1.TrafficTopology{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Name:      tp0.Name,
					Namespace: tp0.Namespace,
				}, tpTmp)
				if err != nil {
					return false
				}
				for _, condition := range tpTmp.Status.Conditions {
					if condition.Type == v1alpha1.TrafficTopologyConditionReady && condition.Status == v1.ConditionTrue {
						return true
					}
				}
				return false
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp := &v1alpha1.BackendRouting{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Namespace: "default",
					Name:      tp0.Name + "-ics-cluster1",
				}, brTmp)
				if err != nil {
					return false
				}
				if brTmp.Spec.TrafficType == v1alpha1.InClusterTrafficType && brTmp.Spec.Backend.Cluster == "cluster1" &&
					brTmp.Spec.Backend.Kind == "Service" && brTmp.Spec.Backend.Name == "tp-controller-ut-comp-backend" &&
					len(brTmp.Spec.Routes) == 1 && brTmp.Spec.Routes[0].Kind == "HTTPRoute" &&
					brTmp.Spec.Routes[0].Name == "tp-controller-ut-comp-route0" && brTmp.Spec.Routes[0].Cluster == "cluster1" {
					return true
				}
				return false
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp := &v1alpha1.BackendRouting{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Namespace: "default",
					Name:      tp0.Name + "-ics-cluster2",
				}, brTmp)
				return errors.IsNotFound(err)
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		It("traffictopology deleting", func() {
			err := fedClient.Delete(ctx, tp0)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				brTmp := &v1alpha1.BackendRouting{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Namespace: "default",
					Name:      tp0.Name + "-ics-cluster1",
				}, brTmp)
				return errors.IsNotFound(err)
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp := &v1alpha1.TrafficTopology{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Namespace: tp0.Namespace,
					Name:      tp0.Name,
				}, brTmp)
				return errors.IsNotFound(err)
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})

	Context("MultiCluster TrafficTopology", func() {
		tp1 := &v1alpha1.TrafficTopology{
			TypeMeta: v1.TypeMeta{
				Kind:       "TrafficTopology",
				APIVersion: "rollout.kusionstack.io/v1alpha1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      "tp-controller-ut-tp1",
				Namespace: "default",
			},
			Spec: v1alpha1.TrafficTopologySpec{
				WorkloadRef: v1alpha1.WorkloadRef{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Match: v1alpha1.ResourceMatch{
						Selector: &v1.LabelSelector{
							MatchLabels: map[string]string{
								"app":       "tp-controller-ut-sts",
								"component": "tp-controller-ut-comp",
							},
						},
					},
				},
				TrafficType: v1alpha1.MultiClusterTrafficType,
				Backend: v1alpha1.BackendRef{
					Name: "tp-controller-ut-comp-backend",
				},
				Routes: []v1alpha1.RouteRef{
					{
						Name: "tp-controller-ut-comp-route0",
					},
				},
			},
		}

		It("cluster1 has workload selected", func() {
			err := fedClient.Create(clusterinfo.WithCluster(ctx, clusterinfo.Fed), tp1)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				// wait for traffic topology reconcile
				temp := &v1alpha1.TrafficTopology{}
				fedClient.Get(context.Background(), client.ObjectKeyFromObject(tp1), temp)
				return temp.Status.ObservedGeneration == temp.Generation
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				tpTmp := &v1alpha1.TrafficTopology{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Name:      tp1.Name,
					Namespace: tp1.Namespace,
				}, tpTmp)
				if err != nil {
					return false
				}

				if tpTmp.Status.ObservedGeneration != tpTmp.Generation {
					return false
				}

				for _, condition := range tpTmp.Status.Conditions {
					if condition.Type == v1alpha1.TrafficTopologyConditionReady && condition.Status == v1.ConditionTrue {
						return true
					}
				}
				return false
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp := &v1alpha1.BackendRouting{}
				err = fedClient.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
					Namespace: "default",
					Name:      tp1.Name + "-msc",
				}, brTmp)
				if err != nil {
					return false
				}
				if brTmp.Spec.TrafficType == v1alpha1.MultiClusterTrafficType && brTmp.Spec.Backend.Cluster == "" &&
					brTmp.Spec.Backend.Kind == "Service" && brTmp.Spec.Backend.Name == "tp-controller-ut-comp-backend" &&
					len(brTmp.Spec.Routes) == 1 && brTmp.Spec.Routes[0].Kind == "HTTPRoute" &&
					brTmp.Spec.Routes[0].Name == "tp-controller-ut-comp-route0" && brTmp.Spec.Routes[0].Cluster == "" {
					return true
				}
				return false
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})
})

func TestTrafficTopologyController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "traffic-topology-controller test")
}
