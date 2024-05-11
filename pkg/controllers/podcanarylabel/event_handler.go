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

package podcanarylabel

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kusionstack.io/rollout/pkg/workload"
)

var _ predicate.Predicate = workloadLabelOrStatusChangedPredict{}

type workloadLabelOrStatusChangedPredict struct {
	accessor workload.Accessor
}

// Create implements predicate.Predicate.
func (w workloadLabelOrStatusChangedPredict) Create(event.CreateEvent) bool {
	return false
}

// Delete implements predicate.Predicate.
func (w workloadLabelOrStatusChangedPredict) Delete(event.DeleteEvent) bool {
	return false
}

// Generic implements predicate.Predicate.
func (w workloadLabelOrStatusChangedPredict) Generic(event.GenericEvent) bool {
	return false
}

// Update implements predicate.Predicate.
func (w workloadLabelOrStatusChangedPredict) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil {
		return false
	}
	if e.ObjectNew == nil {
		return false
	}
	oldInControl := workload.IsControlledByRollout(e.ObjectOld)
	newInControl := workload.IsControlledByRollout(e.ObjectNew)

	oldProgressing := workload.IsProgressing(e.ObjectOld)
	newProgressing := workload.IsProgressing(e.ObjectNew)

	// workload control changed or progressing info changed
	return oldInControl != newInControl || oldProgressing != newProgressing
}

func enqueueWorkloadPods(accessor workload.Accessor, reader client.Reader, scheme *runtime.Scheme, logger logr.Logger) handler.EventHandler {
	handlerWorkloadUpdate := func(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
		// check gvk of object
		obj := evt.ObjectNew

		key := types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}
		kinds, _, err := scheme.ObjectKinds(obj)
		if err != nil {
			logger.Error(err, "failed to get object kind")
			return
		}
		gvk := kinds[0]

		if accessor.GroupVersionKind() != gvk {
			// missmatch group version kind
			return
		}

		pc := accessor.PodControl(reader)
		selector, err := pc.GetPodSelector(obj)
		if err != nil {
			logger.Error(err, "failed to get PodSelector for workload", "key", key.String(), "gvk", gvk.String())
			return
		}

		podList := &corev1.PodList{}
		err = reader.List(context.Background(), podList, client.InNamespace(obj.GetNamespace()), client.MatchingLabelsSelector{Selector: selector})
		if err != nil {
			logger.Error(err, "failed to list pods for workload", "key", key.String(), "gvk", gvk.String())
			return
		}

		for i := range podList.Items {
			pod := &podList.Items[i]
			q.Add(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(pod)})
		}
	}

	return handler.Funcs{
		UpdateFunc: handlerWorkloadUpdate,
	}
}
