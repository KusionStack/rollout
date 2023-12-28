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

package rollout

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/storage/names"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/apis/rollout/v1alpha1/condition"
)

func generateRolloutID(name string) string {
	prefix := name
	if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}
	return names.SimpleNameGenerator.GenerateName(prefix)
}

func resetRolloutStatus(status *rolloutv1alpha1.RolloutStatus, rolloutID string, phase rolloutv1alpha1.RolloutPhase) {
	// clean all existing status
	status.RolloutID = rolloutID
	status.Phase = phase
	status.BatchStatus = nil
	status.Conditions = []rolloutv1alpha1.Condition{}
}

func isRolloutRunCompleted(run *rolloutv1alpha1.RolloutRun) bool {
	return run.Status.Phase == rolloutv1alpha1.RolloutRunPhaseSucceeded || run.Status.Phase == rolloutv1alpha1.RolloutRunPhaseCanceled
}

func setStatusCondition(newStatus *rolloutv1alpha1.RolloutStatus, ctype rolloutv1alpha1.ConditionType, status metav1.ConditionStatus, reason, message string) {
	cond := condition.NewCondition(ctype, status, reason, message)
	newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *cond)
}
