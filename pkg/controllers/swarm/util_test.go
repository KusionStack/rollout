package swarm

import (
	"reflect"
	"testing"
)

func Test_treeWayMergeMap(t *testing.T) {
	tests := []struct {
		name     string
		original map[string]string
		modified map[string]string
		current  map[string]string
		want     map[string]string
	}{
		{
			original: map[string]string{
				"a": "a",
			},
			modified: map[string]string{
				"a": "b",
			},
			current: nil,
			want: map[string]string{
				"a": "b",
			},
		},
		{
			original: map[string]string{
				"a": "a",
			},
			modified: map[string]string{
				"a": "a",
				"b": "b",
			},
			current: nil,
			want: map[string]string{
				"a": "a",
				"b": "b",
			},
		},
		{
			original: map[string]string{
				"a": "a",
			},
			modified: map[string]string{
				"b": "b",
			},
			current: map[string]string{
				"a": "a",
				"b": "x",
				"c": "c",
			},
			want: map[string]string{
				"b": "b",
				"c": "c",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := treeWayMergeMap(tt.original, tt.modified, tt.current); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("treeWayMergeMap() = %v, want %v", got, tt.want)
			}
		})
	}
}
