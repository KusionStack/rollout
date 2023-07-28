/*
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// MaxConcurrencyReconcileTimes is the max concurrency reconcile times
	MaxConcurrencyReconcileTimes float64 = 1
)

// RemoveFinalizer removes the given finalizer from the given object's metadata.
func RemoveFinalizer(meta *metav1.ObjectMeta, needle string) bool {
	finalizers := make([]string, 0)
	found := false
	if meta.Finalizers != nil {
		for _, finalizer := range meta.Finalizers {
			if finalizer != needle {
				finalizers = append(finalizers, finalizer)
			} else {
				found = true
			}
		}
		meta.Finalizers = finalizers
	}

	return found
}

// AddFinalizer adds the given finalizer to the given object's metadata.
func AddFinalizer(meta *metav1.ObjectMeta, finalizer string) bool {
	if meta.Finalizers == nil {
		meta.Finalizers = []string{}
	}
	for _, s := range meta.Finalizers {
		if s == finalizer {
			return false
		}
	}
	meta.Finalizers = append(meta.Finalizers, finalizer)
	return true
}

// ContainsFinalizer checks if the given object's metadata contains the given finalizer.
func ContainsFinalizer(meta metav1.Object, needle string) bool {
	if meta.GetFinalizers() == nil {
		return false
	}
	for _, finalizer := range meta.GetFinalizers() {
		if finalizer == needle {
			return true
		}
	}
	return false
}
