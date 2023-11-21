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

package workload

import (
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

type Set struct {
	set  map[string]map[string]int
	list []Interface
}

func NewWorkloadSet(workloads ...Interface) *Set {
	s := &Set{
		set:  make(map[string]map[string]int),
		list: make([]Interface, 0),
	}
	for _, w := range workloads {
		s.add(w)
	}
	return s
}

func (s *Set) ToSlice() []Interface {
	return s.list
}

func (s *Set) Matches(match *rolloutv1alpha1.ResourceMatch) []Interface {
	if match == nil || (match.Selector == nil && len(match.Names) == 0) {
		// match all
		return s.list
	}

	matcher := MatchAsMatcher(*match)
	result := make([]Interface, 0)
	for i := range s.list {
		w := s.list[i]
		info := w.GetInfo()
		if matcher.Matches(info.Cluster, info.Name, info.Labels) {
			result = append(result, w)
		}
	}
	return result
}

func (s *Set) add(in Interface) {
	info := in.GetInfo()

	_, ok := s.set[info.Cluster]
	if !ok {
		s.set[info.Cluster] = map[string]int{}
	}

	s.list = append(s.list, in)
	s.set[info.Cluster][info.Name] = len(s.list) - 1
}

func (s *Set) Get(cluster, name string) Interface {
	clusterSet, ok := s.set[cluster]
	if !ok {
		return nil
	}
	index, ok := clusterSet[name]
	if !ok {
		return nil
	}
	return s.list[index]
}
