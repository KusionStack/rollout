/**
 * Copyright 2024 The KusionStack Authors
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

package statefulset

import (
	"context"
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	"kusionstack.io/rollout/apis/rollout/v1alpha1"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

func (w *workloadImpl) CanaryStrategy() (workload.CanaryStrategy, error) {
	cw, err := w.canaryWorkload()
	if err != nil {
		return nil, err
	}
	return &canaryStrategy{
		stable: w,
		canary: cw,
	}, nil
}

func (w *workloadImpl) canaryWorkload() (*workloadImpl, error) {
	stableObj := w.obj
	canaryName := stableObj.Name + "-canary"
	var canaryObj appsv1.StatefulSet
	err := w.client.Get(
		clusterinfo.WithCluster(context.TODO(), w.info.ClusterName),
		client.ObjectKey{Namespace: stableObj.Namespace, Name: canaryName},
		&canaryObj,
	)
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	if errors.IsNotFound(err) {
		canaryObj = appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   stableObj.Namespace,
				Name:        canaryName,
				Labels:      stableObj.Labels,
				Annotations: stableObj.Annotations,
				Finalizers: []string{
					rolloutapi.FinalizerCanaryResourceProtection,
				},
			},
			Spec: *stableObj.Spec.DeepCopy(),
		}
	}

	return newFrom(w.info.ClusterName, w.client, &canaryObj), nil
}

var _ workload.CanaryStrategy = &canaryStrategy{}

type canaryStrategy struct {
	stable *workloadImpl
	canary *workloadImpl
}

// GetStableInfo implements workload.CanaryStrategy.
func (s *canaryStrategy) GetStableInfo() workload.Info {
	return s.stable.GetInfo()
}

// Initialize implements workload.CanaryStrategy.
func (s *canaryStrategy) Initialize(rollout string, rolloutRun string) error {
	info := rolloutv1alpha1.ProgressingInfo{
		RolloutName: rollout,
		RolloutID:   rolloutRun,
		Canary:      &rolloutv1alpha1.CanaryProgressingInfo{},
	}
	progress, _ := json.Marshal(info)

	// set progressing info
	return s.stable.UpdateOnConflict(context.TODO(), func(obj client.Object) error {
		utils.MutateAnnotations(obj, func(annotations map[string]string) {
			annotations[rolloutapi.AnnoRolloutProgressingInfo] = string(progress)
		})
		return nil
	})
}

func (s *canaryStrategy) Finalize() error {
	// delete progressing info
	return s.stable.UpdateOnConflict(context.TODO(), func(obj client.Object) error {
		utils.MutateAnnotations(obj, func(annotations map[string]string) {
			delete(annotations, rolloutapi.AnnoRolloutProgressingInfo)
		})
		return nil
	})
}

// GetCanaryInfo implements workload.CanaryStrategy.
func (s *canaryStrategy) GetCanaryInfo() workload.Info {
	return s.canary.GetInfo()
}

// CreateOrUpdate implements workload.CanaryStrategy.
func (s *canaryStrategy) CreateOrUpdate(canaryReplicas intstr.IntOrString, podTemplatePatch *v1alpha1.MetadataPatch) (controllerutil.OperationResult, error) {
	stableObj := s.stable.obj
	replicas, err := workload.CalculatePartitionReplicas(stableObj.Spec.Replicas, canaryReplicas)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}

	// create or update canary resource
	canaryObj := s.canary.obj

	ctx := clusterinfo.WithCluster(context.TODO(), stableObj.ClusterName)
	op, err := controllerutil.CreateOrUpdate(ctx, s.stable.client, canaryObj, func() error {
		canaryObj.Spec.Replicas = &replicas
		utils.MutateLabels(canaryObj, func(labels map[string]string) {
			labels[rolloutapi.LabelCanary] = "true"
		})
		applyPodTemplateMetadataPatch(canaryObj, podTemplatePatch)
		// TODO: we also need to change canaryObj.Spec.ServiceName and VolumeClaimTemplates and volumes in podTemplate
		return nil
	})

	if err != nil {
		return op, err
	}

	if op == controllerutil.OperationResultNone {
		return op, nil
	}

	s.canary.SetObject(canaryObj)
	return op, nil
}

// Delete implements workload.CanaryStrategy.
func (s *canaryStrategy) Delete() error {
	ctx := clusterinfo.WithCluster(context.TODO(), s.canary.GetInfo().ClusterName)
	return client.IgnoreNotFound(s.canary.client.Delete(ctx, s.canary.obj))
}

func applyPodTemplateMetadataPatch(obj *appsv1.StatefulSet, patch *rolloutv1alpha1.MetadataPatch) {
	if patch == nil {
		return
	}
	if len(patch.Labels) > 0 {
		if obj.Spec.Selector.MatchLabels == nil {
			obj.Spec.Selector.MatchLabels = make(map[string]string)
		}
		for k, v := range patch.Labels {
			obj.Spec.Selector.MatchLabels[k] = v
		}

		if obj.Spec.Template.Labels == nil {
			obj.Spec.Template.Labels = make(map[string]string)
		}
		for k, v := range patch.Labels {
			obj.Spec.Template.Labels[k] = v
		}
	}
	if len(patch.Annotations) > 0 {
		if obj.Spec.Template.Annotations == nil {
			obj.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range patch.Annotations {
			obj.Spec.Template.Annotations[k] = v
		}
	}
}
