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

package registry

import (
	"fmt"
	"sync"
)

// Registry is a generic registry that stores items indexed by keys.
type Registry[KEY comparable, ITEM any] interface {
	// Register adds the given item to the registry.
	// It overwrites any existing item associated with the same key.
	Register(key KEY, item ITEM)
	// Get returns the item associated with the given key.
	// If no such key exists, it returns an error.
	Get(key KEY) (ITEM, error)
	// Range calls f for each item in the registry.
	Range(f func(key KEY, item ITEM) bool)
}

var _ Registry[string, any] = &registry[string, any]{}

// New creates a new Registry.
func New[KEY comparable, ITEM any]() Registry[KEY, ITEM] {
	return &registry[KEY, ITEM]{
		store: sync.Map{}, // map[KEY]ITEM,
	}
}

// registry is a generic registry that stores items indexed by keys.
type registry[KEY comparable, ITEM any] struct {
	store sync.Map // map[KEY]ITEM
}

// Get implements Registry.
func (r *registry[KEY, ITEM]) Get(key KEY) (ITEM, error) {
	value, ok := r.store.Load(key)
	var zero ITEM
	if !ok {
		return zero, fmt.Errorf("key %v not found in registry", key)
	}
	return value.(ITEM), nil
}

// Range implements Registry.
func (r *registry[KEY, ITEM]) Range(f func(key KEY, item ITEM) bool) {
	r.store.Range(func(key, value any) bool {
		return f(key.(KEY), value.(ITEM))
	})
}

// Set implements Registry.
func (r *registry[KEY, ITEM]) Register(key KEY, item ITEM) {
	r.store.Store(key, item)
}
