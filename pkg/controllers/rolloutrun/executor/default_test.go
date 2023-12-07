package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload/fake"
)

const (
	reqKeySleepSeconds = "sleepSeconds"
	reqKeyResponseBody = "ResponseBody"
	reqKeyResponseCode = "ResponseCode"
)

var (
	fakeWi     = fake.New("", "", "")
	apiVersion = schema.GroupVersion{
		Group: fakeWi.GetInfo().GVK.Group, Version: fakeWi.GetInfo().GVK.Version,
	}

	rollout = rolloutv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   metav1.NamespaceDefault,
			UID:         types.UID(uuid.New().String()),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: rolloutv1alpha1.RolloutSpec{},
	}

	rolloutRun = rolloutv1alpha1.RolloutRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   metav1.NamespaceDefault,
			UID:         types.UID(uuid.New().String()),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: rolloutv1alpha1.RolloutRunSpec{
			TargetType: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: apiVersion.String(), Kind: fakeWi.GetInfo().GVK.Kind,
			},
			Webhooks: []rolloutv1alpha1.RolloutWebhook{},
			Batch: rolloutv1alpha1.RolloutRunBatchStrategy{
				Toleration: &rolloutv1alpha1.TolerationStrategy{},
				Batches:    []rolloutv1alpha1.RolloutRunStep{},
			},
		},
		Status: rolloutv1alpha1.RolloutRunStatus{
			Conditions: []rolloutv1alpha1.Condition{},
		},
	}
)

type testCase struct {
	name                string
	checkResult         checkResult
	makeExecutorContext makeExecutorContext
}

type checkResult func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error)

type makeExecutorContext func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext

