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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"kusionstack.io/rollout/apis/rollout/v1alpha1"
)

var _ = Describe("traffic-topology-controller", func() {

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
					v1alpha1.RouteRef{
						Name: "tp-controller-ut-comp-route0",
					},
				},
			},
		}

		It("no workloads selected", func() {
			err := fedClient.Create(clusterinfo.WithCluster(context.Background(), clusterinfo.Fed), tp0)
			Expect(err).NotTo(HaveOccurred())

			tpTmp := &v1alpha1.TrafficTopology{}
			err = fedClient.Get(clusterinfo.WithCluster(context.Background(), clusterinfo.Fed), types.NamespacedName{
				Name:      tp0.Name,
				Namespace: tp0.Namespace,
			}, tpTmp)
			fmt.Println(tpTmp)

			time.Sleep(10 * time.Second)

			err = fedClient.Get(clusterinfo.WithCluster(context.Background(), clusterinfo.Fed), types.NamespacedName{
				Name:      tp0.Name,
				Namespace: tp0.Namespace,
			}, tpTmp)
			fmt.Println(tpTmp)
		})

	})
})

func TestTrafficTopologyController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "traffic-topology-controller test")
}
