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
	"testing"
)

func TestCalculateExpectedPartition(t *testing.T) {
	tests := []struct {
		name             string
		total            int32
		expectedReplicas int32
		partitionInSpec  int32
		want             int32
		wantErr          bool
	}{
		{
			name:             "total 10, current partition 10, want to update 1",
			total:            10,
			expectedReplicas: 1,
			partitionInSpec:  10,
			want:             9,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 5, want to update 1",
			total:            10,
			expectedReplicas: 1,
			partitionInSpec:  5,
			want:             5,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 0, want to update 1",
			total:            10,
			expectedReplicas: 1,
			partitionInSpec:  0,
			want:             0,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 15, want to update 0",
			total:            10,
			expectedReplicas: 0,
			partitionInSpec:  15,
			want:             15,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 15, want to update 1",
			total:            10,
			expectedReplicas: 1,
			partitionInSpec:  15,
			want:             9,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 0, want to update 15",
			total:            10,
			expectedReplicas: 15,
			partitionInSpec:  0,
			want:             0,
			wantErr:          false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateExpectedPartition(&tt.total, tt.expectedReplicas, tt.partitionInSpec)
			if got != tt.want {
				t.Errorf("CalculateExpectedPartition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculatexProgressingPartition(t *testing.T) {
	tests := []struct {
		name             string
		total            int32
		expectedReplicas int32
		partitionInSpec  int32
		want             int32
		wantErr          bool
	}{
		{
			name:             "total 10, current partition 0, want to update 1",
			total:            10,
			expectedReplicas: 1,
			partitionInSpec:  0,
			want:             1,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 5, want to update 1",
			total:            10,
			expectedReplicas: 1,
			partitionInSpec:  5,
			want:             5,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 10, want to update 1",
			total:            10,
			expectedReplicas: 1,
			partitionInSpec:  10,
			want:             10,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 15, want to update 0",
			total:            10,
			expectedReplicas: 0,
			partitionInSpec:  15,
			want:             15,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 0, want to update 15",
			total:            10,
			expectedReplicas: 15,
			partitionInSpec:  0,
			want:             10,
			wantErr:          false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateProgressingPartition(&tt.total, tt.expectedReplicas, tt.partitionInSpec)
			if got != tt.want {
				t.Errorf("CalculateExpectedPartition() = %v, want %v", got, tt.want)
			}
		})
	}
}
