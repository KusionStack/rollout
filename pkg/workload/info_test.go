package workload

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckUpdatedReady(t *testing.T) {
	tests := []struct {
		name             string
		generation       int64
		observedGen      int64
		updatedAvailable int32
		desiredReplicas  int32
		observedReplicas int32
		replicas         int32
		strictCheck      bool
		skipToleration   int32
		expectedReady    bool
		expectedReason   string
	}{
		{
			name:             "generation mismatch returns not ready",
			generation:       2,
			observedGen:      1,
			updatedAvailable: 10,
			desiredReplicas:  100,
			observedReplicas: 100,
			replicas:         10,
			strictCheck:      false,
			skipToleration:   0,
			expectedReady:    false,
			expectedReason:   "workload Generation and ObservedGeneration are mismatched",
		},
		{
			name:             "no toleration, replicas satisfied, returns ready",
			generation:       1,
			observedGen:      1,
			updatedAvailable: 10,
			desiredReplicas:  100,
			observedReplicas: 100,
			replicas:         10,
			strictCheck:      false,
			skipToleration:   0,
			expectedReady:    true,
			expectedReason:   "",
		},
		{
			name:             "no toleration, replicas not satisfied, returns not ready",
			generation:       1,
			observedGen:      1,
			updatedAvailable: 8,
			desiredReplicas:  100,
			observedReplicas: 100,
			replicas:         10,
			strictCheck:      false,
			skipToleration:   0,
			expectedReady:    false,
			expectedReason:   "workload updated available replicas is not satisfied",
		},
		{
			name:             "toleration covers gap, not last batch, returns ready",
			generation:       1,
			observedGen:      1,
			updatedAvailable: 8,
			desiredReplicas:  100,
			observedReplicas: 100,
			replicas:         10,
			strictCheck:      false,
			skipToleration:   3,
			expectedReady:    true,
			expectedReason:   "",
		},
		{
			name:             "toleration exactly equals gap, not last batch, returns ready",
			generation:       1,
			observedGen:      1,
			updatedAvailable: 8,
			desiredReplicas:  100,
			observedReplicas: 100,
			replicas:         10,
			strictCheck:      false,
			skipToleration:   2,
			expectedReady:    true,
			expectedReason:   "",
		},
		{
			name:             "toleration insufficient, not last batch, returns not ready",
			generation:       1,
			observedGen:      1,
			updatedAvailable: 8,
			desiredReplicas:  100,
			observedReplicas: 100,
			replicas:         10,
			strictCheck:      false,
			skipToleration:   1,
			expectedReady:    false,
			expectedReason:   "workload updated available replicas is not satisfied",
		},
		{
			name:             "last batch ignores toleration, gap exists, returns not ready",
			generation:       1,
			observedGen:      1,
			updatedAvailable: 96,
			desiredReplicas:  100,
			observedReplicas: 100,
			replicas:         100,
			strictCheck:      true,
			skipToleration:   5,
			expectedReady:    false,
			expectedReason:   "workload updated available replicas is not satisfied",
		},
		{
			name:             "last batch no toleration needed, replicas satisfied, returns ready",
			generation:       1,
			observedGen:      1,
			updatedAvailable: 100,
			desiredReplicas:  100,
			observedReplicas: 100,
			replicas:         100,
			strictCheck:      true,
			skipToleration:   5,
			expectedReady:    true,
			expectedReason:   "",
		},
		{
			name:             "last batch observed replicas exceeds desired",
			generation:       1,
			observedGen:      1,
			updatedAvailable: 100,
			desiredReplicas:  100,
			observedReplicas: 105,
			replicas:         100,
			strictCheck:      true,
			skipToleration:   0,
			expectedReady:    false,
			expectedReason:   "workload observed replicas is more than desiredReplicas",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &Info{
				ObjectMeta: metav1.ObjectMeta{
					Generation: tt.generation,
				},
				Status: InfoStatus{
					ObservedGeneration:       tt.observedGen,
					UpdatedAvailableReplicas: tt.updatedAvailable,
					DesiredReplicas:          tt.desiredReplicas,
					ObservedReplicas:         tt.observedReplicas,
				},
			}
			ready, reason := info.CheckUpdatedReady(tt.replicas, tt.strictCheck, tt.skipToleration)
			if ready != tt.expectedReady {
				t.Errorf("CheckUpdatedReady() ready = %v, want %v", ready, tt.expectedReady)
			}
			if reason != tt.expectedReason {
				t.Errorf("CheckUpdatedReady() reason = %v, want %v", reason, tt.expectedReason)
			}
		})
	}
}