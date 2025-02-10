package swarm

import (
	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	kusionstackappsv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/swarm/workloadcontrol"
)

func treeWayMergeMap(original, modified, current map[string]string) map[string]string {
	result := make(map[string]string)
	originalKeys := sets.StringKeySet(original)
	modifiedKeys := sets.StringKeySet(modified)

	for k, v := range modified {
		result[k] = v
	}

	for k, v := range current {
		if modifiedKeys.Has(k) {
			continue
		}
		if originalKeys.Has(k) {
			// this key exists in original but not in  modified, delete it
			continue
		}
		result[k] = v
	}
	return result
}

func isCollaSetReady(cls *kusionstackappsv1alpha1.CollaSet) bool {
	if cls == nil {
		return false
	}
	return cls.DeletionTimestamp == nil &&
		cls.Generation == cls.Status.ObservedGeneration &&
		ptr.Deref(cls.Spec.Replicas, 0) == cls.Status.UpdatedAvailableReplicas
}

func parseGraphFrom(swarm *kusionstackappsv1alpha1.Swarm, workloads map[string]*workloadcontrol.Workload) (graph.Graph[string, *workloadcontrol.Workload], error) {
	allGroupSpec := lo.SliceToMap(swarm.Spec.PodGroups, func(groupSpec kusionstackappsv1alpha1.SwarmPodGroupSpec) (string, kusionstackappsv1alpha1.SwarmPodGroupSpec) {
		return groupSpec.Name, groupSpec
	})

	g := graph.New(workloadNodeHash, graph.Directed(), graph.PreventCycles())

	for groupName := range allGroupSpec {
		spec := allGroupSpec[groupName]

		workload, ok := workloads[groupName]
		if !ok {
			workload = &workloadcontrol.Workload{
				OwnerName: swarm.Name,
				Spec:      spec,
			}
		}
		g.AddVertex(workload) // nolint:errcheck
	}

	for _, group := range allGroupSpec {
		if group.TopologyAware == nil {
			continue
		}
		for _, from := range group.TopologyAware.DependsOn {
			if _, ok := allGroupSpec[from]; !ok {
				continue
			}
			err := g.AddEdge(from, group.Name)
			if err != nil {
				// maybe there is a cycle
				return nil, err
			}
		}
	}

	return g, nil
}

func workloadNodeHash(w *workloadcontrol.Workload) string {
	return w.Spec.Name
}

func getPodGroupStatusFromGraph(queue []string, g graph.Graph[string, *workloadcontrol.Workload]) []kusionstackappsv1alpha1.SwarmPodGroupStatus {
	result := make([]kusionstackappsv1alpha1.SwarmPodGroupStatus, 0)
	for _, key := range queue {
		w, _ := g.Vertex(key)
		if w.Object == nil {
			continue
		}

		result = append(result, kusionstackappsv1alpha1.SwarmPodGroupStatus{
			Name:              w.Spec.Name,
			Replicas:          ptr.Deref(w.Spec.Replicas, 0),
			UpdatedReplicas:   w.Object.Status.UpdatedReplicas,
			ReadyReplicas:     w.Object.Status.ReadyReplicas,
			AvailableReplicas: w.Object.Status.AvailableReplicas,
		})
	}
	return result
}
