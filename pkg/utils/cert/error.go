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

package cert

import (
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var errNotFound = errors.New("not found")

func newNotFound(name string, err error) error {
	return fmt.Errorf("%s %w: %v", name, errNotFound, err)
}

func IsNotFound(err error) bool {
	return apierrors.IsNotFound(err) || errors.Is(err, errNotFound)
}

func IsConflict(err error) bool {
	return apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err)
}
