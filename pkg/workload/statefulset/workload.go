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

package statefulset

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

var GVK = appsv1.SchemeGroupVersion.WithKind("StatefulSet")

type workloadImpl struct {
	info workload.Info

	obj *appsv1.StatefulSet

	client client.Client

	visual bool
}

func newFrom(cluster string, client client.Client, sts *appsv1.StatefulSet) *workloadImpl {
	return &workloadImpl{
		info:   workload.NewInfoFrom(cluster, GVK, sts, getStatus(sts)),
		client: client,
		obj:    sts.DeepCopy(),
	}
}

// GetInfo implements workload.Interface.
func (w *workloadImpl) GetInfo() workload.Info {
	return w.info
}

func (w *workloadImpl) UpdateObject(obj client.Object) {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return
	}
	w.obj = sts
}

func (w *workloadImpl) IsWaitingRollout() bool {
	sts := w.obj

	if len(sts.Status.CurrentRevision) != 0 &&
		sts.Status.CurrentRevision != sts.Status.UpdateRevision &&
		sts.Status.UpdatedReplicas == 0 {
		return true
	}

	return false
}

func (w *workloadImpl) UpdateOnConflict(ctx context.Context, modifyFunc func(obj client.Object) error) error {
	obj := w.obj
	result, err := utils.UpdateOnConflict(clusterinfo.WithCluster(ctx, w.info.ClusterName), w.client, w.client, obj, func() error {
		return modifyFunc(obj)
	})
	if err != nil {
		return err
	}
	if result == controllerutil.OperationResultUpdated {
		// update local reference
		w.SetObject(obj)
	}
	return nil
}

func (w *workloadImpl) SetObject(obj client.Object) {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		// ignore
		return
	}
	newInfo := workload.NewInfoFrom(w.info.ClusterName, GVK, sts, getStatus(sts))
	w.info = newInfo
	w.obj = sts
}
