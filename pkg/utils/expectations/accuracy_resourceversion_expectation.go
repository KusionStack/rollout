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
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewAccuracyResourceVersionExpectations returns a store for AccuracyResourceVersionExpectations.
func NewAccuracyResourceVersionExpectations(client client.Client) *AccuracyResourceVersionExpectations {
	return &AccuracyResourceVersionExpectations{cache.NewStore(ExpKeyFunc), client}
}

type AccuracyResourceVersionExpectations struct {
	cache.Store
	client client.Client
}

func (r *AccuracyResourceVersionExpectations) InitExpectation(controllerKey string) {
	if _, exists, err := r.GetByKey(controllerKey); err == nil && !exists {
		r.Add(&AccuracyResourceVersionExpectationItem{client: r.client, key: controllerKey, Store: map[string]*ItemInfo{}, timestamp: time.Now()})
	}
}

func (r *AccuracyResourceVersionExpectations) GetExpectations(controllerKey string) (*AccuracyResourceVersionExpectationItem, bool, error) {
	if exp, exists, err := r.GetByKey(controllerKey); err == nil && exists {
		return exp.(*AccuracyResourceVersionExpectationItem), true, nil
	} else {
		return nil, false, err
	}
}

func (r *AccuracyResourceVersionExpectations) DeleteExpectations(controllerKey string) {
	if exp, exists, err := r.GetByKey(controllerKey); err == nil && exists {
		if err := r.Delete(exp); err != nil {
			klog.V(2).Infof("Error deleting expectations for controller %v: %v", controllerKey, err)
		}
	}
}

func (r *AccuracyResourceVersionExpectations) SatisfiedExpectations(controllerKey string) (bool, error) {
	if exp, exists, err := r.GetExpectations(controllerKey); exists {
		if satisfied, err := exp.Fulfilled(); err != nil {
			return false, err
		} else if satisfied {
			klog.V(4).Infof("Accuracy resource version expectations fulfilled %s", controllerKey)
			return true, nil
		} else if exp.isExpired() {
			klog.Errorf("Accuracy resource version expectation expired for key %s", controllerKey)
			panic(fmt.Sprintf("expected panic for accuracy resource version expectation timeout for key %s", controllerKey))
		} else {
			klog.V(4).Infof("Controller still waiting on accuracy resource version expectations %s", controllerKey)
			return false, nil
		}
	} else if err != nil {
		klog.V(2).Infof("Error encountered while checking accuracy resource version expectations %#v, forcing sync", err)
	} else {
		// When a new controller is created, it doesn't have expectations.
		// When it doesn't see expected watch events for > TTL, the expectations expire.
		//	- In this case it wakes up, creates/deletes controllees, and sets expectations again.
		// When it has satisfied expectations and no controllees need to be created/destroyed > TTL, the expectations expire.
		//	- In this case it continues without setting expectations till it needs to create/delete controllees.
		klog.V(4).Infof("Accuracy resource version controller %v either never recorded expectations, or the ttl expired.", controllerKey)
	}
	// Trigger a sync if we either encountered and error (which shouldn't happen since we're
	// getting from local store) or this controller hasn't established expectations.
	return true, nil
}

func (r *AccuracyResourceVersionExpectations) ExpectUpdate(controllerKey string, kind ExpectedResourceType, namespace, name string, rv string) error {
	if exp, exists, err := r.GetExpectations(controllerKey); err != nil {
		return err
	} else if exists {
		exp.Expect(kind, namespace, name, rv)
	}
	return nil
}

func (r *AccuracyResourceVersionExpectations) UpdateObserved(controllerKey string, kind ExpectedResourceType, namespace, name string, rv string) error {
	if exp, exists, err := r.GetExpectations(controllerKey); err != nil {
		return err
	} else if exists {
		exp.Observe(kind, namespace, name, rv)
	}
	return nil
}

func (r *AccuracyResourceVersionExpectations) DeleteSubExpectation(controllerKey string, kind ExpectedResourceType, namespace, name string) error {
	if exp, exists, err := r.GetExpectations(controllerKey); err != nil {
		return err
	} else if exists {
		exp.Delete(kind, namespace, name)
	}
	return nil
}

type ItemInfo struct {
	Kind            ExpectedResourceType
	Namespace       string
	Name            string
	ResourceVersion int64
}

type AccuracyResourceVersionExpectationItem struct {
	client    client.Client
	lock      sync.RWMutex
	Store     map[string]*ItemInfo
	key       string
	timestamp time.Time
}

func (i *AccuracyResourceVersionExpectationItem) Expect(kind ExpectedResourceType, namespace, name string, rv string) {
	i.lock.Lock()
	defer i.lock.Unlock()

	val, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("expected no err, got: %ss", err))
	}

	i.Store[i.buildKey(kind, namespace, name)] = &ItemInfo{Kind: kind, Namespace: namespace, Name: name, ResourceVersion: val}
	i.timestamp = time.Now()
}

func (i *AccuracyResourceVersionExpectationItem) Observe(kind ExpectedResourceType, namespace, name string, rv string) {
	i.lock.Lock()
	defer i.lock.Unlock()

	val, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("expected no err, got: %ss", err))
	}

	key := i.buildKey(kind, namespace, name)
	if v, exist := i.Store[key]; !exist {
		return
	} else if val > v.ResourceVersion {
		delete(i.Store, key)
	}
}

func (i *AccuracyResourceVersionExpectationItem) Delete(kind ExpectedResourceType, namespace, name string) {
	i.lock.Lock()
	defer i.lock.Unlock()

	key := i.buildKey(kind, namespace, name)
	if _, exist := i.Store[key]; !exist {
		return
	} else {
		delete(i.Store, key)
	}
}

func (i *AccuracyResourceVersionExpectationItem) Fulfilled() (bool, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if len(i.Store) == 0 {
		return true, nil
	}

	result := true
	needDeleteKeys := map[string]struct{}{}
	for key, info := range i.Store {
		if info == nil {
			continue
		}

		resource := ResourceInitializers[info.Kind]()
		if err := i.client.Get(context.Background(), types.NamespacedName{Namespace: info.Namespace, Name: info.Name}, resource); err != nil {
			return false, fmt.Errorf("fail to checkout resourceVersion for %s %s/%s: %s", info.Kind, info.Namespace, info.Name, err)
		}

		rvStr := resource.(metav1.Object).GetResourceVersion()
		if rv, err := strconv.ParseInt(rvStr, 10, 64); err != nil {
			return false, fmt.Errorf("fail to parse resourceVersion %s: %s", rvStr, err)
		} else if rv > info.ResourceVersion {
			needDeleteKeys[key] = struct{}{}
		} else {
			result = false
		}
	}

	if len(needDeleteKeys) > 0 {
		for key := range needDeleteKeys {
			delete(i.Store, key)
		}
	}

	return result, nil
}

func (i *AccuracyResourceVersionExpectationItem) isExpired() bool {
	return time.Now().Sub(i.timestamp) > ExpectationsTimeout
}

func (i *AccuracyResourceVersionExpectationItem) buildKey(kind ExpectedResourceType, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
}
