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
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rolloutvalidation "kusionstack.io/rollout/apis/rollout/v1alpha1/validation"
	"kusionstack.io/rollout/pkg/controllers/registry"
)

// +kubebuilder:webhook:path=/webhooks/validating/rolloutrun,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=rolloutruns,verbs=create;update,versions=v1alpha1,name=rolloutruns.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/rolloutstrategy,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=rolloutstrategies,verbs=create;update,versions=v1alpha1,name=rolloutstrategies.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/rollout,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=rollouts,verbs=create;update,versions=v1alpha1,name=rollouts.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/traffictopology,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=traffictopologies,verbs=create;update,versions=v1alpha1,name=traffictopologies.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/backendrouting,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=backendroutings,verbs=create;update,versions=v1alpha1,name=backendroutings.rollout.kusionstack.io

const (
	WebhookInitializerName = "validate-rollout.kusionstack.io"
)

func NewValidatingHandlers(mgr manager.Manager) map[schema.GroupKind]admission.Handler {
	validator := &Validator{}
	objs := []runtime.Object{
		&rolloutv1alpha1.Rollout{},
		&rolloutv1alpha1.RolloutStrategy{},
		&rolloutv1alpha1.RolloutRun{},
		&rolloutv1alpha1.TrafficTopology{},
		&rolloutv1alpha1.BackendRouting{},
	}
	handlers := make(map[schema.GroupKind]admission.Handler, len(objs))
	for _, obj := range objs {
		handler := admission.WithCustomValidator(obj, validator).Handler
		t := reflect.TypeOf(obj)
		t = t.Elem()
		kind := t.Name()
		gk := schema.GroupKind{
			Group: rolloutv1alpha1.GroupName,
			Kind:  kind,
		}
		handlers[gk] = handler
	}

	return handlers
}

var _ admission.CustomValidator = &Validator{}

type Validator struct{}

// ValidateCreate implements admission.CustomValidator.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	var errs field.ErrorList
	switch t := obj.(type) {
	case *rolloutv1alpha1.Rollout:
		errs = rolloutvalidation.ValidateRollout(t, registry.IsSupportedWorkload)
	case *rolloutv1alpha1.RolloutStrategy:
		errs = rolloutvalidation.ValidateRolloutStrategy(t)
	case *rolloutv1alpha1.RolloutRun:
		errs = rolloutvalidation.ValidateRolloutRun(t)
	case *rolloutv1alpha1.TrafficTopology:
		errs = rolloutvalidation.ValidateTrafficTopology(t, registry.IsSupportedWorkload, registry.IsSupportedRoute, registry.IsSupportedBackend)
	case *rolloutv1alpha1.BackendRouting:
		errs = rolloutvalidation.ValidateBackendRouting(t, registry.IsSupportedRoute, registry.IsSupportedBackend)
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
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	var errs field.ErrorList
	switch newV := newObj.(type) {
	case *rolloutv1alpha1.Rollout:
		errs = rolloutvalidation.ValidateRollout(newV, registry.IsSupportedWorkload)
		if len(errs) == 0 {
			errs = rolloutvalidation.ValidateRolloutUpdate(newV, oldObj.(*rolloutv1alpha1.Rollout))
		}
	case *rolloutv1alpha1.RolloutStrategy:
		errs = rolloutvalidation.ValidateRolloutStrategy(newV)
	case *rolloutv1alpha1.RolloutRun:
		errs = rolloutvalidation.ValidateRolloutRun(newV)
		if len(errs) == 0 {
			errs = rolloutvalidation.ValidateRolloutRunUpdate(newV, oldObj.(*rolloutv1alpha1.RolloutRun))
		}
	case *rolloutv1alpha1.TrafficTopology:
		errs = rolloutvalidation.ValidateTrafficTopology(newV, registry.IsSupportedWorkload, registry.IsSupportedRoute, registry.IsSupportedBackend)
		// TODO: validate traffic topology update
	case *rolloutv1alpha1.BackendRouting:
		errs = rolloutvalidation.ValidateBackendRouting(newV, registry.IsSupportedRoute, registry.IsSupportedBackend)
		// TODO: validate backend routing update
	default:
		return fmt.Errorf("unexpected object type %T", newObj)
	}
	return errs.ToAggregate()
}
