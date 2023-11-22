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

package dag

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
)

// Task is an interface for all types that could be in a DAG
type Task interface {
	HashKey() string
	Deps() []string
}

// Tasks is an interface for lists of types that could be in a DAG
type Tasks interface {
	Items() []Task
}

// DAG represents a directed acyclic graph
type DAG struct {
	Nodes map[string]*Node
}

// Node represents a node in a DAG
type Node struct {
	Key  string
	Prev []*Node
	Next []*Node
}

// NewDAG returns an empty DAG
func NewDAG() *DAG {
	return &DAG{Nodes: map[string]*Node{}}
}

// Build returns a valid DAG. Returns error if the DAG is invalid
func Build(tasks Tasks, deps map[string][]string) (*DAG, error) {
	dag := NewDAG()

	// add tasks to DAG, and ensure no duplicate tasks
	for _, task := range tasks.Items() {
		if _, err := dag.addTask(task); err != nil {
			return nil, fmt.Errorf("task %s is already present in DAG, can't add it again: %w", task.HashKey(), err)
		}
	}

	// ensure no cycles in dependencies
	if err := findCyclesInDependencies(deps); err != nil {
		return nil, fmt.Errorf("cycle detected; %w", err)
	}

	// add task dependency edges
	for taskKey, taskDeps := range deps {
		taskNode, ok := dag.Nodes[taskKey]
		if !ok {
			return nil, fmt.Errorf("task %s not found in DAG", taskKey)
		}
		for _, taskDep := range taskDeps {
			taskDepNode, ok := dag.Nodes[taskDep]
			if !ok {
				return nil, fmt.Errorf("task %s not found in DAG", taskDep)
			}
			taskNode.Prev = append(taskNode.Prev, taskDepNode)
			taskDepNode.Next = append(taskDepNode.Next, taskNode)
		}
	}
	return dag, nil
}

func findCyclesInDependencies(deps map[string][]string) error {
	visited := sets.NewString()
	for taskKey := range deps {
		if err := dfs(taskKey, visited, deps); err != nil {
			return err
		}
	}
	return nil
}

// dfs performs a depth-first search on the DAG to detect cycles
func dfs(key string, visited sets.String, deps map[string][]string) error {
	if visited.Has(key) {
		return fmt.Errorf("cycle detected at %s", key)
	}
	visited.Insert(key)
	for _, dep := range deps[key] {
		if err := dfs(dep, visited, deps); err != nil {
			return err
		}
	}
	visited.Delete(key)
	return nil
}

// addTask adds a task to the DAG, and ensures that there's no duplicate task
func (dag *DAG) addTask(task Task) (*Node, error) {
	if _, ok := dag.Nodes[task.HashKey()]; ok {
		return nil, errors.New("duplicate task")
	}
	newNode := &Node{
		Key: task.HashKey(),
	}
	dag.Nodes[task.HashKey()] = newNode
	return newNode, nil
}

func (dag *DAG) GetCandidateTasks(completedTasks []string) sets.String {
	roots := dag.getRoots()
	visited, candidates := sets.NewString(), sets.NewString()

	for _, root := range roots {
		bfs(root, visited, candidates, sets.NewString(completedTasks...))
	}
	return candidates
}

// bfs performs a breadth-first search on the DAG to find candidate tasks
func bfs(root *Node, visited, candidates, completedTasks sets.String) {
	queue := []*Node{root}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if visited.Has(node.Key) {
			continue
		}
		visited.Insert(node.Key)

		if !completedTasks.Has(node.Key) {
			if prevTasksCompleted(node, completedTasks) {
				candidates.Insert(node.Key)
			}
			continue
		}

		queue = append(queue, node.Next...)
	}
}

func prevTasksCompleted(node *Node, completedTasks sets.String) bool {
	for _, prev := range node.Prev {
		if !completedTasks.Has(prev.Key) {
			return false
		}
	}
	return true
}

func (dag *DAG) getRoots() []*Node {
	var roots []*Node
	for _, node := range dag.Nodes {
		if len(node.Prev) == 0 {
			roots = append(roots, node)
		}
	}
	return roots
}
