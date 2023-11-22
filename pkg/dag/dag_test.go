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

package dag_test

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"kusionstack.io/rollout/apis/workflow/v1alpha1"
	"kusionstack.io/rollout/pkg/dag"
)

func testDAG1(t *testing.T) *dag.DAG {
	t.Helper()

	tasks := []v1alpha1.WorkflowTask{
		{
			Name: "a",
		},
		{
			Name: "b",
		},
		{
			Name: "c",
		},
		{
			Name:     "d",
			RunAfter: []string{"a"},
		},
		{
			Name:     "e",
			RunAfter: []string{"b"},
		},
		{
			Name:     "f",
			RunAfter: []string{"c"},
		},
		{
			Name:     "g",
			RunAfter: []string{"d", "e", "f"},
		},
		{
			Name:     "h",
			RunAfter: []string{"g"},
		},
		{
			Name:     "i",
			RunAfter: []string{"g"},
		},
	}
	d, err := dag.Build(v1alpha1.WorkflowTaskList(tasks), v1alpha1.WorkflowTaskList(tasks).Deps())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func testDAG2(t *testing.T) *dag.DAG {
	t.Helper()

	//  b     a
	//  |    / \
	//  |   |   x
	//  |   | / |
	//  |   y   |
	//   \ /    z
	//    w
	tasks := []v1alpha1.WorkflowTask{
		{
			Name: "a",
		},
		{
			Name: "b",
		},
		{
			Name:     "x",
			RunAfter: []string{"a"},
		},
		{
			Name:     "y",
			RunAfter: []string{"a", "x"},
		},
		{
			Name:     "z",
			RunAfter: []string{"x"},
		},
		{
			Name:     "w",
			RunAfter: []string{"y", "b"},
		},
	}
	d, err := dag.Build(v1alpha1.WorkflowTaskList(tasks), v1alpha1.WorkflowTaskList(tasks).Deps())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDAG_GetCandidateTasks(t *testing.T) {
	type fields struct {
		Nodes map[string]*dag.Node
	}
	type args struct {
		completedTasks []string
	}
	dag1 := testDAG1(t)
	dag2 := testDAG2(t)
	tests := []struct {
		name   string
		fields fields
		args   args
		want   sets.String
	}{
		{
			name: "dag1-all-done",
			fields: fields{
				Nodes: dag1.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
			},
			want: sets.NewString(),
		},
		{
			name: "dag1-a-done",
			fields: fields{
				Nodes: dag1.Nodes,
			},
			args: args{
				completedTasks: []string{"a"},
			},
			want: sets.NewString("b", "c", "d"),
		},
		{
			name: "dag1-abc-done",
			fields: fields{
				Nodes: dag1.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "b", "c"},
			},
			want: sets.NewString("d", "e", "f"),
		},
		{
			name: "dag1-abcde-done",
			fields: fields{
				Nodes: dag1.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "b", "c", "d", "e"},
			},
			want: sets.NewString("f"),
		},
		{
			name: "dag1-abcdef-done",
			fields: fields{
				Nodes: dag1.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "b", "c", "d", "e", "f"},
			},
			want: sets.NewString("g"),
		},
		{
			name: "dag1-abcdefg-done",
			fields: fields{
				Nodes: dag1.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "b", "c", "d", "e", "f", "g"},
			},
			want: sets.NewString("h", "i"),
		},
		{
			name: "dag2-ab-done",
			fields: fields{
				Nodes: dag2.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "b"},
			},
			want: sets.NewString("x"),
		},
		{
			name: "dag2-nothing-done",
			fields: fields{
				Nodes: dag2.Nodes,
			},
			args: args{
				completedTasks: []string{},
			},
			want: sets.NewString("a", "b"),
		},
		{
			name: "dag2-a-done",
			fields: fields{
				Nodes: dag2.Nodes,
			},
			args: args{
				completedTasks: []string{"a"},
			},
			want: sets.NewString("b", "x"),
		},
		{
			name: "dag2-b-done",
			fields: fields{
				Nodes: dag2.Nodes,
			},
			args: args{
				completedTasks: []string{"b"},
			},
			want: sets.NewString("a"),
		},
		{
			name: "dag2-ab-done",
			fields: fields{
				Nodes: dag2.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "b"},
			},
			want: sets.NewString("x"),
		},
		{
			name: "dag2-ax-done",
			fields: fields{
				Nodes: dag2.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "x"},
			},
			want: sets.NewString("b", "y", "z"),
		},
		{
			name: "dag2-axb-done",
			fields: fields{
				Nodes: dag2.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "x", "b"},
			},
			want: sets.NewString("y", "z"),
		},
		{
			name: "dag2-axy-done",
			fields: fields{
				Nodes: dag2.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "x", "y"},
			},
			want: sets.NewString("b", "z"),
		},
		{
			name: "dag2-axyb-done",
			fields: fields{
				Nodes: dag2.Nodes,
			},
			args: args{
				completedTasks: []string{"a", "x", "y", "b"},
			},
			want: sets.NewString("w", "z"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &dag.DAG{
				Nodes: tt.fields.Nodes,
			}
			if got := d.GetCandidateTasks(tt.args.completedTasks); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetCandidateTasks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuild(t *testing.T) {
	type args struct {
		tasks v1alpha1.WorkflowTaskList
	}
	tests := []struct {
		name    string
		args    args
		want    *dag.DAG
		wantErr bool
	}{
		{
			// a
			// |
			// a
			name: "self-link-cycle",
			args: args{
				tasks: v1alpha1.WorkflowTaskList{
					{
						Name:     "a",
						RunAfter: []string{"a"},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			//   a
			//  / \
			// b - c
			name: "cycle-a-b-c-a",
			args: args{
				tasks: v1alpha1.WorkflowTaskList{
					{
						Name:     "a",
						RunAfter: []string{"c"},
					},
					{
						Name:     "b",
						RunAfter: []string{"a"},
					},
					{
						Name:     "c",
						RunAfter: []string{"b"},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			//   a
			//  / \
			// b   c
			//  \ /
			//   d   \
			//   |
			//   e --  c
			name: "cycle-c-d-e-c",
			args: args{
				tasks: v1alpha1.WorkflowTaskList{
					{
						Name: "a",
					},
					{
						Name:     "b",
						RunAfter: []string{"a"},
					},
					{
						Name:     "c",
						RunAfter: []string{"a", "e"},
					},
					{
						Name:     "d",
						RunAfter: []string{"b", "c"},
					},
					{
						Name:     "e",
						RunAfter: []string{"d"},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "duplicate-task",
			args: args{
				tasks: v1alpha1.WorkflowTaskList{
					{
						Name: "a",
					},
					{
						Name: "a",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "runAfter-non-existent-task",
			args: args{
				tasks: v1alpha1.WorkflowTaskList{
					{
						Name:     "a",
						RunAfter: []string{"x"},
					},
					{
						Name: "b",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "valid-dag1",
			args: args{
				tasks: v1alpha1.WorkflowTaskList{
					{
						Name: "a",
					},
					{
						Name: "b",
					},
					{
						Name: "c",
					},
					{
						Name:     "d",
						RunAfter: []string{"a"},
					},
					{
						Name:     "e",
						RunAfter: []string{"b"},
					},
					{
						Name:     "f",
						RunAfter: []string{"c"},
					},
					{
						Name:     "g",
						RunAfter: []string{"d", "e", "f"},
					},
					{
						Name:     "h",
						RunAfter: []string{"g"},
					},
					{
						Name:     "i",
						RunAfter: []string{"g"},
					},
				},
			},
			want:    testDAG1(t),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dag.Build(tt.args.tasks, tt.args.tasks.Deps())
			if (err != nil) != tt.wantErr {
				t.Errorf("Build() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assertSameDAG(t, got, tt.want)
		})
	}
}

// assertSameDAG asserts that the two DAGs are the same.
func assertSameDAG(t *testing.T, l, r *dag.DAG) {
	//t.Helper()

	if l == nil && r == nil {
		return
	}

	if l == nil || r == nil {
		t.Errorf("one of the DAGs is nil")
		return
	}

	// Check that the nodes are the same.
	if len(l.Nodes) != len(r.Nodes) {
		t.Errorf("different number of nodes: %d != %d", len(l.Nodes), len(r.Nodes))
	}
	lKeys, rKeys := make([]string, 0, len(l.Nodes)), make([]string, 0, len(r.Nodes))
	for key := range l.Nodes {
		lKeys = append(lKeys, key)
	}
	for key := range r.Nodes {
		rKeys = append(rKeys, key)
	}
	sort.Strings(lKeys)
	sort.Strings(rKeys)
	if !reflect.DeepEqual(lKeys, rKeys) {
		t.Errorf("different node keys: %v != %v", lKeys, rKeys)
	}

	// Check that the edges are the same.
	for key, lNode := range l.Nodes {
		rNode := r.Nodes[key]
		if err := assertSameEdges(lNode.Prev, rNode.Prev); err != nil {
			t.Errorf("different prev edges for node %s: %v", key, err)
		}
		if err := assertSameEdges(lNode.Next, rNode.Next); err != nil {
			t.Errorf("different next edges for node %s: %v", key, err)
		}
	}
}

// assertSameEdges asserts that the two sets of edges are the same.
func assertSameEdges(l, r []*dag.Node) error {
	if len(l) != len(r) {
		return fmt.Errorf("different number of edges: %d != %d", len(l), len(r))
	}
	lKeys, rKeys := make([]string, 0, len(l)), make([]string, 0, len(r))
	for _, node := range l {
		lKeys = append(lKeys, node.Key)
	}
	for _, node := range r {
		rKeys = append(rKeys, node.Key)
	}
	sort.Strings(lKeys)
	sort.Strings(rKeys)
	if !reflect.DeepEqual(lKeys, rKeys) {
		return fmt.Errorf("different edge keys: %v != %v", lKeys, rKeys)
	}
	return nil
}
