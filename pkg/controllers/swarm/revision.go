package swarm

import (
	"bytes"
	"context"

	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	kusionstackappsv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"kusionstack.io/kube-utils/controller/history"
	"kusionstack.io/kube-utils/extractor"

	"kusionstack.io/rollout/pkg/controllers/swarm/workloadcontrol"
)

func NewRevisionOwner(clsControl workloadcontrol.Interface) history.RevisionOwner {
	return &revisionOwnerSwarm{
		clsControl: clsControl,
	}
}

var _ history.RevisionOwner = &revisionOwnerSwarm{}

type revisionOwnerSwarm struct {
	clsControl workloadcontrol.Interface
}

func (r *revisionOwnerSwarm) GetGroupVersionKind() schema.GroupVersionKind {
	return kusionstackappsv1alpha1.SchemeGroupVersion.WithKind("Swarm")
}

// GetCollisionCount implements revision.OwnerAdapter.
func (r *revisionOwnerSwarm) GetCollisionCount(obj metav1.Object) *int32 {
	swarm := obj.(*kusionstackappsv1alpha1.Swarm)
	return swarm.Status.CollisionCount
}

// GetCurrentRevision implements revision.OwnerAdapter.
func (r *revisionOwnerSwarm) GetCurrentRevision(obj metav1.Object) string {
	swarm := obj.(*kusionstackappsv1alpha1.Swarm)
	return swarm.Status.CurrentRevision
}

// GetHistoryLimit implements revision.OwnerAdapter.
func (r *revisionOwnerSwarm) GetHistoryLimit(obj metav1.Object) int32 {
	swarm := obj.(*kusionstackappsv1alpha1.Swarm)
	return swarm.Spec.HistoryLimit
}

// GetPatch implements revision.OwnerAdapter.
func (r *revisionOwnerSwarm) GetPatch(obj metav1.Object) ([]byte, error) {
	swarm := obj.(*kusionstackappsv1alpha1.Swarm)
	return getSwarmPatch(swarm)
}

// GetSelector implements revision.OwnerAdapter.
func (r *revisionOwnerSwarm) GetMatchLabels(obj metav1.Object) map[string]string {
	swarm := obj.(*kusionstackappsv1alpha1.Swarm)
	return map[string]string{
		kusionstackappsv1alpha1.SwarmNameLabelKey: swarm.Name,
	}
}

func (r *revisionOwnerSwarm) GetInUsedRevisions(obj metav1.Object) (sets.String, error) {
	swarm := obj.(*kusionstackappsv1alpha1.Swarm)
	inUsedWorkloads, _, err := r.clsControl.ListAll(context.Background(), swarm)
	if err != nil {
		return nil, err
	}
	inUsed := sets.NewString()
	if len(swarm.Status.CurrentRevision) > 0 {
		inUsed.Insert(swarm.Status.CurrentRevision)
	}
	if len(swarm.Status.UpdatedRevision) > 0 {
		inUsed.Insert(swarm.Status.UpdatedRevision)
	}
	for _, cls := range inUsedWorkloads {
		inUseHash := lo.ValueOr(cls.Object.GetLabels(), history.ControllerRevisionHashLabel, "")
		if len(inUseHash) > 0 {
			inUsed.Insert(inUseHash)
		}
	}
	return inUsed, nil
}

var swarmPatchExtractor = extractor.NewOrDie([]string{
	".spec.podGroups[*]['name','labels','annotations','template','volumeClaimTemplates']",
}, extractor.IgnoreMissingKey(true))

func getSwarmPatch(obj *kusionstackappsv1alpha1.Swarm) ([]byte, error) {
	obj.APIVersion = kusionstackappsv1alpha1.SchemeGroupVersion.String()
	obj.Kind = "Swarm"

	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	// prune spec
	prunedObj, err := swarmPatchExtractor.Extract(raw)
	if err != nil {
		return nil, err
	}

	objCopy := &unstructured.Unstructured{Object: prunedObj}
	buffer := bytes.NewBuffer(nil)
	err = unstructured.UnstructuredJSONScheme.Encode(objCopy, buffer)
	if err != nil {
		return nil, err
	}
	data := buffer.Bytes()
	// encode will add newline at the end of the buffer, we need to trim it
	data = bytes.TrimSpace(data)
	return data, nil
}
