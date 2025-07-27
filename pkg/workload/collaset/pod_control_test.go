/**
 * Copyright 2025 The KusionStack Authors
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

package collaset

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kusionstackappsv1 "kusionstack.io/kube-api/apps/v1alpha1"
)

func TestRecognizeRevision(t *testing.T) {
	// Test cases where workload is StatefulSet and obj is Pod
	tests := []struct {
		name        string
		sts         *kusionstackappsv1.CollaSet
		pod         *corev1.Pod // Input Pod
		wantCurrent bool        // Expected isCurrent result
		wantUpdated bool        // Expected isUpdated result
		wantErr     bool        // Expected error
	}{
		{
			name: "pod with current revision label",
			sts: &kusionstackappsv1.CollaSet{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: kusionstackappsv1.CollaSetStatus{
					CurrentRevision:    "current-rev",
					UpdatedRevision:    "update-rev",
					ObservedGeneration: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						appsv1.ControllerRevisionHashLabelKey: "current-rev",
					},
				},
			},
			wantCurrent: true,
			wantUpdated: false,
			wantErr:     false,
		},
		{
			name: "pod with update revision label and matching generation",
			sts: &kusionstackappsv1.CollaSet{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: kusionstackappsv1.CollaSetStatus{
					CurrentRevision:    "current-rev",
					UpdatedRevision:    "update-rev",
					ObservedGeneration: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						appsv1.ControllerRevisionHashLabelKey: "update-rev",
					},
				},
			},
			wantCurrent: false,
			wantUpdated: true,
			wantErr:     false,
		},
		{
			name: "pod with no revision label",
			sts: &kusionstackappsv1.CollaSet{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: kusionstackappsv1.CollaSetStatus{
					CurrentRevision:    "current-rev",
					UpdatedRevision:    "update-rev",
					ObservedGeneration: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			wantCurrent: false,
			wantUpdated: false,
			wantErr:     false,
		},
		{
			name: "pod with update revision label but generation mismatch",
			sts: &kusionstackappsv1.CollaSet{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Status: kusionstackappsv1.CollaSetStatus{
					CurrentRevision:    "current-rev",
					UpdatedRevision:    "update-rev",
					ObservedGeneration: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						appsv1.ControllerRevisionHashLabelKey: "update-rev",
					},
				},
			},
			wantCurrent: false,
			wantUpdated: false,
			wantErr:     false,
		},
		{
			name: "pod with unknown revision label",
			sts: &kusionstackappsv1.CollaSet{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: kusionstackappsv1.CollaSetStatus{
					CurrentRevision:    "current-rev",
					UpdatedRevision:    "update-rev",
					ObservedGeneration: 1,
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						appsv1.ControllerRevisionHashLabelKey: "unknown-rev",
					},
				},
			},
			wantCurrent: false,
			wantUpdated: false,
			wantErr:     false,
		},
	}

	a := &accessorImpl{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isCurrent, isUpdated, err := a.RecognizeRevision(context.Background(), nil, tt.sts, tt.pod)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantCurrent, isCurrent)
			assert.Equal(t, tt.wantUpdated, isUpdated)
		})
	}
}