func runTestCase(t *testing.T, cases []testCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			executor := NewDefaultExecutor(zap.New(
				zap.WriteTo(os.Stdout), zap.UseDevMode(true),
			))

			newRollout := rollout.DeepCopy()
			newRolloutRun := rolloutRun.DeepCopy()
			done, result, err := executor.Do(
				ctx, tc.makeExecutorContext(newRollout, newRolloutRun),
			)

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

func TestDoInitialized(t *testing.T) {
	RegisterFailHandler(Fail)

	testcases := []testCase{
		{
			name: "empty batch",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if !done || result.Requeue || error != nil {
					return false, nil
				}
				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 0 {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "doInitialized",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}
				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoError(t *testing.T) {
	RegisterFailHandler(Fail)

	testcases := []testCase{
		{
			name: "do error",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook,
						CurrentBatchError: &rolloutv1alpha1.CodeReasonMessage{Code: "PreBatchStepHookError", Reason: "WebhookFailureThresholdExceeded"},
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     rolloutv1alpha1.BatchStepStatePreBatchStepHook,
						StartTime: &metav1.Time{Time: time.Now()},
						Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
							{
								Name:              "wh-01",
								FailureCount:      2,
								HookType:          rolloutv1alpha1.HookTypePreBatchStep,
								CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: "PreBatchStepHookError", Reason: "WebhookFailureThresholdExceeded"},
							},
						},
					}},
				}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.IsZero() || error != nil {
					return false, nil
				}

				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}

				if batchStatus.CurrentBatchError.Code != newCode(rolloutv1alpha1.HookTypePreBatchStep) ||
					batchStatus.CurrentBatchError.Reason != ReasonWebhookFailureThresholdExceeded {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookFailureThresholdExceeded" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != "PreBatchStepHookError" {
					return false, nil
				}

				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoPreBatchHook(t *testing.T) {
	RegisterFailHandler(Fail)

	ts := httptest.NewServer(makeHandlerFunc())
	defer ts.Close()

	testcases := []testCase{
		{
			name: "no webhook",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: rolloutv1alpha1.BatchStepStatePreBatchStepHook, StartTime: &metav1.Time{Time: time.Now()}}},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStateRunning ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStateRunning {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "one webhook and client config invalid and failureThreshold is 2 and do first time",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: rolloutv1alpha1.BatchStepStatePreBatchStepHook, StartTime: &metav1.Time{Time: time.Now()}}},
				}
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: "@*&"},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(5)*time.Second) || error != nil {
					return false, nil
				}

				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 1 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "one webhook and client config invalid and failureThreshold is 2 and do second time and FailurePolicy fail",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     rolloutv1alpha1.BatchStepStatePreBatchStepHook,
						StartTime: &metav1.Time{Time: time.Now()},
						Webhooks:  []rolloutv1alpha1.BatchWebhookStatus{{Name: "wh-01", FailureCount: 1, HookType: rolloutv1alpha1.HookTypePreBatchStep}},
					}},
				}
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: "@*&"},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.IsZero() || error == nil {
					return false, nil
				}

				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}

				if batchStatus.CurrentBatchError.Code != newCode(rolloutv1alpha1.HookTypePreBatchStep) ||
					batchStatus.CurrentBatchError.Reason != ReasonWebhookFailureThresholdExceeded {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "one webhook and client config invalid and failureThreshold is 2 and do second time and FailurePolicy ignore",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     rolloutv1alpha1.BatchStepStatePreBatchStepHook,
						StartTime: &metav1.Time{Time: time.Now()},
						Webhooks:  []rolloutv1alpha1.BatchWebhookStatus{{Name: "wh-01", FailureCount: 1, HookType: rolloutv1alpha1.HookTypePreBatchStep}},
					}},
				}
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Ignore,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: "@*&"},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStateRunning ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStateRunning {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "one webhook and client config invalid and failureThreshold is 2 and do second time and FailurePolicy ignore and move to next webhook",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     rolloutv1alpha1.BatchStepStatePreBatchStepHook,
						StartTime: &metav1.Time{Time: time.Now()},
						Webhooks:  []rolloutv1alpha1.BatchWebhookStatus{{Name: "wh-01", FailureCount: 1, HookType: rolloutv1alpha1.HookTypePreBatchStep}},
					}},
				}
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Ignore,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: "@*&"},
					},
					{
						Name:         "wh-02",
						HookTypes:    []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig: rolloutv1alpha1.WebhookClientConfig{URL: "@*&"},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(5)*time.Second) || error != nil {
					return false, nil
				}
				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[1].Name != "wh-02" ||
					batchStatus.Records[0].Webhooks[1].FailureCount != 1 ||
					batchStatus.Records[0].Webhooks[1].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[1].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					batchStatus.Records[0].Webhooks[1].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "one webhook and server side not 200",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: rolloutv1alpha1.BatchStepStatePreBatchStepHook, StartTime: &metav1.Time{Time: time.Now()},
					}},
				}
				url := fmt.Sprintf("%s/webhook?%s=1&%s=400", ts.URL, reqKeySleepSeconds, reqKeyResponseCode)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url, TimeoutSeconds: 2, PeriodSeconds: 3},
						Properties:       map[string]string{},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.CurrentBatchError != nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 1 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "one webhook and server side error",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: rolloutv1alpha1.BatchStepStatePreBatchStepHook, StartTime: &metav1.Time{Time: time.Now()},
					}},
				}
				url := fmt.Sprintf("%s/webhook?", ts.URL)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url, TimeoutSeconds: 2, PeriodSeconds: 3},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"Error\",\"reason\":\"Failed\",\"message\":\"Something is wrong\"}"},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.CurrentBatchError != nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 1 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "Failed" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "one webhook and server side processing",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: rolloutv1alpha1.BatchStepStatePreBatchStepHook, StartTime: &metav1.Time{Time: time.Now()},
					}},
				}
				url := fmt.Sprintf("%s/webhook", ts.URL)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url, TimeoutSeconds: 2, PeriodSeconds: 3},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"Processing\",\"reason\":\"WaitForCondition\",\"message\":\"Something is Processing\"}"},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.CurrentBatchError != nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 0 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WaitForCondition" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeProcessing {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "one webhook and server side ok",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: rolloutv1alpha1.BatchStepStatePreBatchStepHook, StartTime: &metav1.Time{Time: time.Now()},
					}},
				}
				url := fmt.Sprintf("%s/webhook", ts.URL)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url, TimeoutSeconds: 2, PeriodSeconds: 3},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.CurrentBatchError != nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.RolloutReasonProgressingRunning ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.RolloutReasonProgressingRunning {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 0 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "Success" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "one webhook and server side timeout",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: rolloutv1alpha1.BatchStepStatePreBatchStepHook, StartTime: &metav1.Time{Time: time.Now()},
					}},
				}
				url := fmt.Sprintf("%s/webhook?%s=3&%s=400", ts.URL, reqKeySleepSeconds, reqKeyResponseCode)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url, TimeoutSeconds: 2, PeriodSeconds: 3},
						Properties:       map[string]string{},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.CurrentBatchError != nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 1 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "two webhook and do hook timeout",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePreBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: rolloutv1alpha1.BatchStepStatePreBatchStepHook, StartTime: &metav1.Time{Time: time.Now()},
					}},
				}
				urlWh1 := fmt.Sprintf("%s/webhook?%s=5", ts.URL, reqKeySleepSeconds)
				urlWh2 := fmt.Sprintf("%s/webhook?%s=3&%s=400", ts.URL, reqKeySleepSeconds, reqKeyResponseCode)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:         "wh-01",
						HookTypes:    []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig: rolloutv1alpha1.WebhookClientConfig{URL: urlWh1, TimeoutSeconds: 6, PeriodSeconds: 3},
						Properties:   map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
					{
						Name:             "wh-02",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Fail,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: urlWh2, TimeoutSeconds: 2, PeriodSeconds: 3},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.CurrentBatchError != nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 0 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "Success" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[1].Name != "wh-02" ||
					batchStatus.Records[0].Webhooks[1].FailureCount != 0 ||
					batchStatus.Records[0].Webhooks[1].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[1].CodeReasonMessage != (rolloutv1alpha1.CodeReasonMessage{}) {
					return false, nil
				}

				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoPostBatchHook(t *testing.T) {
	RegisterFailHandler(Fail)

	ts := httptest.NewServer(makeHandlerFunc())
	defer ts.Close()

	testcases := []testCase{
		{
			name: "no webhook",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePostBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: rolloutv1alpha1.BatchStepStatePreBatchStepHook, StartTime: &metav1.Time{Time: time.Now()}}},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Pause: pointer.Bool(true),
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePaused ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePaused {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "two webhook",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: rolloutv1alpha1.BatchStepStatePostBatchStepHook, CurrentBatchError: nil,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     rolloutv1alpha1.BatchStepStatePreBatchStepHook,
						StartTime: &metav1.Time{Time: time.Now()},
						Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
							{
								Name:              "wh-01",
								HookType:          rolloutv1alpha1.HookTypePreBatchStep,
								FailureCount:      2,
								CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: rolloutv1alpha1.WebhookReviewCodeError, Reason: ReasonWebhookExecuteError, Message: "Error"},
							},
						},
					}},
				}

				url := fmt.Sprintf("%s/webhook", ts.URL)
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:             "wh-01",
						FailureThreshold: 2,
						FailurePolicy:    rolloutv1alpha1.Ignore,
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
					{
						Name:         "wh-02",
						HookTypes:    []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig: rolloutv1alpha1.WebhookClientConfig{URL: url},
						Properties:   map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Pause: pointer.Bool(true),
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePaused ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePaused {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					batchStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					batchStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[1].Name != "wh-01" ||
					batchStatus.Records[0].Webhooks[1].FailureCount != 0 ||
					batchStatus.Records[0].Webhooks[1].HookType != rolloutv1alpha1.HookTypePostBatchStep ||
					batchStatus.Records[0].Webhooks[1].CodeReasonMessage.Reason != "Success" ||
					batchStatus.Records[0].Webhooks[1].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
					return false, nil
				}

				if batchStatus.Records[0].Webhooks[2].Name != "wh-02" ||
					batchStatus.Records[0].Webhooks[2].FailureCount != 0 ||
					batchStatus.Records[0].Webhooks[2].HookType != rolloutv1alpha1.HookTypePostBatchStep ||
					batchStatus.Records[0].Webhooks[2].CodeReasonMessage.Reason != "Success" ||
					batchStatus.Records[0].Webhooks[2].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
					return false, nil
				}

				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}
