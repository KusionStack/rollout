package swarm

import (
	"encoding/json"
	"fmt"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kusionstackappsv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"kusionstack.io/kube-utils/controller/history"

	"kusionstack.io/rollout/pkg/controllers/swarm/workloadcontrol"
)

const (
	InjectionHashLabel = "swarm.apps.kusionstack.io/topology-injection-hash"
)

func generateCollaSetFromRevision(swarm *kusionstackappsv1alpha1.Swarm, workload *workloadcontrol.Workload, revision *appsv1.ControllerRevision) (*kusionstackappsv1alpha1.CollaSet, error) {
	groupName := workload.Spec.Name
	objectName := workload.GetObjectName()
	owner := metav1.NewControllerRef(swarm, kusionstackappsv1alpha1.SchemeGroupVersion.WithKind("Swarm"))

	spec, err := getPodGroupSpecFromRevision(revision, groupName)
	if err != nil {
		return nil, err
	}

	uniqueLabels := labels.Set{
		kusionstackappsv1alpha1.SwarmNameLabelKey:      swarm.Name,
		kusionstackappsv1alpha1.SwarmGroupNameLabelKey: groupName,
	}

	// insert unique labels into workload labels
	labels := lo.Assign(spec.Labels, uniqueLabels)
	labels = lo.Assign(labels, map[string]string{
		history.ControllerRevisionHashLabel: revision.Name,
	})

	// insert unique labels into template
	template := spec.Template.DeepCopy()
	tm := NewPodTemplateModifier(template)
	tm.AddLebls(uniqueLabels)

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
			Annotations: spec.Annotations,
			Labels:      labels,
		},
		Spec: kusionstackappsv1alpha1.CollaSetSpec{
			Replicas:             workload.Spec.Replicas,
			Selector:             metav1.SetAsLabelSelector(uniqueLabels),
			Template:             *template,
			VolumeClaimTemplates: spec.VolumeClaimTemplates,
		},
	}

	return result, nil
}

func getPodGroupSpecFromRevision(revision *appsv1.ControllerRevision, groupName string) (*kusionstackappsv1alpha1.SwarmPodGroupSpec, error) {
	swarm, err := getSwarmFromRevision(revision)
	if err != nil {
		return nil, err
	}

	spec, ok := lo.Find(swarm.Spec.PodGroups, func(item kusionstackappsv1alpha1.SwarmPodGroupSpec) bool {
		return item.Name == groupName
	})
	if !ok {
		return nil, fmt.Errorf("%v pod group spec not found in swarm", groupName)
	}
	return &spec, nil
}

func getSwarmFromRevision(revision *appsv1.ControllerRevision) (*kusionstackappsv1alpha1.Swarm, error) {
	swarm := &kusionstackappsv1alpha1.Swarm{}
	err := json.Unmarshal(revision.Data.Raw, swarm)
	if err != nil {
		return nil, err
	}
	return swarm, nil
}

func injectTopology(cls *kusionstackappsv1alpha1.CollaSet, g graph.Graph[string, *workloadcontrol.Workload], workload *workloadcontrol.Workload) {
	tm := NewPodTemplateModifier(&cls.Spec.Template)
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
		hash := history.HashControllerRevisionRawData(injectionValue, nil)
		cls.Labels = lo.Assign(cls.Labels, map[string]string{
			InjectionHashLabel: hash,
		})
	}
}
