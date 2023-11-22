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

package eventhandler

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kusionstack.io/rollout/pkg/utils/expectations"
)

type enqueueRequestForOwnerWithCreationObserved struct {
	*handler.EnqueueRequestForOwner

	Expectation expectations.ControllerExpectationsInterface
}

func EqueueRequestForOwnerWithCreationObserved(ownerType runtime.Object, isController bool, expectation expectations.ControllerExpectationsInterface) handler.EventHandler {
	return &enqueueRequestForOwnerWithCreationObserved{
		EnqueueRequestForOwner: &handler.EnqueueRequestForOwner{
			OwnerType:    ownerType,
			IsController: isController,
		},
		Expectation: expectation,
	}
}

func (h *enqueueRequestForOwnerWithCreationObserved) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	newQ := &dependentCreatedExpectationWorkqueue{
		RateLimitingInterface: q,
		e:                     h.Expectation,
	}
	h.EnqueueRequestForOwner.Create(evt, newQ)
}

type dependentCreatedExpectationWorkqueue struct {
	workqueue.RateLimitingInterface
	e expectations.ControllerExpectationsInterface
}

func (q *dependentCreatedExpectationWorkqueue) Add(item interface{}) {
	req := item.(reconcile.Request)
	q.e.CreationObserved(req.String())
	q.RateLimitingInterface.Add(item)
}
