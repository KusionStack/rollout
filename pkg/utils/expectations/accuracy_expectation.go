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

package expectations

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

// NewAccuracyExpectations returns a store for AccuracyExpectations.
func NewAccuracyExpectations() *AccuracyExpectations {
	return &AccuracyExpectations{cache.NewStore(ExpKeyFunc)}
}

type AccuracyExpectations struct {
	cache.Store
}

func (r *AccuracyExpectations) InitExpectation(controllerKey string) {
	if _, exists, err := r.GetByKey(controllerKey); err == nil && !exists {
		r.Add(&AccuracyExpectationItem{key: controllerKey, Store: sets.String{}, timestamp: time.Now()})
	}
}

func (r *AccuracyExpectations) GetExpectations(controllerKey string) (*AccuracyExpectationItem, bool, error) {
	if exp, exists, err := r.GetByKey(controllerKey); err == nil && exists {
		return exp.(*AccuracyExpectationItem), true, nil
	} else {
		return nil, false, err
	}
}

func (r *AccuracyExpectations) DeleteExpectations(controllerKey string) {
	if exp, exists, err := r.GetByKey(controllerKey); err == nil && exists {
		if err := r.Delete(exp); err != nil {
			klog.V(2).Infof("Error deleting expectations for controller %v: %v", controllerKey, err)
		}
	}
}

func (r *AccuracyExpectations) SatisfiedExpectations(controllerKey string) bool {
	if exp, exists, err := r.GetExpectations(controllerKey); exists {
		if exp.Fulfilled() {
			klog.V(4).Infof("Accuracy expectations fulfilled %s", controllerKey)
			return true
		} else if exp.isExpired() {
			klog.Errorf("Accuracy expectation expired for key %s", controllerKey)
			panic(fmt.Sprintf("expected panic for accuracy expectation timeout for key %s", controllerKey))
		} else {
			klog.V(4).Infof("Controller still waiting on accuracy expectations %s", controllerKey)
			return false
		}
	} else if err != nil {
		klog.V(2).Infof("Error encountered while checking accuracy expectations %#v, forcing sync", err)
	} else {
		// When a new controller is created, it doesn't have expectations.
		// When it doesn't see expected watch events for > TTL, the expectations expire.
		//	- In this case it wakes up, creates/deletes controllees, and sets expectations again.
		// When it has satisfied expectations and no controllees need to be created/destroyed > TTL, the expectations expire.
		//	- In this case it continues without setting expectations till it needs to create/delete controllees.
		klog.V(4).Infof("Accuracy controller %v either never recorded expectations, or the ttl expired.", controllerKey)
	}
	// Trigger a sync if we either encountered and error (which shouldn't happen since we're
	// getting from local store) or this controller hasn't established expectations.
	return true
}

func (r *AccuracyExpectations) SetExpectations(controllerKey string, items ...string) error {
	exp := &AccuracyExpectationItem{key: controllerKey, Store: sets.NewString(items...), timestamp: time.Now()}
	klog.V(4).Infof("Setting expectations %#v", exp)
	return r.Add(exp)
}

func (r *AccuracyExpectations) ExpectUpdate(controllerKey string, items ...string) error {
	if exp, exists, err := r.GetExpectations(controllerKey); err != nil {
		return err
	} else if exists {
		exp.Add(items...)
	}
	return nil
}

func (r *AccuracyExpectations) UpdateObserved(controllerKey string, items ...string) error {
	if exp, exists, err := r.GetExpectations(controllerKey); err != nil {
		return err
	} else if exists {
		exp.Del(items...)
	}
	return nil
}

func (r *AccuracyExpectations) GetRestItems(controllerKey string) []string {
	if exp, exists, err := r.GetExpectations(controllerKey); err != nil {
		return []string{}
	} else if exists {
		return exp.List()
	}
	return []string{}
}

type AccuracyExpectationItem struct {
	lock      sync.RWMutex
	Store     sets.String
	key       string
	timestamp time.Time
}

func (i *AccuracyExpectationItem) Add(keys ...string) {
	i.lock.Lock()
	defer i.lock.Unlock()

	i.Store.Insert(keys...)
	i.timestamp = time.Now()
}

func (i *AccuracyExpectationItem) Del(keys ...string) {
	i.lock.Lock()
	defer i.lock.Unlock()

	i.Store.Delete(keys...)
	i.timestamp = time.Now()
}

func (i *AccuracyExpectationItem) List() []string {
	i.lock.Lock()
	defer i.lock.Unlock()

	return i.Store.List()
}

func (i *AccuracyExpectationItem) Fulfilled() bool {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if i.Store.Len() == 0 {
		return true
	}

	// TODO why it has empty string element
	if i.Store.Len() == 1 {
		return i.Store.Has("")
	}

	return false
}

func (i *AccuracyExpectationItem) isExpired() bool {
	return time.Now().Sub(i.timestamp) > ExpectationsTimeout
}
