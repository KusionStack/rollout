package executor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rolloutapis "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
	fakev1alpha1 "kusionstack.io/rollout/pkg/workload/fake"
)

const (
	reqKeySleepSeconds = "sleepSeconds"
	reqKeyResponseBody = "ResponseBody"
	reqKeyResponseCode = "ResponseCode"
)

var (
	apiVersion = schema.GroupVersion{
		Group: fakev1alpha1.GVK.Group, Version: fakev1alpha1.GVK.Version,
	}

	testRollout = rolloutv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-rollout",
			Namespace:   metav1.NamespaceDefault,
			UID:         types.UID(uuid.New().String()),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: rolloutv1alpha1.RolloutSpec{},
	}

	testRolloutRun = rolloutv1alpha1.RolloutRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-rolloutrun",
			Namespace:   metav1.NamespaceDefault,
			UID:         types.UID(uuid.New().String()),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: rolloutv1alpha1.RolloutRunSpec{
			TargetType: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: apiVersion.String(), Kind: fakev1alpha1.GVK.Kind,
			},
			Webhooks: []rolloutv1alpha1.RolloutWebhook{},
			Batch: &rolloutv1alpha1.RolloutRunBatchStrategy{
				Toleration: &rolloutv1alpha1.TolerationStrategy{},
				Batches:    []rolloutv1alpha1.RolloutRunStep{},
			},
		},
		Status: rolloutv1alpha1.RolloutRunStatus{
			Conditions: []rolloutv1alpha1.Condition{},
		},
	}
)

func newTestLogger() logr.Logger {
	return zap.New(zap.UseDevMode(true), zap.ConsoleEncoder())
}

// func newTestExecutor() *Executor {
// 	return &Executor{
// 		logger: newTestLogger(),
// 	}
// }

func createTestExecutorContext(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun, workloads *workload.Set) *ExecutorContext {
	if workloads == nil {
		workloads = workload.NewWorkloadSet()
	}
	ctx := &ExecutorContext{
		Rollout:    rollout,
		RolloutRun: rolloutRun,
		Workloads:  workloads,
		NewStatus:  rolloutRun.Status.DeepCopy(),
	}
	ctx.Initialize()
	return ctx
}

type testCase struct {
	name                string
	checkResult         checkResult
	makeExecutorContext makeExecutorContext
}

type checkResult func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error)

type makeExecutorContext func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext

func runTestCase(t *testing.T, cases []testCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer GinkgoRecover()
			executor := NewDefaultExecutor(zap.New(
				zap.WriteTo(os.Stdout), zap.UseDevMode(true),
			))

			newRollout := testRollout.DeepCopy()
			newRolloutRun := testRolloutRun.DeepCopy()
			done, result, err := executor.Do(tc.makeExecutorContext(newRollout, newRolloutRun))

			expect, err := tc.checkResult(done, result, err, newRolloutRun)
			Expect(err).NotTo(HaveOccurred())
			Expect(expect).Should(BeTrue())
		})
	}
}

// makeHandlerFunc mock http server
func makeHandlerFunc() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/webhook" {
			http.Error(writer, "NotFound", http.StatusNotFound)
		}

		query := request.URL.Query()

		sleepSeconds := query.Get(reqKeySleepSeconds)
		if len(sleepSeconds) > 0 {
			if val, err := strconv.Atoi(sleepSeconds); err == nil {
				time.Sleep(time.Duration(val) * time.Second)
			} else {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		body, err := io.ReadAll(request.Body)
		defer request.Body.Close()
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		var review = &rolloutv1alpha1.RolloutWebhookReview{}
		err = json.Unmarshal(body, &review)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", "application/json")

		responseCode := query.Get(reqKeyResponseCode)
		if len(responseCode) <= 0 {
			writer.WriteHeader(200)
		} else if val, err := strconv.Atoi(responseCode); err == nil {
			writer.WriteHeader(val)
		} else {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		if val, exist := review.Spec.Properties[reqKeyResponseBody]; exist {
			if _, err = writer.Write([]byte(val)); err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
			}
		} else {
			if _, err = writer.Write([]byte("{\"foo\":\"bar\"}")); err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
			}
		}
	}
}

