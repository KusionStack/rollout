package progressinginfos

import (
	"cmp"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/samber/lo"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/utils"
)

var _ sort.Interface = ProgressingInfos{}

type ProgressingInfos []rolloutv1alpha1.ProgressingInfo

// Len implements sort.Interface.
func (p ProgressingInfos) Len() int {
	return len(p)
}

// Less implements sort.Interface.
func (p ProgressingInfos) Less(i, j int) bool {
	return cmp.Or(
		p[i].Kind < p[j].Kind,
		p[i].Kind == p[j].Kind && p[i].RolloutName < p[j].RolloutName,
	)
}

// Swap implements sort.Interface.
func (p ProgressingInfos) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

type ProgressingInfoMutator struct {
	ProgressingInfosAnnotationKey string
}

func (m *ProgressingInfoMutator) MutatePogressingInfo(obj runtimeclient.Object, owners []*registry.WorkloadAccessor) bool {
	// get progressingInfos from owners
	controlInfo, newInfos := generateProgressingInfos(owners)
	if len(newInfos) == 0 {
		return false
	}
	// get progressingInfos from obj
	existingInfos := m.GetProgressingInfos(obj)
	// merge progressing info
	merged := mergeProgressingInfos(existingInfos, newInfos)

	changed := m.SetProgressingInfos(obj, merged)

	// for compatibility, we also need to set rollout.kusionstack.io/progressing-info
	changed = m.SetProgressingInfo(obj, controlInfo) || changed

	return changed
}

func (m *ProgressingInfoMutator) GetProgressingInfos(obj runtimeclient.Object) ProgressingInfos {
	objInfo := utils.GetMapValueByDefault(obj.GetAnnotations(), m.ProgressingInfosAnnotationKey, "")
	if len(objInfo) == 0 {
		return nil
	}
	info := ProgressingInfos{}
	err := json.Unmarshal([]byte(objInfo), &info)
	if err != nil {
		// ignore invalid json
		return nil
	}
	return info
}

func (m *ProgressingInfoMutator) SetProgressingInfos(obj runtimeclient.Object, infos ProgressingInfos) bool {
	var expected string
	// set expectedObjInfoStr to "" if merged is empty
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
	utils.MutateAnnotations(obj, func(annotations map[string]string) {
		existing, ok := annotations[m.ProgressingInfosAnnotationKey]
		if len(expected) == 0 && ok {
			// delete if no progressingInfo
			delete(annotations, m.ProgressingInfosAnnotationKey)
			changed = true
		} else if existing != expected {
			// set infos if changed
			annotations[m.ProgressingInfosAnnotationKey] = expected
			changed = true
		}
	})

	return changed
}

func (m *ProgressingInfoMutator) SetProgressingInfo(obj runtimeclient.Object, info *rolloutv1alpha1.ProgressingInfo) bool {
	if info == nil {
		return false
	}

	// get info from obj annotation
	var existing *rolloutv1alpha1.ProgressingInfo
	objInfo := utils.GetMapValueByDefault(obj.GetAnnotations(), rolloutapi.AnnoRolloutProgressingInfo, "")
	if len(objInfo) > 0 {
		temp := rolloutv1alpha1.ProgressingInfo{}
		err := json.Unmarshal([]byte(objInfo), &temp)
		if err == nil {
			existing = &temp
		}
	}

	changed := false
	expected, _ := json.Marshal(info)
	if existing == nil || existing.RolloutID != info.RolloutID {
		// if no progressing info found on obj or rolloutID changed, we need to update annotation
		changed = true
		// set progressingInfo if no progressingInfo
		utils.MutateAnnotations(obj, func(annotations map[string]string) {
			annotations[rolloutapi.AnnoRolloutProgressingInfo] = string(expected)
		})
	}
	return changed
}

func mergeProgressingInfos(existings, news ProgressingInfos) ProgressingInfos {
	return mergeSliceByKey(
		existings,
		news,
		func(info rolloutv1alpha1.ProgressingInfo) string {
			return fmt.Sprintf("%s/%s", info.Kind, info.RolloutName)
		},
		func(inObj, inOwner rolloutv1alpha1.ProgressingInfo) rolloutv1alpha1.ProgressingInfo {
			if inObj.RolloutID == inOwner.RolloutID {
				// we should not change progressingInfo if rolloutID is not changed
				return inObj
			}
			return inOwner
		},
	)
}

func generateProgressingInfos(owners []*registry.WorkloadAccessor) (*rolloutv1alpha1.ProgressingInfo, ProgressingInfos) {
	var controllerProgressingInfo *rolloutv1alpha1.ProgressingInfo
	result := ProgressingInfos{}

	for _, owner := range owners {
		ownerInfo := utils.GetMapValueByDefault(owner.Object.GetAnnotations(), rolloutapi.AnnoRolloutProgressingInfo, "")
		if len(ownerInfo) == 0 {
			continue
		}
		info := rolloutv1alpha1.ProgressingInfo{}
		err := json.Unmarshal([]byte(ownerInfo), &info)
		if err != nil {
			continue
		}
		result = append(result, info)
		if owner.IsController {
			controllerProgressingInfo = &info
		}
	}
	return controllerProgressingInfo, result
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
