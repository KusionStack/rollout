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
