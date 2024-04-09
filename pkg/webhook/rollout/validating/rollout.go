/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package validating

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/apis/rollout/v1alpha1/validation"
	"kusionstack.io/rollout/pkg/controllers/registry"
)

// +kubebuilder:webhook:path=/webhooks/validating/rollout,mutating=false,failurePolicy=ignore,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=rollouts,verbs=create;update,versions=v1alpha1,name=rollouts.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/rolloutstrategy,mutating=false,failurePolicy=ignore,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=rolloutstrategies,verbs=create;update,versions=v1alpha1,name=rolloutstrategies.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/rolloutrun,mutating=false,failurePolicy=ignore,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=rolloutruns,verbs=create;update,versions=v1alpha1,name=rolloutruns.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/traffictopology,mutating=false,failurePolicy=ignore,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=traffictopologies,verbs=create;update,versions=v1alpha1,name=traffictopologies.rollout.kusionstack.io

const (
	ValidatingRollout = "validate-rollout.kusionstack.io"
)

func NewValidatingHandler(mgr manager.Manager) map[runtime.Object]http.Handler {
	validator := &Validator{Client: mgr.GetClient()}
	objs := []runtime.Object{
		&rolloutv1alpha1.Rollout{},
		&rolloutv1alpha1.RolloutStrategy{},
		&rolloutv1alpha1.RolloutRun{},
		&rolloutv1alpha1.TrafficTopology{},
	}
	result := make(map[runtime.Object]http.Handler, len(objs))
	for _, obj := range objs {
		result[obj] = admission.WithCustomValidator(obj, validator)
	}

	return result
}

var _ admission.CustomValidator = &Validator{}

type Validator struct {
	Client client.Client
}

// ValidateCreate implements admission.CustomValidator.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	var errs field.ErrorList
	switch t := obj.(type) {
	case *rolloutv1alpha1.Rollout:
		errs = v.validateRollout(ctx, t)
	case *rolloutv1alpha1.RolloutStrategy:
		errs = validation.ValidateRolloutStrategy(t)
	case *rolloutv1alpha1.RolloutRun:
		errs = validation.ValidateRolloutRun(t)
	case *rolloutv1alpha1.TrafficTopology:
		errs = validation.ValidateTrafficTopology(t)
	default:
		return fmt.Errorf("unexpected object type %T", obj)
	}
	return errs.ToAggregate()
}

// ValidateDelete implements admission.CustomValidator.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// ValidateUpdate implements admission.CustomValidator.
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) error {
	var errs field.ErrorList
	switch newV := newObj.(type) {
	case *rolloutv1alpha1.Rollout:
		errs = v.validateRollout(ctx, newV)
		if len(errs) == 0 {
			errs = validation.ValidateRolloutUpdate(newV, oldObj.(*rolloutv1alpha1.Rollout))
		}
	case *rolloutv1alpha1.RolloutStrategy:
		errs = validation.ValidateRolloutStrategy(newV)
	case *rolloutv1alpha1.RolloutRun:
		errs = validation.ValidateRolloutRun(newV)
		if len(errs) == 0 {
			errs = validation.ValidateRolloutRunUpdate(newV, oldObj.(*rolloutv1alpha1.RolloutRun))
		}
	case *rolloutv1alpha1.TrafficTopology:
		errs = validation.ValidateTrafficTopology(newV)
	default:
		return fmt.Errorf("unexpected object type %T", newObj)
	}
	return errs.ToAggregate()
}

func (v *Validator) validateRollout(ctx context.Context, obj *rolloutv1alpha1.Rollout) field.ErrorList {
	errs := validation.ValidateRollout(obj, registry.IsSupportedWorkload)
	if len(errs) > 0 {
		return errs
	}

	var strategy rolloutv1alpha1.RolloutStrategy
	err := v.Client.Get(
		clusterinfo.WithCluster(ctx, clusterinfo.Fed),
		client.ObjectKey{Namespace: obj.Namespace, Name: obj.Spec.StrategyRef},
		&strategy,
	)
	if err != nil {
		errs = append(errs, newGetFieldError(field.NewPath("spec", "strategyRef"), obj.Spec.StrategyRef, err))
	}

	for i, tt := range obj.Spec.TrafficTopologyRefs {
		var topology rolloutv1alpha1.TrafficTopology
		err := v.Client.Get(
			clusterinfo.WithCluster(ctx, clusterinfo.Fed),
			client.ObjectKey{Namespace: obj.Namespace, Name: tt},
			&topology,
		)
		if err != nil {
			errs = append(errs, newGetFieldError(field.NewPath("spec", "trafficTopologyRefs").Index(i), tt, err))
		}
	}
	return errs
}

func newGetFieldError(fldPath *field.Path, value interface{}, err error) *field.Error {
	if errors.IsNotFound(err) {
		return field.NotFound(fldPath, value)
	}
	return field.InternalError(fldPath, err)
}
