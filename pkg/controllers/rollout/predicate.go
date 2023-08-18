/*
 * Copyright 2023 The KusionStack Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rollout

import (
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/KusionStack/rollout/api"
	rolloutv1alpha1 "github.com/KusionStack/rollout/api/v1alpha1"
)

// NamespacePredicate returns a predicate.Funcs that returns true if the object
// is in a list of namespaces.
func NamespacePredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return checkNamespace(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return checkNamespace(event.ObjectOld) || checkNamespace(event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return checkNamespace(event.Object)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return checkNamespace(event.Object)
		},
	}
}

func checkNamespace(object interface{}) bool {
	// TODO: dynamic load from configmap
	namespaces := sets.NewString("default", "onlinetestd", "metaslo")
	if object == nil {
		return false
	}
	if objectMeta, ok := object.(metav1.Object); ok {
		return objectMeta.GetNamespace() == "" || namespaces.Has(objectMeta.GetNamespace())
	}
	return true
}

func WorkflowPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return checkWorkflowLabel(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return checkWorkflowLabel(event.ObjectOld) || checkWorkflowLabel(event.ObjectNew)
		},

		GenericFunc: func(event event.GenericEvent) bool {
			return checkWorkflowLabel(event.Object)
		},

		DeleteFunc: func(event event.DeleteEvent) bool {
			return checkWorkflowLabel(event.Object)
		},
	}
}

func checkWorkflowLabel(object metav1.Object) bool {
	if object.GetLabels() == nil {
		return false
	}
	if _, ok := object.GetLabels()[api.LabelRolloutControl]; !ok {
		return false
	}
	return true
}

func RolloutPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return checkRolloutUpdate(event)
		},

		GenericFunc: func(event event.GenericEvent) bool {
			return true
		},

		DeleteFunc: func(event event.DeleteEvent) bool {
			return true
		},
	}
}

func checkRolloutUpdate(event event.UpdateEvent) bool {
	if event.ObjectOld == nil || event.ObjectNew == nil {
		return false
	}

	oldRollout, ok := event.ObjectOld.(*rolloutv1alpha1.Rollout)
	if !ok {
		return false
	}
	newRollout, ok := event.ObjectNew.(*rolloutv1alpha1.Rollout)
	if !ok {
		return false
	}

	rv := oldRollout.ResourceVersion
	defer func() {
		oldRollout.ResourceVersion = rv
	}()

	oldRollout.ResourceVersion = newRollout.ResourceVersion
	return !equality.Semantic.DeepEqual(oldRollout.ObjectMeta, newRollout.ObjectMeta) || !equality.Semantic.DeepEqual(oldRollout.Spec, newRollout.Spec)
}
