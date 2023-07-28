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

package rollout

import (
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"

	rolloutv1alpha1 "github.com/KusionStack/rollout/api/v1alpha1"
	"github.com/KusionStack/rollout/pkg/workload"
	"github.com/KusionStack/rollout/test/pkg/builder"
)

func TestConstructWorkflow(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	wrappers := []workload.Interface{
		// TODO: add workload wrappers
	}
	instance := rolloutv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rollout-1",
			Namespace: "default",
		},
	}
	rolloutStrategy := builder.NewRolloutStrategy().Build()
	workflow, err := constructWorkflow(&instance, rolloutStrategy.Spec.Batch, wrappers, "")
	g.Expect(err).To(gomega.BeNil())
	g.Expect(len(workflow.Spec.Tasks) > 0).To(gomega.BeTrue())

	bytes, _ := json.Marshal(workflow)
	fmt.Println(string(bytes))
}
