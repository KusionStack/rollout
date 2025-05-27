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

package backendrouting

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &EnqueueBR{}

type EnqueueBR struct{}

func (e *EnqueueBR) Create(createEvent event.CreateEvent, q workqueue.RateLimitingInterface) {
	if createEvent.Object == nil {
		return
	}
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      createEvent.Object.GetName(),
		Namespace: createEvent.Object.GetNamespace(),
	}})
}

func (e *EnqueueBR) Update(updateEvent event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if updateEvent.ObjectOld != nil {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      updateEvent.ObjectOld.GetName(),
			Namespace: updateEvent.ObjectOld.GetNamespace(),
		}})
	}

	if updateEvent.ObjectNew != nil {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      updateEvent.ObjectNew.GetName(),
			Namespace: updateEvent.ObjectNew.GetNamespace(),
		}})
	}
}

func (e *EnqueueBR) Delete(deleteEvent event.DeleteEvent, q workqueue.RateLimitingInterface) {
	if deleteEvent.Object == nil {
		return
	}

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      deleteEvent.Object.GetName(),
		Namespace: deleteEvent.Object.GetNamespace(),
	}})
}

func (e *EnqueueBR) Generic(genericEvent event.GenericEvent, q workqueue.RateLimitingInterface) {
	if genericEvent.Object == nil {
		return
	}

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      genericEvent.Object.GetName(),
		Namespace: genericEvent.Object.GetNamespace(),
	}})
}
