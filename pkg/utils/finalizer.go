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

package utils

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func AddAndUpdateFinalizer(c client.Client, obj client.Object, finalizer string) error {
	if controllerutil.ContainsFinalizer(obj, finalizer) {
		return nil
	}
	_, err := UpdateOnConflict(context.TODO(), c, c, obj, func() error {
		controllerutil.AddFinalizer(obj, finalizer)
		return nil
	})
	return err
}

func RemoveAndUpdateFinalizer(c client.Client, obj client.Object, finalizer string) error {
	if !controllerutil.ContainsFinalizer(obj, finalizer) {
		return nil
	}
	_, err := UpdateOnConflict(context.TODO(), c, c, obj, func() error {
		controllerutil.RemoveFinalizer(obj, finalizer)
		return nil
	})
	return err
}

func DeleteWithFinalizer(ctx context.Context, c client.Client, obj client.Object, finalizer string) error {
	err := RemoveAndUpdateFinalizer(c, obj, finalizer)
	if err != nil {
		return err
	}
	return c.Delete(ctx, obj)
}
