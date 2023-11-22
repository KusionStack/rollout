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
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type UpdateWriter interface {
	// Update updates the fields corresponding to the status subresource for the
	// given obj. obj must be a struct pointer so that obj can be updated
	// with the content returned by the Server.
	Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error

	// Patch patches the given object's subresource. obj must be a struct
	// pointer so that obj can be updated with the content returned by the
	// Server.
	Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
}

// reader is used to get freshest object
// writer can be client.Writer or client.StatusWriter
func UpdateOnConflict(ctx context.Context, reader client.Reader, writer UpdateWriter, obj client.Object, mutateFn controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	key := client.ObjectKeyFromObject(obj)
	result := controllerutil.OperationResultNone
	first := true

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if !first {
			// refresh object
			if err := reader.Get(ctx, key, obj); err != nil {
				return err
			}
		} else {
			first = false
		}

		existing := obj.DeepCopyObject()
		if err := mutate(mutateFn, key, obj); err != nil {
			return err
		}

		if equality.Semantic.DeepEqual(existing, obj) {
			// nothing changed, skip update
			return nil
		}

		if err := writer.Update(ctx, obj); err != nil {
			return err
		}

		result = controllerutil.OperationResultUpdated
		return nil
	})

	return result, err
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
