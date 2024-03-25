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

type Set struct {
	set  map[string]map[string]int
	list []*Info
}

func NewSet(workloads ...*Info) *Set {
	s := &Set{
		set:  make(map[string]map[string]int),
		list: make([]*Info, 0),
	}
	for _, w := range workloads {
		s.add(w)
	}
	return s
}

func (s *Set) ToSlice() []*Info {
	return s.list
}

func (s *Set) add(info *Info) {
	_, ok := s.set[info.ClusterName]
	if !ok {
		s.set[info.ClusterName] = map[string]int{}
	}

	s.list = append(s.list, info)
	s.set[info.ClusterName][info.Name] = len(s.list) - 1
}

func (s *Set) Get(cluster, name string) *Info {
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
