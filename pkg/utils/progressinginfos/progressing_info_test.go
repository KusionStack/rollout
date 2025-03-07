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

package progressinginfos

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

func newTestProgressingInfo(kind string, name string, id string) rolloutv1alpha1.ProgressingInfo {
	return rolloutv1alpha1.ProgressingInfo{
		Kind:        kind,
		RolloutName: name,
		RolloutID:   id,
	}
}

func newTestProgressingInfoWithIndex(kind string, name string, id string, index int32) rolloutv1alpha1.ProgressingInfo {
	return rolloutv1alpha1.ProgressingInfo{
		Kind:        kind,
		RolloutName: name,
		RolloutID:   id,
		Batch: &rolloutv1alpha1.BatchProgressingInfo{
			CurrentBatchIndex: index,
		},
	}
}

func TestMergeSliceByKey(t *testing.T) {
	tests := []struct {
		name      string
		existings ProgressingInfos
		news      ProgressingInfos
		want      ProgressingInfos
	}{
		{
			name: "merge",
			existings: ProgressingInfos{
				newTestProgressingInfo("Rollout", "rollout-1", "rollout-id-1"),
			},
			news: ProgressingInfos{
				newTestProgressingInfo("Rollout", "rollout-2", "rollout-id-2"),
			},
			want: ProgressingInfos{
				newTestProgressingInfo("Rollout", "rollout-1", "rollout-id-1"),
				newTestProgressingInfo("Rollout", "rollout-2", "rollout-id-2"),
			},
		},
		{
			name: "remain old info when rollout id is not changed",
			existings: ProgressingInfos{
				newTestProgressingInfo("Rollout", "rollout-1", "rollout-id-1"),
				newTestProgressingInfo("Rollout", "rollout-2", "rollout-id-2"),
			},
			news: ProgressingInfos{
				newTestProgressingInfoWithIndex("Rollout", "rollout-2", "rollout-id-2", 1),
			},
			want: ProgressingInfos{
				newTestProgressingInfo("Rollout", "rollout-1", "rollout-id-1"),
				newTestProgressingInfo("Rollout", "rollout-2", "rollout-id-2"),
			},
		},
		{
			name: "replace",
			existings: ProgressingInfos{
				newTestProgressingInfo("Rollout", "rollout-1", "rollout-id-1"),
				newTestProgressingInfo("Rollout", "rollout-2", "rollout-id-2"),
			},
			news: ProgressingInfos{
				newTestProgressingInfoWithIndex("Rollout", "rollout-2", "rollout-id-2-2", 1),
			},
			want: ProgressingInfos{
				newTestProgressingInfo("Rollout", "rollout-1", "rollout-id-1"),
				newTestProgressingInfoWithIndex("Rollout", "rollout-2", "rollout-id-2-2", 1),
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got := mergeProgressingInfos(tt.existings, tt.news)
			sort.Sort(got)
			sort.Sort(tt.want)
			assert.EqualValues(t, tt.want, got)
		})
	}
}
