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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kusionstack.io/kube-utils/multicluster/clusterinfo"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
)

var _ = Describe("backend-routing-controller", func() {
	Context("InCluster BackendRouting", func() {
		It("xxx", func() {
			br0 := v1alpha1.BackendRouting{
				ObjectMeta: v1.ObjectMeta{
					Name:      "br-controller-ut-br0",
					Namespace: "default",
				},
			}
			err := fedClient.Create(clusterinfo.WithCluster(ctx, clusterinfo.Fed), &br0)
			Expect(err).Should(BeNil())
			time.Sleep(3 * time.Second)
		})
	})
})

func TestBackendRoutingController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "backend-routing-controller test")
}