func TestDo(t *testing.T) {
	RegisterFailHandler(Fail)

	testcases := []testCase{
		{
			name: "Input={ProgressingStatus==nil}, Context={}, Output={ProgressingStatus.State==Initial}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}
				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 ||
					len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhasePreRollout {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Initial}, Context={}, Output={ProgressingStatus.State==PreRollout}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseInitial
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 ||
					len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhasePreRollout {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Initial, ProgressingStatus.Error!=nil}, Context={}, Output={ProgressingStatus.State==Initial}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseInitial
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				rolloutRun.Status.Error = &rolloutv1alpha1.CodeReasonMessage{
					Code:   "PreBatchStepHookError",
					Reason: "WebhookFailureThresholdExceeded",
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error == nil ||
					len(newBatchStatus.Records) != 0 ||
					len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseInitial {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Initial, ProgressingStatus.Error!=nil}, Context={DeletionTimestamp is not nil}, Output={ProgressingStatus.State==Canceled}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.DeletionTimestamp = ptr.To(metav1.Now())
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseInitial
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 ||
					len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseCanceling {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Paused}, Context={}, Output={ProgressingStatus.State==Paused}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhasePaused
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 ||
					len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhasePaused {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Canceling}, Context={}, Output={ProgressingStatus.State==Completed}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 ||
					len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseCanceled {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Completed}, Context={}, Output={ProgressingStatus.State==Completed}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseSucceeded
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if !done || result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 ||
					len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseSucceeded {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==PreRollout}, Context={}, Output={ProgressingStatus.State==Rolling}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhasePreRollout
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 ||
					len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseProgressing {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Rolling}, Context={}, Output={ProgressingStatus.State==PostRollout}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhasePostRollout ||
					newBatchStatus.RolloutBatchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePending {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==PostRollout}, Context={}, Output={ProgressingStatus.State==Completed}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if !done || result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 ||
					len(newBatchStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseSucceeded {
					return false, nil
				}
				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoCommand(t *testing.T) {
	RegisterFailHandler(Fail)

	ts := httptest.NewServer(makeHandlerFunc())
	defer ts.Close()

	testcases := []testCase{
		{
			name: "Input={CurrentBatchState==Paused}, Context={command=Resume}, Output={CurrentBatchState==PreBatchStepHook}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandContinue
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				url := fmt.Sprintf("%s/webhook?%s=200", ts.URL, reqKeyResponseCode)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.PreBatchStepHook, rolloutv1alpha1.PostBatchStepHook},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1, CurrentBatchState: BatchStatePaused,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:      BatchStateSucceeded,
							StartTime:  &metav1.Time{Time: time.Now()},
							FinishTime: &metav1.Time{Time: time.Now()},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{
									Name:              "wh-01",
									FailureCount:      2,
									HookType:          rolloutv1alpha1.PreBatchStepHook,
									CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: "Ok", Reason: "Success"},
								},
							},
						},
						{
							StartTime: &metav1.Time{Time: time.Now()},
							State:     BatchStatePaused,
						},
					},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 2 ||
					newBatchStatus.CurrentBatchState != BatchStatePreBatchHook ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseProgressing {
					return false, nil
				}

				batchStatus := newBatchStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newBatchStatus.Records[1].StartTime == nil ||
					newBatchStatus.Records[1].State != BatchStatePreBatchHook {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.Error!=nil}, Context={command=Retry}, Output={ProgressingStatus.Error==nil}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandRetry
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:     BatchStatePreBatchHook,
							StartTime: &metav1.Time{Time: time.Now()},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{
									Name:              "wh-01",
									FailureCount:      2,
									HookType:          rolloutv1alpha1.PreBatchStepHook,
									CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: "Ok", Reason: "Success"},
								},
							},
						},
					},
				}
				rolloutRun.Status.Error = &rolloutv1alpha1.CodeReasonMessage{
					Code:   "PreBatchStepHookError",
					Reason: "WebhookFailureThresholdExceeded",
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 1 ||
					newBatchStatus.CurrentBatchState != BatchStatePreBatchHook ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseProgressing {
					return false, nil
				}

				batchStatus := newBatchStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newBatchStatus.Records[0].StartTime == nil ||
					newBatchStatus.Records[0].State != BatchStatePreBatchHook {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Rolling}, Context={command=Pause}, Output={ProgressingStatus.State==Paused}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandPause
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				url := fmt.Sprintf("%s/webhook?%s=200", ts.URL, reqKeyResponseCode)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.PreBatchStepHook, rolloutv1alpha1.PostBatchStepHook},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1, CurrentBatchState: BatchStatePaused,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:      BatchStateSucceeded,
							StartTime:  &metav1.Time{Time: time.Now()},
							FinishTime: &metav1.Time{Time: time.Now()},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{
									Name:              "wh-01",
									FailureCount:      2,
									HookType:          rolloutv1alpha1.PreBatchStepHook,
									CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: "Ok", Reason: "Success"},
								},
							},
						},
						{
							StartTime: &metav1.Time{Time: time.Now()}, State: BatchStatePaused,
						},
					},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 2 ||
					newBatchStatus.RolloutBatchStatus.CurrentBatchState != BatchStatePaused ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhasePausing {
					return false, nil
				}

				batchStatus := newBatchStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStatePaused {
					return false, nil
				}

				if newBatchStatus.Records[1].StartTime == nil ||
					newBatchStatus.Records[1].State != BatchStatePaused {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Rolling}, Context={command=Cancel}, Output={ProgressingStatus.State==Canceling}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandCancel
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				url := fmt.Sprintf("%s/webhook?%s=200", ts.URL, reqKeyResponseCode)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.PreBatchStepHook, rolloutv1alpha1.PostBatchStepHook},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1, CurrentBatchState: BatchStatePaused,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:      BatchStateSucceeded,
							StartTime:  &metav1.Time{Time: time.Now()},
							FinishTime: &metav1.Time{Time: time.Now()},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{
									Name:              "wh-01",
									FailureCount:      2,
									HookType:          rolloutv1alpha1.PreBatchStepHook,
									CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: "Ok", Reason: "Success"},
								},
							},
						},
						{
							StartTime: &metav1.Time{Time: time.Now()},
							State:     BatchStatePaused,
						},
					},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 2 ||
					newBatchStatus.CurrentBatchState != BatchStatePaused ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseCanceling {
					return false, nil
				}

				batchStatus := newBatchStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStatePaused {
					return false, nil
				}

				if newBatchStatus.Records[1].StartTime == nil ||
					newBatchStatus.Records[1].State != BatchStatePaused {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==2, currentBatchIndex=0, ProgressingStatus.Error!=nil}, Context={command=Skip}, Output={CurrentBatchState=2}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandSkip
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()}}, {}},
				}
				rolloutRun.Status.Error = &rolloutv1alpha1.CodeReasonMessage{Code: "PreBatchStepHookError", Reason: "WebhookFailureThresholdExceeded"}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)}}},
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-2"}, Replicas: intstr.FromInt(1)}}},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 2 ||
					newBatchStatus.CurrentBatchState != BatchStateInitial ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseProgressing {
					return false, nil
				}

				batchStatus := newBatchStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStateInitial {
					return false, nil
				}

				if newBatchStatus.Records[1].StartTime != nil ||
					len(newBatchStatus.Records[1].State) != 0 {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==2, currentBatchIndex=1, ProgressingStatus.Error!=nil}, Context={command=Skip}, Output={CurrentBatchState=2}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandSkip
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{State: BatchStateSucceeded, StartTime: &metav1.Time{Time: time.Now()}, FinishTime: &metav1.Time{Time: time.Now()}},
						{State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()}},
					},
				}
				rolloutRun.Status.Error = &rolloutv1alpha1.CodeReasonMessage{
					Code:   "PreBatchStepHookError",
					Reason: "WebhookFailureThresholdExceeded",
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)}}},
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-2"}, Replicas: intstr.FromInt(1)}}},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := newStatus.BatchStatus
				if newStatus.Error != nil ||
					len(newBatchStatus.Records) != 2 ||
					newBatchStatus.CurrentBatchState != BatchStatePreBatchHook ||
					newStatus.Phase != rolloutv1alpha1.RolloutRunPhasePostRollout {
					return false, nil
				}

				batchStatus := newBatchStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newBatchStatus.Records[1].StartTime == nil ||
					newBatchStatus.Records[1].State != BatchStatePreBatchHook {
					return false, nil
				}

				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoBatchInitial(t *testing.T) {
	RegisterFailHandler(Fail)

	testcases := []testCase{
		{
			name: "Input={len(Batches)==0}, Context={}, Output={}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}
				newStatus := rolloutRun.Status
				newBatchStatus := rolloutRun.Status.BatchStatus
				if newBatchStatus.CurrentBatchIndex != 0 ||
					newStatus.Error != nil ||
					len(newBatchStatus.Records) != 0 {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1}, Context={}, Output={CurrentBatchState=PreBatchStepHook}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Records: make([]rolloutv1alpha1.RolloutRunBatchStatusRecord, 1),
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}
				newStatus := rolloutRun.Status
				newBatchStatus := rolloutRun.Status.BatchStatus
				if newBatchStatus.CurrentBatchIndex != 0 ||
					newStatus.Error != nil ||
					len(newBatchStatus.Records) != 1 ||
					newBatchStatus.Records[0].StartTime == nil ||
					newBatchStatus.Records[0].State != BatchStatePreBatchHook ||
					newBatchStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}
				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoBatchSucceeded(t *testing.T) {
	RegisterFailHandler(Fail)

	ts := httptest.NewServer(makeHandlerFunc())
	defer ts.Close()

	testcases := []testCase{
		{
			name: "Input={len(Batches)==1, CurrentBatchState=Succeed,}, Context={}, Output={}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{

					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStateSucceeded,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: BatchStateSucceeded, StartTime: &metav1.Time{Time: time.Now()}, FinishTime: &metav1.Time{Time: time.Now()}}},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := rolloutRun.Status.BatchStatus
				if newBatchStatus.CurrentBatchIndex != 0 ||
					newStatus.Error != nil ||
					len(newBatchStatus.Records) != 1 ||
					newBatchStatus.CurrentBatchState != BatchStateSucceeded {
					return false, nil
				}

				if newBatchStatus.Records[0].StartTime == nil ||
					newBatchStatus.Records[0].FinishTime == nil ||
					newBatchStatus.Records[0].State != BatchStateSucceeded {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==2, CurrentBatchState=Succeed,}, Context={}, Output={CurrentBatchState=Pending}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{

					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStateSucceeded,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: BatchStateSucceeded, StartTime: &metav1.Time{Time: time.Now()}, FinishTime: &metav1.Time{Time: time.Now()}}},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)}}},
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-2"}, Replicas: intstr.FromInt(1)}}},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, err error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || err != nil {
					return false, nil
				}

				newStatus := rolloutRun.Status
				newBatchStatus := rolloutRun.Status.BatchStatus
				if newBatchStatus.CurrentBatchIndex != 1 ||
					newStatus.Error != nil ||
					newBatchStatus.CurrentBatchState != BatchStateInitial {
					return false, nil
				}

				if len(newBatchStatus.Records) != 1 ||
					newBatchStatus.Records[0].StartTime == nil ||
					newBatchStatus.Records[0].FinishTime == nil ||
					newBatchStatus.Records[0].State != BatchStateSucceeded {
					return false, nil
				}

				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}
