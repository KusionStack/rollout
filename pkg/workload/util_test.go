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

	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCalculateExpectedPartition(t *testing.T) {
	tests := []struct {
		name             string
		total            int32
		expectedReplicas intstr.IntOrString
		partitionInSpec  int32
		want             int32
		wantErr          bool
	}{
		{
			name:             "total 10, current partition 10, want to update 1",
			total:            10,
			expectedReplicas: intstr.FromInt(1),
			partitionInSpec:  10,
			want:             9,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 5, want to update 1",
			total:            10,
			expectedReplicas: intstr.FromInt(1),
			partitionInSpec:  5,
			want:             5,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 0, want to update 1",
			total:            10,
			expectedReplicas: intstr.FromInt(1),
			partitionInSpec:  0,
			want:             0,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 15, want to update 0",
			total:            10,
			expectedReplicas: intstr.FromInt(0),
			partitionInSpec:  15,
			want:             15,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 15, want to update 1",
			total:            10,
			expectedReplicas: intstr.FromInt(1),
			partitionInSpec:  15,
			want:             9,
			wantErr:          false,
		},
		{
			name:             "total 10, current partition 15, want to update 1",
			total:            10,
			expectedReplicas: intstr.FromInt(1),
			partitionInSpec:  15,
			want:             9,
			wantErr:          false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateExpectedPartition(&tt.total, tt.expectedReplicas, tt.partitionInSpec)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateExpectedPartition() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CalculateExpectedPartition() = %v, want %v", got, tt.want)
			}
		})
	}
}
