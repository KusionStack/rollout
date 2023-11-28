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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

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
