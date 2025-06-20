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

package backendrouting

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
)

var _ = Describe("backend-routing-controller", func() {
	Context("InCluster BackendRouting", func() {
		It("create local cluster", func() {
			var replicas int32 = 1
			err := fedClient.Create(ctx, &appsv1.Deployment{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "default",
					Name:      "cluster1",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &v1.LabelSelector{
						MatchLabels: map[string]string{
							"cluster": "cluster1",
						},
					},
					Replicas: &replicas,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{
								"cluster": "cluster1",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "cluster1",
									Image: "kennethreitz/httpbin",
								},
							},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		br0 := v1alpha1.BackendRouting{
			ObjectMeta: v1.ObjectMeta{
				Name:       "br-controller-ut-br0",
				Namespace:  "default",
				Generation: int64(1),
			},
			Spec: v1alpha1.BackendRoutingSpec{
				TrafficType: v1alpha1.InClusterTrafficType,
				Backend: v1alpha1.CrossClusterObjectReference{
					ObjectTypeRef: v1alpha1.ObjectTypeRef{
						APIVersion: "v1",
						Kind:       "Service",
					},
					CrossClusterObjectNameReference: v1alpha1.CrossClusterObjectNameReference{
						Cluster: "cluster1",
						Name:    "br-controller-ut-svc1",
					},
				},
				Routes: []v1alpha1.CrossClusterObjectReference{
					{
						ObjectTypeRef: v1alpha1.ObjectTypeRef{
							APIVersion: "networking.k8s.io/v1",
							Kind:       "Ingress",
						},
						CrossClusterObjectNameReference: v1alpha1.CrossClusterObjectNameReference{
							Cluster: "cluster1",
							Name:    "br-controller-ut-igs1",
						},
					},
				},
			},
		}

		It("Initialization of traffic", func() {
			err := fedClient.Create(clusterinfo.WithCluster(ctx, clusterinfo.Fed), &br0)
			Expect(err).ShouldNot(HaveOccurred())

			time.Sleep(3 * time.Second)

			// service not created yet
			Eventually(func() bool {
				brTmp := &v1alpha1.BackendRouting{}
				err = fedClient.Get(ctx, types.NamespacedName{
					Name:      br0.Name,
					Namespace: br0.Namespace,
				}, brTmp)
				if err != nil {
					return false
				}
				return brTmp.Status.Phase == v1alpha1.BackendUpgrading
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			// service created but ingress not created yet
			err = clusterClient1.Create(ctx, &corev1.Service{
				ObjectMeta: v1.ObjectMeta{
					Name:      "br-controller-ut-svc1",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Protocol:   corev1.ProtocolTCP,
							Port:       80,
							TargetPort: intstr.IntOrString{IntVal: 80},
						},
					},
				},
			})
			Expect(err).ShouldNot(HaveOccurred())
			// trigger traffictopology reconcile
			brTmp := &v1alpha1.BackendRouting{}
			err = fedClient.Get(ctx, types.NamespacedName{
				Name:      br0.Name,
				Namespace: br0.Namespace,
			}, brTmp)
			Expect(err).ShouldNot(HaveOccurred())
			if brTmp.Labels == nil {
				brTmp.Labels = make(map[string]string)
			}
			brTmp.Labels["trigger-reconcile-ut"] = "x"
			err = fedClient.Update(ctx, brTmp)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				brTmp = &v1alpha1.BackendRouting{}
				err = fedClient.Get(ctx, types.NamespacedName{
					Name:      br0.Name,
					Namespace: br0.Namespace,
				}, brTmp)
				if err != nil {
					return false
				}
				// status would be ready since we didn't check whether route -> origin
				return brTmp.Status.Phase == v1alpha1.Ready && brTmp.Generation == brTmp.Status.ObservedGeneration &&
					*brTmp.Status.Backends.Origin.Conditions.Ready
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			// create ingress
			pathType := networkingv1.PathTypePrefix
			err = clusterClient1.Create(ctx, &networkingv1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Name:      "br-controller-ut-igs1",
					Namespace: "default",
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "br-controller-ut-svc1",
													Port: networkingv1.ServiceBackendPort{
														Number: int32(80),
													},
												},
											},
											Path:     "/",
											PathType: &pathType,
										},
									},
								},
							},
						},
					},
				},
			})
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Stable route ready", func() {
			// add forwarding to backendrouting
			brTmp := &v1alpha1.BackendRouting{}
			err := fedClient.Get(ctx, types.NamespacedName{
				Name:      br0.Name,
				Namespace: br0.Namespace,
			}, brTmp)
			Expect(err).ShouldNot(HaveOccurred())
			brTmp.Spec.Forwarding = &v1alpha1.BackendForwarding{
				Stable: v1alpha1.StableBackendRule{
					Name: "br-controller-ut-svc1-stable",
				},
			}
			err = fedClient.Update(ctx, brTmp)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				igsTmp := &networkingv1.Ingress{}
				err = clusterClient1.Get(ctx, types.NamespacedName{
					Name:      "br-controller-ut-igs1",
					Namespace: "default",
				}, igsTmp)
				if err != nil {
					return false
				}
				return igsTmp.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name == "br-controller-ut-svc1-stable"
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp := &v1alpha1.BackendRouting{}
				err = fedClient.Get(ctx, types.NamespacedName{
					Name:      br0.Name,
					Namespace: br0.Namespace,
				}, brTmp)
				if err != nil {
					return false
				}
				return brTmp.Status.Phase == v1alpha1.Ready && brTmp.Generation == brTmp.Status.ObservedGeneration
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		It("Canary ready", func() {
			// add canary to backendrouting
			brTmp := &v1alpha1.BackendRouting{}
			err := fedClient.Get(ctx, types.NamespacedName{
				Name:      br0.Name,
				Namespace: br0.Namespace,
			}, brTmp)
			Expect(err).ShouldNot(HaveOccurred())
			canaryWeight := int32(50)
			brTmp.Spec.Forwarding.Canary = v1alpha1.CanaryBackendRule{
				Name: "br-controller-ut-svc1-canary",
				TrafficStrategy: v1alpha1.TrafficStrategy{
					Weight: &canaryWeight,
					HTTPRule: &v1alpha1.HTTPRouteRule{
						Matches: []v1alpha1.HTTPRouteMatch{
							{
								Headers: []gatewayapiv1.HTTPHeaderMatch{
									{
										Name:  "env",
										Value: "canary",
									},
								},
							},
						},
						Filter: v1alpha1.HTTPRouteFilter{
							RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
								Set: []gatewayapiv1.HTTPHeader{
									{
										Name:  "x-mse-tag",
										Value: "canary",
									},
								},
							},
						},
					},
				},
			}
			err = fedClient.Update(ctx, brTmp)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				igsTmp := &networkingv1.Ingress{}
				err = clusterClient1.Get(ctx, types.NamespacedName{
					Name:      "br-controller-ut-igs1-canary",
					Namespace: "default",
				}, igsTmp)
				if err != nil {
					return false
				}
				return igsTmp.Annotations["nginx.ingress.kubernetes.io/canary"] == "true" &&
					igsTmp.Annotations["nginx.ingress.kubernetes.io/canary-weight"] == "50" &&
					igsTmp.Annotations["nginx.ingress.kubernetes.io/canary-by-header-value"] == "canary" &&
					igsTmp.Annotations["mse.ingress.kubernetes.io/request-header-control-update"] == "" &&
					igsTmp.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name == "br-controller-ut-svc1-canary"
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp = &v1alpha1.BackendRouting{}
				err = fedClient.Get(ctx, types.NamespacedName{
					Name:      br0.Name,
					Namespace: br0.Namespace,
				}, brTmp)
				if err != nil {
					return false
				}
				return brTmp.Status.Phase == v1alpha1.Ready && brTmp.Generation == brTmp.Status.ObservedGeneration
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			// update weight
			brTmp = &v1alpha1.BackendRouting{}
			err = fedClient.Get(ctx, types.NamespacedName{
				Name:      br0.Name,
				Namespace: br0.Namespace,
			}, brTmp)
			Expect(err).ShouldNot(HaveOccurred())
			canaryWeight = int32(20)
			brTmp.Spec.Forwarding.Canary = v1alpha1.CanaryBackendRule{
				Name: "br-controller-ut-svc1-canary",
				TrafficStrategy: v1alpha1.TrafficStrategy{
					Weight: &canaryWeight,
					HTTPRule: &v1alpha1.HTTPRouteRule{
						Matches: []v1alpha1.HTTPRouteMatch{
							{
								Headers: []gatewayapiv1.HTTPHeaderMatch{
									{
										Name:  "env",
										Value: "canary",
									},
								},
							},
						},
						Filter: v1alpha1.HTTPRouteFilter{
							RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
								Set: []gatewayapiv1.HTTPHeader{
									{
										Name:  "x-mse-tag",
										Value: "canary",
									},
								},
							},
						},
					},
				},
			}
			err = fedClient.Update(ctx, brTmp)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				igsTmp := &networkingv1.Ingress{}
				err = clusterClient1.Get(ctx, types.NamespacedName{
					Name:      "br-controller-ut-igs1-canary",
					Namespace: "default",
				}, igsTmp)
				if err != nil {
					return false
				}
				return igsTmp.Annotations["nginx.ingress.kubernetes.io/canary-weight"] == "20" &&
					igsTmp.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name == "br-controller-ut-svc1-canary"
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp = &v1alpha1.BackendRouting{}
				err = fedClient.Get(ctx, types.NamespacedName{
					Name:      br0.Name,
					Namespace: br0.Namespace,
				}, brTmp)
				if err != nil {
					return false
				}
				return brTmp.Status.Phase == v1alpha1.Ready && brTmp.Generation == brTmp.Status.ObservedGeneration
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		It("End Canary", func() {
			brTmp := &v1alpha1.BackendRouting{}
			err := fedClient.Get(ctx, types.NamespacedName{
				Name:      br0.Name,
				Namespace: br0.Namespace,
			}, brTmp)
			Expect(err).ShouldNot(HaveOccurred())
			brTmp.Spec.Forwarding = &v1alpha1.BackendForwarding{
				Stable: brTmp.Spec.Forwarding.Stable,
			}
			err = fedClient.Update(ctx, brTmp)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				igsTmp := &networkingv1.Ingress{}
				err = clusterClient1.Get(ctx, types.NamespacedName{
					Name:      "br-controller-ut-igs1-canary",
					Namespace: "default",
				}, igsTmp)
				return errors.IsNotFound(err)
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				svcTmp := &corev1.Service{}
				err = clusterClient1.Get(ctx, types.NamespacedName{
					Name:      "br-controller-ut-svc1-canary",
					Namespace: "default",
				}, svcTmp)
				return errors.IsNotFound(err)
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp = &v1alpha1.BackendRouting{}
				err = fedClient.Get(ctx, types.NamespacedName{
					Name:      br0.Name,
					Namespace: br0.Namespace,
				}, brTmp)
				if err != nil {
					return false
				}
				return brTmp.Status.Phase == v1alpha1.Ready && brTmp.Generation == brTmp.Status.ObservedGeneration
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		It("forwarding deleted", func() {
			brTmp := &v1alpha1.BackendRouting{}
			err := fedClient.Get(ctx, types.NamespacedName{
				Name:      br0.Name,
				Namespace: br0.Namespace,
			}, brTmp)
			Expect(err).ShouldNot(HaveOccurred())
			brTmp.Spec.Forwarding = nil
			err = fedClient.Update(ctx, brTmp)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(func() bool {
				igsTmp := &networkingv1.Ingress{}
				err = clusterClient1.Get(ctx, types.NamespacedName{
					Name:      "br-controller-ut-igs1",
					Namespace: "default",
				}, igsTmp)
				if err != nil {
					return false
				}
				return igsTmp.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name == "br-controller-ut-svc1"
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				svcTmp := &corev1.Service{}
				err = clusterClient1.Get(ctx, types.NamespacedName{
					Name:      "br-controller-ut-svc1-stable",
					Namespace: "default",
				}, svcTmp)
				return errors.IsNotFound(err)
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() bool {
				brTmp = &v1alpha1.BackendRouting{}
				err = fedClient.Get(ctx, types.NamespacedName{
					Name:      br0.Name,
					Namespace: br0.Namespace,
				}, brTmp)
				if err != nil {
					return false
				}
				return brTmp.Status.Phase == v1alpha1.Ready && brTmp.Generation == brTmp.Status.ObservedGeneration
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})
})

func TestBackendRoutingController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "backend-routing-controller test")
}
