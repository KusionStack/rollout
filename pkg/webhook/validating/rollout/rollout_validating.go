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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/apis/rollout/v1alpha1/validation"
	"kusionstack.io/rollout/pkg/controllers/registry"
)

// +kubebuilder:webhook:path=/webhooks/validating/rolloutrun,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=rolloutruns,verbs=create;update,versions=v1alpha1,name=rolloutruns.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/rolloutstrategy,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=rolloutstrategies,verbs=create;update,versions=v1alpha1,name=rolloutstrategies.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/rollout,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=rollouts,verbs=create;update,versions=v1alpha1,name=rollouts.rollout.kusionstack.io
// +kubebuilder:webhook:path=/webhooks/validating/traffictopology,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="rollout.kusionstack.io",resources=traffictopologies,verbs=create;update,versions=v1alpha1,name=traffictopologies.rollout.kusionstack.io

const (
	WebhookInitializerName = "validate-rollout.kusionstack.io"
)

func NewValidatingHandlers(mgr manager.Manager) map[runtime.Object]http.Handler {
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
		errs = validation.ValidateRollout(t, registry.IsSupportedWorkload)
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
		errs = validation.ValidateRollout(newV, registry.IsSupportedWorkload)
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
