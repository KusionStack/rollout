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

type PatchWriter interface {
	// Patch updates the fields corresponding to the status subresource for the
	// given obj. obj must be a struct pointer so that obj can be updated
	// with the content returned by the Server.
	Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
}

// PatchOnConflict attempts to update a resource while avoiding conflicts that may arise from concurrent modifications.
// It utilizes the mutateFn function to apply changes to the original obj and then attempts an update using the writer,
// which can be either client.Writer or client.StatusWriter.
// In case of an update failure due to a conflict, UpdateOnConflict will retrieve the latest version of the object using
// the reader and attempt the update again.
// The retry mechanism adheres to the retry.DefaultBackoff policy.
func PatchOnConflict(ctx context.Context, reader client.Reader, writer PatchWriter, obj client.Object, mutateFn controllerutil.MutateFn) (patched bool, err error) {
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

		var existing client.Object
		var ok bool
		if existing, ok = obj.DeepCopyObject().(client.Object); !ok {
			return fmt.Errorf("object %s does not implement client.Object", key)
		}
		if innerErr := mutate(mutateFn, key, obj); innerErr != nil {
			return innerErr
		}

		if equality.Semantic.DeepEqual(existing, obj) {
			// nothing changed, skip update
			return nil
		}

		patch := client.MergeFrom(existing)
		if innerErr := writer.Patch(ctx, obj, patch); innerErr != nil {
			return innerErr
		}

		patched = true
		return nil
	})

	return patched, err
}
