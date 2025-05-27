/**
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

package utils

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// CreateOrUpdateOnConflict creates or updates the given object in the Kubernetes
// cluster. The object's desired state must be reconciled with the existing
// state inside the passed in callback MutateFn.
//
// The MutateFn is called regardless of creating or updating an object.
//
// It returns the executed operation and an error.
func CreateOrUpdateOnConflict(ctx context.Context, reader client.Reader, writer client.Writer, obj client.Object, f controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	key := client.ObjectKeyFromObject(obj)
	if err := reader.Get(ctx, key, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		if err := mutate(f, key, obj); err != nil {
			return controllerutil.OperationResultNone, err
		}
		if err := writer.Create(ctx, obj); err != nil {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultCreated, nil
	}
	updated, err := UpdateOnConflict(ctx, reader, writer, obj, f)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}
	if !updated {
		return controllerutil.OperationResultNone, nil
	}
	return controllerutil.OperationResultUpdated, nil
}

type UpdateWriter interface {
	// Update updates the fields corresponding to the status subresource for the
	// given obj. obj must be a struct pointer so that obj can be updated
	// with the content returned by the Server.
	Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
}

// UpdateOnConflict attempts to update a resource while avoiding conflicts that may arise from concurrent modifications.
// It utilizes the mutateFn function to apply changes to the original obj and then attempts an update using the writer,
// which can be either client.Writer or client.StatusWriter.
// In case of an update failure due to a conflict, UpdateOnConflict will retrieve the latest version of the object using
// the reader and attempt the update again.
// The retry mechanism adheres to the retry.DefaultBackoff policy.
func UpdateOnConflict(ctx context.Context, reader client.Reader, writer UpdateWriter, obj client.Object, mutateFn controllerutil.MutateFn) (updated bool, err error) {
	key := client.ObjectKeyFromObject(obj)
	first := true

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if !first {
			// refresh object
			if innerErr := reader.Get(ctx, key, obj); innerErr != nil {
				return innerErr
			}
		} else {
			first = false
		}

		existing := obj.DeepCopyObject()
		if innerErr := mutate(mutateFn, key, obj); innerErr != nil {
			return innerErr
		}

		if equality.Semantic.DeepEqual(existing, obj) {
			// nothing changed, skip update
			return nil
		}

		if innerErr := writer.Update(ctx, obj); innerErr != nil {
			return innerErr
		}

		updated = true
		return nil
	})

	return updated, err
}

// mutate wraps a MutateFn and applies validation to its result.
func mutate(f controllerutil.MutateFn, key client.ObjectKey, obj client.Object) error {
	if err := f(); err != nil {
		return err
	}
	if newKey := client.ObjectKeyFromObject(obj); key != newKey {
		return fmt.Errorf("MutateFn cannot mutate object name and/or object namespace")
	}
	return nil
}

func MutateLabels(obj client.Object, mutateFn func(labels map[string]string)) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	mutateFn(labels)
	obj.SetLabels(labels)
}

func MutateAnnotations(obj client.Object, mutateFn func(annotations map[string]string)) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	mutateFn(annotations)
	obj.SetAnnotations(annotations)
}
