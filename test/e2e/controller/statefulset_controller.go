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

package controller

import (
	"context"
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/pkg/controller/history"
	"k8s.io/utils/ptr"
	"kusionstack.io/kube-utils/controller/mixin"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kusionstack.io/rollout/pkg/utils"
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = appsv1.SchemeGroupVersion.WithKind("StatefulSet")

var (
	FakeStsControllerName = "fake-sts-controller"

	patchCodec = scheme.Codecs.LegacyCodec(appsv1.SchemeGroupVersion)
)

func InitFakeStsControllerFunc(mgr manager.Manager) (bool, error) {
	err := NewFakeStatefulSetController(mgr).SetupWithManager(mgr)
	if err != nil {
		return false, err
	}
	return true, nil
}

func NewFakeStatefulSetController(mgr manager.Manager) *FakeStatefulSetController {
	return &FakeStatefulSetController{
		ReconcilerMixin: mixin.NewReconcilerMixin(FakeStsControllerName, mgr),
	}
}

type FakeStatefulSetController struct {
	*mixin.ReconcilerMixin
}

// SetupWithManager sets up the controller with the Manager.
func (r *FakeStatefulSetController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 5}).
		For(&appsv1.StatefulSet{}).
		Complete(r)
}

func (r *FakeStatefulSetController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := &appsv1.StatefulSet{}
	err := r.Client.Get(ctx, req.NamespacedName, obj)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	logger := r.Logger.WithValues("sts", obj.Name)
	// sync status
	patch, err := getPatch(obj)
	if err != nil {
		logger.Error(err, "failed to get patch of statefulset")
		return ctrl.Result{}, err
	}

	revision, err := history.NewControllerRevision(obj, controllerKind, obj.Spec.Template.Labels, runtime.RawExtension{Raw: patch}, 0, nil)
	if err != nil {
		logger.Error(err, "failed to create revision for statefulset")
		return ctrl.Result{}, err
	}

	newStatus := obj.Status.DeepCopy()

	newStatus.UpdateRevision = revision.Name
	if len(newStatus.CurrentRevision) == 0 {
		newStatus.CurrentRevision = newStatus.UpdateRevision
	}

	newStatus.ObservedGeneration = obj.Generation
	newStatus.Replicas = *obj.Spec.Replicas
	newStatus.ReadyReplicas = newStatus.Replicas
	newStatus.AvailableReplicas = newStatus.Replicas

	if newStatus.CurrentRevision != newStatus.UpdateRevision {
		if obj.Spec.UpdateStrategy.RollingUpdate != nil {
			partition := ptr.Deref(obj.Spec.UpdateStrategy.RollingUpdate.Partition, newStatus.Replicas)
			newStatus.UpdatedReplicas = newStatus.Replicas - partition
		} else {
			newStatus.UpdatedReplicas = newStatus.Replicas
		}
		// newStatus.CurrentReplicas = newStatus.Replicas - newStatus.UpdatedReplicas
	}

	if newStatus.UpdatedReplicas == newStatus.Replicas {
		// all replicas is updated
		newStatus.CurrentReplicas = newStatus.Replicas
		newStatus.CurrentRevision = newStatus.UpdateRevision
	} else {
		newStatus.CurrentReplicas = newStatus.Replicas - newStatus.UpdatedReplicas
	}

	logger.Info("update statefulset status", "status", newStatus)
	_, err = utils.UpdateOnConflict(ctx, r.Client, r.Client.Status(), obj, func() error {
		obj.Status = *newStatus
		return nil
	})
	if err != nil {
		logger.Error(err, "failed to update statefulset status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// getPatch returns a strategic merge patch that can be applied to restore a StatefulSet to a
// previous version. If the returned error is nil the patch is valid. The current state that we save is just the
// PodSpecTemplate. We can modify this later to encompass more state (or less) and remain compatible with previously
// recorded patches.
func getPatch(set *appsv1.StatefulSet) ([]byte, error) {
	data, err := runtime.Encode(patchCodec, set)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	if err != nil {
		return nil, err
	}
	objCopy := make(map[string]interface{})
	specCopy := make(map[string]interface{})
	spec := raw["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	specCopy["template"] = template
	template["$patch"] = "replace"
	objCopy["spec"] = specCopy
	patch, err := json.Marshal(objCopy)
	return patch, err
}
