package workloadcontrol

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	kusionstackappsv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	names "kusionstack.io/kube-utils/controller/names"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CollaSeNameMaxLength = 57 // DNS1035LabelMaxLength(63)-randomSuffix(6)=57
)

type Workload struct {
	OwnerName string
	Spec      kusionstackappsv1alpha1.SwarmPodGroupSpec
	Object    *kusionstackappsv1alpha1.CollaSet
	Pods      []*corev1.Pod
}

func NewWorkload(spec kusionstackappsv1alpha1.SwarmPodGroupSpec) *Workload {
	return &Workload{
		Spec: spec,
	}
}

func (w *Workload) GetObjectName() string {
	if w.Object != nil {
		return w.Object.Name
	}
	return names.GenerateDNS1035LabelByMaxLength(w.OwnerName, w.Spec.Name, CollaSeNameMaxLength)
}

func (w *Workload) GenerateTopologyAwere() (*kusionstackappsv1alpha1.SwarmTopologyAwareInjectionGroup, error) {
	if w.Object == nil {
		return nil, fmt.Errorf("collaset is nil")
	}

	podIPs := []string{}
	for _, pod := range w.Pods {
		// skip terminating pod
		if pod.DeletionTimestamp != nil {
			continue
		}
		podIPs = append(podIPs, pod.Status.PodIP)
	}
	sort.Strings(podIPs)

	topologyAware := &kusionstackappsv1alpha1.SwarmTopologyAwareInjectionGroup{
		GroupName:    w.Spec.Name,
		Replicas:     ptr.Deref(w.Spec.Replicas, 0),
		Labels:       w.Spec.Labels,
		Annotations:  w.Spec.Annotations,
		PodAddresses: podIPs,
	}
	return topologyAware, nil
}

type Interface interface {
	// Get(ctx context.Context, namespace, ownerName, groupName string) (*Workload, error)
	ListAll(ctx context.Context, swarm *kusionstackappsv1alpha1.Swarm) (inUse, toDelete map[string]*Workload, err error)
	Delete(ctx context.Context, obj client.Object) (err error)
}

func New(client client.Client) Interface {
	return &RealWorkloadControl{
		Client: client,
	}
}

type RealWorkloadControl struct {
	Client client.Client
}

// func (w *RealWorkloadControl) Get(ctx context.Context, namespace, ownerName, groupName string) (*Workload, error) {
// 	list := &kusionstackappsv1alpha1.CollaSetList{}
// 	err := w.Client.List(ctx, &kusionstackappsv1alpha1.CollaSetList{}, client.InNamespace(namespace), client.MatchingLabels{
// 		kusionstackappsv1alpha1.SwarmNameLabelKey:      ownerName,
// 		kusionstackappsv1alpha1.SwarmGroupNameLabelKey: groupName,
// 	})
// 	if err != nil {
// 		return nil, err
// 	}

// 	filtered := filterCollaSetsForSwarm(list, ownerName)

// 	if len(filtered) == 0 {
// 		return nil, newWorkloadNotFound(kusionstackappsv1alpha1.Resource("collasets"), namespace, ownerName, groupName)
// 	}
// 	if len(filtered) > 1 {
// 		return nil, errors.NewInternalError(fmt.Errorf("found more than one collaset for swarm(%s/%s) group(%s)", namespace, ownerName, groupName))
// 	}

// 	// NOTE: DO NOT get pointer from slice item
// 	// refer to https://github.com/kubernetes/kubernetes/issues/111370
// 	workload := &Workload{
// 		GroupName:  groupName,
// 		ObjectName: filtered[0].Name,
// 		Object:     filtered[0].DeepCopy(),
// 	}
// 	return workload, nil
// }

