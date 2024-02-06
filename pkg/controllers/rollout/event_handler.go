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

package rollout

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

func enqueueRolloutForWorkloadHandler(reader client.Reader, scheme *runtime.Scheme, logger logr.Logger) handler.EventHandler {
	mapFunc := func(obj client.Object) []reconcile.Request {
		key := types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}
		kinds, _, err := scheme.ObjectKinds(obj)
		if err != nil {
			logger.Error(err, "failed to get ObjectKind from object", "key", key.String())
			return nil
		}
		gvk := kinds[0]

		cluster := workload.GetClusterFromLabel(obj.GetLabels())
		info := workload.NewInfoFrom(cluster, gvk, obj, workload.Status{})
		rollout, err := getRolloutForWorkload(reader, logger, info)
		if err != nil {
			logger.Error(err, "failed to get rollout for workload", "key", key.String(), "gvk", gvk.String())
			return nil
		}
		if rollout == nil {
			// TODO: if workload has rollout label but no rollout matches it, we need to clean the label
			// no matched
			// logger.V(5).Info("no matched rollout found for workload", "workload", key.String(), "gvk", gvk.String())
			return nil
		}

		logger.V(1).Info("get matched rollout for workload", "workload", key.String(), "rollout", rollout.Name)
		req := types.NamespacedName{Namespace: rollout.GetNamespace(), Name: rollout.GetName()}
		return []reconcile.Request{{NamespacedName: req}}
	}

	return handler.EnqueueRequestsFromMapFunc(mapFunc)
}

func getRolloutForWorkload(
	reader client.Reader,
	logger logr.Logger,
	workloadInfo workload.Info,
) (*rolloutv1alpha1.Rollout, error) {
	rList := &rolloutv1alpha1.RolloutList{}
	ctx := clusterinfo.WithCluster(context.TODO(), clusterinfo.Fed)
	if err := reader.List(ctx, rList, client.InNamespace(workloadInfo.Namespace)); err != nil {
		logger.Error(err, "failed to list rollouts")
		return nil, err
	}

	for i := range rList.Items {
		rollout := rList.Items[i]
		workloadRef := rollout.Spec.WorkloadRef
		refGV, err := schema.ParseGroupVersion(workloadRef.APIVersion)
		if err != nil {
			logger.Error(err, "failed to parse rollout workload ref group version", "rollout", rollout.Name, "apiVersion", workloadRef.APIVersion)
			continue
		}
		refGVK := refGV.WithKind(workloadRef.Kind)

		if !reflect.DeepEqual(refGVK, workloadInfo.GroupVersionKind) {
			// group version kind not match
			// logger.Info("gvk not match", "gvk", workloadInfo.GVK.String(), "refGVK", refGVK)
			continue
		}

		macher := workload.MatchAsMatcher(workloadRef.Match)
		if macher.Matches(workloadInfo.ClusterName, workloadInfo.Name, workloadInfo.Labels) {
			return &rollout, nil
		}
	}

	// not found
	return nil, nil
}

func enqueueRolloutForStrategyHandler(
	reader client.Reader,
	logger logr.Logger,
) handler.EventHandler {
	mapFunc := func(obj client.Object) (result []reconcile.Request) {
		strategy, ok := obj.(*rolloutv1alpha1.RolloutStrategy)
		if !ok {
			return
		}

		rList := &rolloutv1alpha1.RolloutList{}
		ctx := clusterinfo.WithCluster(context.TODO(), clusterinfo.Fed)
		if err := reader.List(ctx, rList, client.InNamespace(strategy.Namespace)); err != nil {
			logger.Error(err, "failed to list rollouts")
			return
		}
		name := strategy.GetName()

		for _, rollout := range rList.Items {
			if rollout.Spec.StrategyRef == name {
				result = append(result, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: rollout.Namespace,
						Name:      rollout.Name,
					},
				})
			}
		}
		return
	}

	return handler.EnqueueRequestsFromMapFunc(mapFunc)
}
