package pod

import (
	"cmp"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"

	"kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/utils"
)

var _ sort.Interface = PodProgressingInfos{}

type PodProgressingInfos []rolloutv1alpha1.ProgressingInfo

// Len implements sort.Interface.
func (p PodProgressingInfos) Len() int {
	return len(p)
}

// Less implements sort.Interface.
func (p PodProgressingInfos) Less(i int, j int) bool {
	return cmp.Or(
		p[i].Kind < p[j].Kind,
		p[i].Kind == p[j].Kind && p[i].RolloutName < p[j].RolloutName,
	)
}

// Swap implements sort.Interface.
func (p PodProgressingInfos) Swap(i int, j int) {
	p[i], p[j] = p[j], p[i]
}

func MergePodProgressingInfos(existings, news PodProgressingInfos) PodProgressingInfos {
	return mergeSliceByKey(
		existings,
		news,
		func(info rolloutv1alpha1.ProgressingInfo) string {
			return fmt.Sprintf("%s/%s", info.Kind, info.RolloutName)
		},
		func(inPod, inOwner rolloutv1alpha1.ProgressingInfo) rolloutv1alpha1.ProgressingInfo {
			if inPod.RolloutID == inOwner.RolloutID {
				// we should not change progressingInfo if rolloutID is not changed
				return inPod
			}
			return inOwner
		},
	)
}

func GetPodProgressingInfos(pod *corev1.Pod) PodProgressingInfos {
	podInfo := utils.GetMapValueByDefault(pod.Annotations, rollout.AnnoPodRolloutProgressingInfos, "")
	if len(podInfo) == 0 {
		return nil
	}
	info := PodProgressingInfos{}
	err := json.Unmarshal([]byte(podInfo), &info)
	if err != nil {
		// ignore invalid json
		return nil
	}
	return info
}

func SetProgressingInfos(pod *corev1.Pod, infos PodProgressingInfos) bool {
	var expected string
	// set expectedPodInfoStr to "" if merged is empty
	if len(infos) > 0 {
		// sort infos before marshaling
		sort.Sort(infos)

		data, err := json.Marshal(infos)
		if err != nil {
			return false
		}
		expected = string(data)
	}

	var changed bool
	utils.MutateAnnotations(pod, func(annotations map[string]string) {
		existing, ok := annotations[rollout.AnnoPodRolloutProgressingInfos]
		if len(expected) == 0 && ok {
			// delete if no progressingInfo
			delete(annotations, rollout.AnnoPodRolloutProgressingInfos)
			changed = true
		} else if existing != expected {
			// set infos if changed
			annotations[rollout.AnnoPodRolloutProgressingInfos] = expected
			changed = true
		}
	})

	return changed
}

func mutatePodPogressingInfo(pod *corev1.Pod, owners []*registry.WorkloadAccessor) bool {
	// get progressingInfos from owners
	newInfos := generateProgressingInfos(owners)
	if len(newInfos) == 0 {
		return false
	}
	// get progressingInfos from pod
	existingInfos := GetPodProgressingInfos(pod)
	// merge progressing info
	merged := MergePodProgressingInfos(existingInfos, newInfos)

	return SetProgressingInfos(pod, merged)
}

func generateProgressingInfos(owners []*registry.WorkloadAccessor) PodProgressingInfos {
	result := PodProgressingInfos{}

	for _, owner := range owners {
		ownerInfo := utils.GetMapValueByDefault(owner.Object.GetAnnotations(), rollout.AnnoRolloutProgressingInfo, "")
		if len(ownerInfo) == 0 {
			continue
		}
		info := rolloutv1alpha1.ProgressingInfo{}
		err := json.Unmarshal([]byte(ownerInfo), &info)
		if err != nil {
			continue
		}
		result = append(result, info)
	}
	return result
}

func mergeSliceByKey[T any, Slice ~[]T, K comparable](a, b Slice, keyFunc func(item T) K, whenConflict func(aItem, bItem T) T) Slice {
	// fast path
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	merge := map[K]T{}

	for i := range a {
		key := keyFunc(a[i])
		merge[key] = a[i]
	}
	for i := range b {
		key := keyFunc(b[i])
		if aItem, ok := merge[key]; ok {
			// resolve conflict
			merge[key] = whenConflict(aItem, b[i])
		} else {
			merge[key] = b[i]
		}
	}

	result := Slice(lo.Values(merge))

	return result
}