func (w *RealWorkloadControl) ListAll(ctx context.Context, swarm *kusionstackappsv1alpha1.Swarm) (inUse, toDelete map[string]*Workload, err error) {
	list := &kusionstackappsv1alpha1.CollaSetList{}
	err = w.Client.List(ctx, list,
		client.InNamespace(swarm.Namespace),
		client.MatchingLabels{
			// match swarm name
			kusionstackappsv1alpha1.SwarmNameLabelKey: swarm.Name,
		},
		client.HasLabels{
			// has group name label
			kusionstackappsv1alpha1.SwarmGroupNameLabelKey,
		},
	)
	if err != nil {
		return nil, nil, err
	}
	filtered := filterCollaSetsForSwarm(list, swarm.Name)

	allGroupSpec := lo.SliceToMap(swarm.Spec.PodGroups, func(groupSpec kusionstackappsv1alpha1.SwarmPodGroupSpec) (string, kusionstackappsv1alpha1.SwarmPodGroupSpec) {
		return groupSpec.Name, groupSpec
	})

	inUse, toDelete = map[string]*Workload{}, map[string]*Workload{}

	for i := range filtered {
		item := filtered[i]
		groupName := lo.ValueOr(item.Labels, kusionstackappsv1alpha1.SwarmGroupNameLabelKey, "")
		workload := &Workload{
			OwnerName: swarm.Name,
			Object:    item.DeepCopy(),
		}
		if spec, ok := allGroupSpec[groupName]; ok {
			workload.Spec = spec
			inUse[groupName] = workload
		} else {
			toDelete[groupName] = workload
		}
	}

	// insert pods for remain workloads
	err = w.listAllPods(ctx, swarm, inUse)
	if err != nil {
		return nil, nil, err
	}
	return inUse, toDelete, nil
}

func (w *RealWorkloadControl) listAllPods(ctx context.Context, swarm *kusionstackappsv1alpha1.Swarm, workloads map[string]*Workload) (err error) {
	list := &corev1.PodList{}
	err = w.Client.List(ctx, list,
		client.InNamespace(swarm.Namespace),
		client.MatchingLabels{
			// match swarm name
			kusionstackappsv1alpha1.SwarmNameLabelKey: swarm.Name,
		},
		client.HasLabels{
			// has group name label
			kusionstackappsv1alpha1.SwarmGroupNameLabelKey,
		},
	)
	if err != nil {
		return err
	}

	uidToWorkload := make(map[types.UID]*Workload, len(workloads))
	for _, workload := range workloads {
		uidToWorkload[workload.Object.UID] = workload
	}

	for _, pod := range list.Items {
		controllerRef := metav1.GetControllerOfNoCopy(&pod)
		if controllerRef == nil {
			continue
		}
		w, ok := uidToWorkload[controllerRef.UID]
		if !ok {
			continue
		}
		w.Pods = append(w.Pods, pod.DeepCopy())
	}

	return nil
}

func (w *RealWorkloadControl) Delete(ctx context.Context, obj client.Object) error {
	if obj.GetDeletionTimestamp() != nil {
		return nil
	}
	return client.IgnoreNotFound(w.Client.Delete(ctx, obj))
}

func filterCollaSetsForSwarm(collaSets *kusionstackappsv1alpha1.CollaSetList, swarmName string) []kusionstackappsv1alpha1.CollaSet {
	filtered := lo.Filter(collaSets.Items, func(item kusionstackappsv1alpha1.CollaSet, _ int) bool {
		if len(item.OwnerReferences) == 0 {
			return false
		}
		owner := metav1.GetControllerOfNoCopy(&item)
		if owner == nil {
			return false
		}
		return owner.Kind == "Swarm" && owner.Name == swarmName
	})
	return filtered
}

// func newWorkloadNotFound(qualifiedResource schema.GroupResource, namespace, ownerName, groupName string) *errors.StatusError {
// 	return &errors.StatusError{ErrStatus: metav1.Status{
// 		Status: metav1.StatusFailure,
// 		Code:   http.StatusNotFound,
// 		Reason: metav1.StatusReasonNotFound,
// 		Details: &metav1.StatusDetails{
// 			Group: qualifiedResource.Group,
// 			Kind:  qualifiedResource.Resource,
// 			Name:  groupName,
// 		},
// 		Message: fmt.Sprintf("failed to find %s for swarm(%s/%s) group(%s) not found", qualifiedResource.String(), namespace, ownerName, groupName),
// 	}}
// }
