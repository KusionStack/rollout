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

package workload

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

type Matcher interface {
	Matches(cluster, name string, label map[string]string) bool
}

type matcherImpl struct {
	selector labels.Selector
	refs     []rolloutv1alpha1.CrossClusterObjectNameReference
}

func MatchAsMatcher(match rolloutv1alpha1.ResourceMatch) Matcher {
	var selector labels.Selector
	if match.Selector != nil {
		selector, _ = metav1.LabelSelectorAsSelector(match.Selector)
	}

	return &matcherImpl{
		selector: selector,
		refs:     match.Names,
	}
}

func (m *matcherImpl) Matches(cluster, name string, label map[string]string) bool {
	if m.selector != nil {
		return m.selector.Matches(labels.Set(label))
	}
	for _, ref := range m.refs {
		if ref.Matches(cluster, name) {
			return true
		}
	}
	return false
}
