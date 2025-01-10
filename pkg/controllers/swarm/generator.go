package swarm

import (
	"encoding/json"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kusionstackappsv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"kusionstack.io/kube-utils/controller/history"

	"kusionstack.io/rollout/pkg/controllers/swarm/workloadcontrol"
)

func generateCollaSet(swarm *kusionstackappsv1alpha1.Swarm, g graph.Graph[string, *workloadcontrol.Workload], workload *workloadcontrol.Workload) (*kusionstackappsv1alpha1.CollaSet, error) {
	objectName := workload.GetObjectName()
	owner := metav1.NewControllerRef(swarm, kusionstackappsv1alpha1.SchemeGroupVersion.WithKind("Swarm"))
	defaultLabels := labels.Set{
		kusionstackappsv1alpha1.SwarmNameLabelKey:      swarm.Name,
		kusionstackappsv1alpha1.SwarmGroupNameLabelKey: workload.Spec.Name,
	}

	// insert default labels into workload labels and template labels
	labels := lo.Assign(workload.Spec.Labels, defaultLabels)

	template := workload.Spec.Template.DeepCopy()
	tm := NewTemplateModifier(template)
	tm.MergeLabels(defaultLabels)

	if workload.Spec.TopologyAware != nil &&
		workload.Spec.TopologyAware.TopologyInjection != nil &&
		workload.Spec.TopologyAware.TopologyInjection.PodIPs {
		// inject pod ips to env

		injection := &kusionstackappsv1alpha1.SwarmTopologyAwareInjection{}
		for _, dependency := range workload.Spec.TopologyAware.DependsOn {
			dv, _ := g.Vertex(dependency)
			topo, _ := dv.GenerateTopologyAwere()
			injection.Groups = append(injection.Groups, *topo)
		}
		injectionValue, _ := json.Marshal(injection)
		tm.AddEnvs(corev1.EnvVar{
			Name:  kusionstackappsv1alpha1.SwarmTopologyAwareInjectionEnvKey,
			Value: string(injectionValue),
		})
	}

	result := &kusionstackappsv1alpha1.CollaSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kusionstackappsv1alpha1.GroupVersion.String(),
			Kind:       "CollaSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName,
			Namespace: swarm.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*owner,
			},
			Annotations: workload.Spec.Annotations,
			Labels:      labels,
		},
		Spec: kusionstackappsv1alpha1.CollaSetSpec{
			Replicas:             workload.Spec.Replicas,
			Selector:             metav1.SetAsLabelSelector(defaultLabels),
			Template:             *template,
			VolumeClaimTemplates: workload.Spec.VolumeClaimTemplates,
		},
	}

	revisionPatch, err := getPatchForSwarmPodGroupSpec(result)
	if err != nil {
		return nil, err
	}

	revisionHash := history.HashControllerRevisionRawData(revisionPatch, nil)
	result.Labels = lo.Assign(result.Labels, map[string]string{
		history.ControllerRevisionHashLabel: revisionHash,
	})

	return result, nil
}
