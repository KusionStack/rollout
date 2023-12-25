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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rolloutapis "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/workloadregistry"
	"kusionstack.io/rollout/pkg/workload"
	fakev1alpha1 "kusionstack.io/rollout/pkg/workload/fake"
)

const (
	reqKeySleepSeconds = "sleepSeconds"
	reqKeyResponseBody = "ResponseBody"
	reqKeyResponseCode = "ResponseCode"
)

var (
	apiSchema *runtime.Scheme

	apiVersion = schema.GroupVersion{
		Group: fakev1alpha1.GVK.Group, Version: fakev1alpha1.GVK.Version,
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
				APIVersion: apiVersion.String(), Kind: fakev1alpha1.GVK.Kind,
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
	setup               setup
	checkResult         checkResult
	makeExecutorContext makeExecutorContext
}

type setup func()

type checkResult func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error)

type makeExecutorContext func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext

func runTestCase(t *testing.T, cases []testCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			executor := NewDefaultExecutor(zap.New(
				zap.WriteTo(os.Stdout), zap.UseDevMode(true),
			))

			if tc.setup != nil {
				tc.setup()
			}

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

// setupStore mock store and wi
func setupStore() {
	apiSchema = runtime.NewScheme()
	utilruntime.Must(rolloutv1alpha1.AddToScheme(apiSchema))

	clientBuilder := fake.NewClientBuilder().WithScheme(apiSchema)

	workloadregistry.DefaultRegistry.Register(
		fakev1alpha1.GVK,
		&fakev1alpha1.Storage{Client: clientBuilder.Build()},
	)
}

type emptyStore struct {
}

func (e emptyStore) NewObject() client.Object {
	//TODO implement me
	panic("implement me")
}

func (e emptyStore) NewObjectList() client.ObjectList {
	//TODO implement me
	panic("implement me")
}

func (e emptyStore) Watchable() bool {
	//TODO implement me
	panic("implement me")
}

func (e emptyStore) Wrap(cluster string, obj client.Object) (workload.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (e emptyStore) Get(ctx context.Context, cluster, namespace, name string) (workload.Interface, error) {
	return nil, nil
}

func (e emptyStore) List(ctx context.Context, namespace string, match rolloutv1alpha1.ResourceMatch) ([]workload.Interface, error) {
	//TODO implement me
	panic("implement me")
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
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 0 ||
					len(progressingStatus.Context) != 0 ||
					len(progressingStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStatePreRollout {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Initial}, Context={}, Output={ProgressingStatus.State==PreRollout}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State: rolloutv1alpha1.RolloutProgressingStateInitial, Context: map[string]string{},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 0 ||
					len(progressingStatus.Context) != 0 ||
					len(progressingStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStatePreRollout {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Initial, ProgressingStatus.Error!=nil}, Context={}, Output={ProgressingStatus.State==Initial}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Error: &rolloutv1alpha1.CodeReasonMessage{
						Code: "PreBatchStepHookError", Reason: "WebhookFailureThresholdExceeded",
					},
					State: rolloutv1alpha1.RolloutProgressingStateInitial, Context: map[string]string{},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.Requeue || error != nil {
					return false, nil
				}
				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error == nil ||
					len(progressingStatus.Records) != 0 ||
					len(progressingStatus.Context) != 0 ||
					len(progressingStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStateInitial {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Paused}, Context={}, Output={ProgressingStatus.State==Paused}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State: rolloutv1alpha1.RolloutProgressingStatePaused, Context: map[string]string{},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.Requeue || error != nil {
					return false, nil
				}
				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 0 ||
					len(progressingStatus.Context) != 0 ||
					len(progressingStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStatePaused {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Canceling}, Context={}, Output={ProgressingStatus.State==Completed}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State: rolloutv1alpha1.RolloutProgressingStateCanceling, Context: map[string]string{},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.Requeue || error != nil {
					return false, nil
				}
				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 0 ||
					len(progressingStatus.Context) != 0 ||
					len(progressingStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStateCompleted {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Completed}, Context={}, Output={ProgressingStatus.State==Completed}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State: rolloutv1alpha1.RolloutProgressingStateCompleted, Context: map[string]string{},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.Requeue || error != nil {
					return false, nil
				}
				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 0 ||
					len(progressingStatus.Context) != 0 ||
					len(progressingStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStateCompleted {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==PreRollout}, Context={}, Output={ProgressingStatus.State==Rolling}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State: rolloutv1alpha1.RolloutProgressingStatePreRollout, Context: map[string]string{},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 0 ||
					len(progressingStatus.Context) != 0 ||
					len(progressingStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStateRolling {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Rolling}, Context={}, Output={ProgressingStatus.State==PostRollout}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State: rolloutv1alpha1.RolloutProgressingStateRolling, Context: map[string]string{},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 0 ||
					len(progressingStatus.Context) != 0 ||
					len(progressingStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStatePostRollout {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==PostRollout}, Context={}, Output={ProgressingStatus.State==Completed}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State: rolloutv1alpha1.RolloutProgressingStatePostRollout, Context: map[string]string{},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if !done || result.Requeue || error != nil {
					return false, nil
				}
				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 0 ||
					len(progressingStatus.Context) != 0 ||
					len(progressingStatus.RolloutBatchStatus.CurrentBatchState) != 0 ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStateCompleted {
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
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandResume
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
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
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
									HookType:          rolloutv1alpha1.HookTypePreBatchStep,
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
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 2 ||
					len(progressingStatus.Context) != 0 ||
					progressingStatus.CurrentBatchState != BatchStatePreBatchHook ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStateRolling {
					return false, nil
				}

				batchStatus := progressingStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if progressingStatus.Records[1].StartTime == nil ||
					progressingStatus.Records[1].State != BatchStatePreBatchHook {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.Error!=nil}, Context={command=Retry}, Output={ProgressingStatus.Error==nil}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandRetry
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					Error:   &rolloutv1alpha1.CodeReasonMessage{Code: "PreBatchStepHookError", Reason: "WebhookFailureThresholdExceeded"},
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
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
									HookType:          rolloutv1alpha1.HookTypePreBatchStep,
									CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: "Ok", Reason: "Success"},
								},
							},
						},
					},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 1 ||
					len(progressingStatus.Context) != 0 ||
					progressingStatus.CurrentBatchState != BatchStatePreBatchHook ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStateRolling {
					return false, nil
				}

				batchStatus := progressingStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if progressingStatus.Records[0].StartTime == nil ||
					progressingStatus.Records[0].State != BatchStatePreBatchHook {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Rolling}, Context={command=Pause}, Output={ProgressingStatus.State==Paused}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
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
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
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
									HookType:          rolloutv1alpha1.HookTypePreBatchStep,
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
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 2 ||
					len(progressingStatus.Context) != 0 ||
					progressingStatus.RolloutBatchStatus.CurrentBatchState != BatchStatePaused ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStatePaused {
					return false, nil
				}

				batchStatus := progressingStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStatePaused {
					return false, nil
				}

				if progressingStatus.Records[1].StartTime == nil ||
					progressingStatus.Records[1].State != BatchStatePaused {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={ProgressingStatus.State==Rolling}, Context={command=Cancel}, Output={ProgressingStatus.State==Canceling}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
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
						HookTypes:        []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
						ClientConfig:     rolloutv1alpha1.WebhookClientConfig{URL: url},
						Properties:       map[string]string{reqKeyResponseBody: "{\"code\":\"OK\",\"reason\":\"Success\",\"message\":\"Success\"}"},
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{},
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
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
									HookType:          rolloutv1alpha1.HookTypePreBatchStep,
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
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 2 ||
					len(progressingStatus.Context) != 0 ||
					progressingStatus.CurrentBatchState != BatchStatePaused ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStateCanceling {
					return false, nil
				}

				batchStatus := progressingStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStatePaused {
					return false, nil
				}

				if progressingStatus.Records[1].StartTime == nil ||
					progressingStatus.Records[1].State != BatchStatePaused {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==2, currentBatchIndex=0, ProgressingStatus.Error!=nil}, Context={command=Skip}, Output={CurrentBatchState=2}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandSkip
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   &rolloutv1alpha1.CodeReasonMessage{Code: "PreBatchStepHookError", Reason: "WebhookFailureThresholdExceeded"},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()}}, {}},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)}}},
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-2"}, Replicas: intstr.FromInt(1)}}},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 2 ||
					len(progressingStatus.Context) != 0 ||
					progressingStatus.CurrentBatchState != BatchStateInitial ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStateRolling {
					return false, nil
				}

				batchStatus := progressingStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStateInitial {
					return false, nil
				}

				if progressingStatus.Records[1].StartTime != nil ||
					len(progressingStatus.Records[1].State) != 0 {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==2, currentBatchIndex=1, ProgressingStatus.Error!=nil}, Context={command=Skip}, Output={CurrentBatchState=2}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey] = rolloutapis.AnnoManualCommandSkip
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   &rolloutv1alpha1.CodeReasonMessage{Code: "PreBatchStepHookError", Reason: "WebhookFailureThresholdExceeded"},
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{State: BatchStateSucceeded, StartTime: &metav1.Time{Time: time.Now()}, FinishTime: &metav1.Time{Time: time.Now()}},
						{State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()}},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)}}},
					{Targets: []rolloutv1alpha1.RolloutRunStepTarget{{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-2"}, Replicas: intstr.FromInt(1)}}},
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				if _, exist := rolloutRun.Annotations[rolloutapis.AnnoManualCommandKey]; exist {
					return false, nil
				}

				progressingStatus := rolloutRun.Status.BatchStatus
				if progressingStatus.Error != nil ||
					len(progressingStatus.Records) != 2 ||
					len(progressingStatus.Context) != 0 ||
					progressingStatus.CurrentBatchState != BatchStatePreBatchHook ||
					progressingStatus.State != rolloutv1alpha1.RolloutProgressingStatePostRollout {
					return false, nil
				}

				batchStatus := progressingStatus.RolloutBatchStatus
				if batchStatus.CurrentBatchIndex != 1 ||
					batchStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if progressingStatus.Records[1].StartTime == nil ||
					progressingStatus.Records[1].State != BatchStatePreBatchHook {
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
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{}, State: rolloutv1alpha1.RolloutProgressingStateRolling,
				}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 0 {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1}, Context={}, Output={CurrentBatchState=PreBatchStepHook}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context: map[string]string{}, State: rolloutv1alpha1.RolloutProgressingStateRolling,
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}
				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoBatchError(t *testing.T) {
	RegisterFailHandler(Fail)

	testcases := []testCase{
		{
			name: "Input={len(Batches)==1, CurrentBatchState=PreBatchStepHook, error!=nil}, Context={}, Output={CurrentBatchState=PreBatchStepHook}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{Targets: []rolloutv1alpha1.RolloutRunStepTarget{}}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					Context:            map[string]string{},
					State:              rolloutv1alpha1.RolloutProgressingStateRolling,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook},
					Error:              &rolloutv1alpha1.CodeReasonMessage{Code: "PreBatchStepHookError", Reason: "WebhookFailureThresholdExceeded"},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     BatchStatePreBatchHook,
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.IsZero() || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newProgressingStatus.Error.Code != newCode(rolloutv1alpha1.HookTypePreBatchStep) ||
					newProgressingStatus.Error.Reason != ReasonWebhookFailureThresholdExceeded {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookFailureThresholdExceeded" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != "PreBatchStepHookError" {
					return false, nil
				}

				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoBatchPreBatchHook(t *testing.T) {
	RegisterFailHandler(Fail)

	ts := httptest.NewServer(makeHandlerFunc())
	defer ts.Close()

	testcases := []testCase{
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook,}, Context={}, Output={CurrentBatchState=Running}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()}}},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStateUpgrading ||
					newProgressingStatus.CurrentBatchState != BatchStateUpgrading {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook, failureThreshold=2, client config invalid,}, Context={do first time}, Output={CurrentBatchState=PreBatchStepHook}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()}}},
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(5)*time.Second) || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 1 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook, failureThreshold=2, client config invalid,}, Context={do second time}, Output={CurrentBatchState=PreBatchStepHook, CurrentBatchError!=nil}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     BatchStatePreBatchHook,
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.IsZero() || error == nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newProgressingStatus.Error.Code != newCode(rolloutv1alpha1.HookTypePreBatchStep) ||
					newProgressingStatus.Error.Reason != ReasonWebhookFailureThresholdExceeded {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook, failureThreshold=2, FailurePolicy=ignore, client config invalid,}, Context={do second time}, Output={CurrentBatchState=Running, }",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     BatchStatePreBatchHook,
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStateUpgrading ||
					newProgressingStatus.CurrentBatchState != BatchStateUpgrading {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==2, CurrentBatchState=PreBatchStepHook, failureThreshold=2, FailurePolicy=ignore, client config invalid,}, Context={do second time}, Output={CurrentBatchState=Running, }",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     BatchStatePreBatchHook,
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(5)*time.Second) || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[1].Name != "wh-02" ||
					newProgressingStatus.Records[0].Webhooks[1].FailureCount != 1 ||
					newProgressingStatus.Records[0].Webhooks[1].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[1].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					newProgressingStatus.Records[0].Webhooks[1].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook, }, Context={server side 400}, Output={CurrentBatchState=PreBatchStepHook, CurrentBatchError!=nil }",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()},
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 1 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook, }, Context={server side error}, Output={CurrentBatchState=PreBatchStepHook, CurrentBatchError!=nil }",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()},
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 1 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "Failed" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook, }, Context={server side processing}, Output={CurrentBatchState=PreBatchStepHook, }",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()},
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 0 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WaitForCondition" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeProcessing {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook, }, Context={server side ok}, Output={CurrentBatchState=Running, }",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()},
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.Records[0].State != rolloutv1alpha1.RolloutReasonProgressingRunning ||
					newProgressingStatus.CurrentBatchState != rolloutv1alpha1.RolloutReasonProgressingRunning {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 0 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "Success" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook, }, Context={server side timeout}, Output={CurrentBatchState=PreBatchStepHook, }",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()},
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 1 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)==1, CurrentBatchState=PreBatchStepHook, }, Context={webhook timeout}, Output={CurrentBatchState=PreBatchStepHook, }",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePreBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()},
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
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || (result.RequeueAfter != time.Duration(3)*time.Second) || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.Records[0].State != BatchStatePreBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePreBatchHook {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 0 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "Success" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[1].Name != "wh-02" ||
					newProgressingStatus.Records[0].Webhooks[1].FailureCount != 0 ||
					newProgressingStatus.Records[0].Webhooks[1].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[1].CodeReasonMessage != (rolloutv1alpha1.CodeReasonMessage{}) {
					return false, nil
				}

				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoBatchUpgrading(t *testing.T) {
	RegisterFailHandler(Fail)

	testcases := []testCase{
		{
			name: "Input{CurrentBatchState=Upgrading, len(targets)==1}, Context{store is null}, Output{CurrentBatchState=Upgrading, CurrentBatchError is not nil}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:      "wh-01",
						HookTypes: []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
				}}}

				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{Name: "wh-01", HookType: rolloutv1alpha1.HookTypePreBatchStep, CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: rolloutv1alpha1.WebhookReviewCodeOK, Reason: "Success"}, FailureCount: 0},
							},
						},
					},
				}

				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.Requeue || error == nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error == nil ||
					newProgressingStatus.Error.Code != CodeUpgradingError ||
					newProgressingStatus.Error.Reason != ReasonWorkloadStoreNotExist ||
					newProgressingStatus.CurrentBatchState != BatchStateUpgrading {
					return false, nil
				}

				if len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStateUpgrading {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input{CurrentBatchState==Upgrading, len(targets)==1}, Context{WorkloadInterface is null}, Output{CurrentBatchState=Upgrading, CurrentBatchError is not nil}",
			setup: func() {
				workloadregistry.DefaultRegistry.Register(fakev1alpha1.GVK, &emptyStore{})
			},
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:      "wh-01",
						HookTypes: []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
				}}}

				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{Name: "wh-01", HookType: rolloutv1alpha1.HookTypePreBatchStep, CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: rolloutv1alpha1.WebhookReviewCodeOK, Reason: "Success"}, FailureCount: 0},
							},
						},
					},
				}

				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.Requeue || error == nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error == nil ||
					newProgressingStatus.Error.Code != CodeUpgradingError ||
					newProgressingStatus.Error.Reason != ReasonWorkloadInterfaceNotExist ||
					newProgressingStatus.CurrentBatchState != BatchStateUpgrading {
					return false, nil
				}

				if len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					len(newProgressingStatus.Records[0].Targets) != 0 ||
					newProgressingStatus.Records[0].State != BatchStateUpgrading {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name:  "Input{CurrentBatchState==Upgrading, len(targets)==0}, Output{CurrentBatchState=PostBatchStepHook}",
			setup: setupStore,
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:      "wh-01",
						HookTypes: []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{Targets: []rolloutv1alpha1.RolloutRunStepTarget{}}}

				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{Name: "wh-01", HookType: rolloutv1alpha1.HookTypePreBatchStep, CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: rolloutv1alpha1.WebhookReviewCodeOK, Reason: "Success"}, FailureCount: 0},
							},
						},
					},
				}

				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStatePostBatchHook ||
					newProgressingStatus.CurrentBatchState != BatchStatePostBatchHook {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name:  "Input{CurrentBatchState==Upgrading, len(targets)==1}, Output{CurrentBatchState=PostBatchStepHook}",
			setup: setupStore,
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:      "wh-01",
						HookTypes: []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
				}}}

				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{Name: "wh-01", HookType: rolloutv1alpha1.HookTypePreBatchStep, CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: rolloutv1alpha1.WebhookReviewCodeOK, Reason: "Success"}, FailureCount: 0},
							},
						},
					},
				}

				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.CurrentBatchState != BatchStatePostBatchHook {
					return false, nil
				}

				if _, exist := newProgressingStatus.Context[ctxKeyLastUpgradeAt]; !exist {
					return false, nil
				}

				if len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					len(newProgressingStatus.Records[0].Webhooks) != 1 ||
					len(newProgressingStatus.Records[0].Targets) != 1 ||
					newProgressingStatus.Records[0].Targets[0].Name != "test-1" ||
					newProgressingStatus.Records[0].Targets[0].Cluster != "cluster-a" ||
					newProgressingStatus.Records[0].State != BatchStatePostBatchHook {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name:  "Input{CurrentBatchState==Upgrading, len(targets)==2}, Output{CurrentBatchState=Upgrading}",
			setup: setupStore,
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:      "wh-01",
						HookTypes: []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
					},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
					{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-2"}, Replicas: intstr.FromInt(2)},
				}}}

				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{ctxKeyLastUpgradeAt: time.Now().UTC().Format(time.RFC3339)},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
							Targets:   []rolloutv1alpha1.RolloutWorkloadStatus{{Cluster: "cluster-a", Name: "test-1"}},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{Name: "wh-01", HookType: rolloutv1alpha1.HookTypePreBatchStep, CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: rolloutv1alpha1.WebhookReviewCodeOK, Reason: "Success"}, FailureCount: 0},
							},
						},
					},
				}

				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.CurrentBatchState != BatchStatePostBatchHook {
					return false, nil
				}

				if _, exist := newProgressingStatus.Context[ctxKeyLastUpgradeAt]; !exist {
					return false, nil
				}

				if len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					len(newProgressingStatus.Records[0].Webhooks) != 1 ||
					len(newProgressingStatus.Records[0].Targets) != 2 ||
					newProgressingStatus.Records[0].Targets[0].Name != "test-1" ||
					newProgressingStatus.Records[0].Targets[0].Cluster != "cluster-a" ||
					newProgressingStatus.Records[0].Targets[1].Name != "test-2" ||
					newProgressingStatus.Records[0].Targets[1].Cluster != "cluster-a" ||
					newProgressingStatus.Records[0].State != BatchStatePostBatchHook {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name:  "Input{CurrentBatchState==Upgrading, len(targets)==2}, Context={Within InitialDelaySeconds}, Output{CurrentBatchState=Upgrading}",
			setup: setupStore,
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:      "wh-01",
						HookTypes: []rolloutv1alpha1.HookType{rolloutv1alpha1.HookTypePreBatchStep, rolloutv1alpha1.HookTypePostBatchStep},
					},
				}
				rolloutRun.Spec.Batch = rolloutv1alpha1.RolloutRunBatchStrategy{
					Toleration: &rolloutv1alpha1.TolerationStrategy{InitialDelaySeconds: 10},
					Batches: []rolloutv1alpha1.RolloutRunStep{{Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-2"}, Replicas: intstr.FromInt(2)},
					}}},
				}

				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{ctxKeyLastUpgradeAt: time.Now().Add(time.Duration(-5) * time.Second).UTC().Format(time.RFC3339)},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
							Targets:   []rolloutv1alpha1.RolloutWorkloadStatus{{Cluster: "cluster-a", Name: "test-1"}, {Cluster: "cluster-a", Name: "test-2"}},
							Webhooks: []rolloutv1alpha1.BatchWebhookStatus{
								{Name: "wh-01", HookType: rolloutv1alpha1.HookTypePreBatchStep, CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{Code: rolloutv1alpha1.WebhookReviewCodeOK, Reason: "Success"}, FailureCount: 0},
							},
						},
					},
				}

				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || result.RequeueAfter < 0 || result.RequeueAfter > (time.Duration(5)*time.Second) || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.CurrentBatchState != BatchStateUpgrading {
					return false, nil
				}

				if _, exist := newProgressingStatus.Context[ctxKeyLastUpgradeAt]; !exist {
					return false, nil
				}

				if len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					len(newProgressingStatus.Records[0].Webhooks) != 1 ||
					len(newProgressingStatus.Records[0].Targets) != 2 ||
					newProgressingStatus.Records[0].Targets[0].Name != "test-1" ||
					newProgressingStatus.Records[0].Targets[0].Cluster != "cluster-a" ||
					newProgressingStatus.Records[0].Targets[1].Name != "test-2" ||
					newProgressingStatus.Records[0].Targets[1].Cluster != "cluster-a" ||
					newProgressingStatus.Records[0].State != BatchStateUpgrading {
					return false, nil
				}

				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}

func TestDoBatchPostBatchHook(t *testing.T) {
	RegisterFailHandler(Fail)

	ts := httptest.NewServer(makeHandlerFunc())
	defer ts.Close()

	testcases := []testCase{
		{
			name: "Input={len(Batches)==1, len(Webhooks)==0, Pause=true, CurrentBatchState=PostBatchStepHook,}, Context={}, Output={CurrentBatchState=Succeeded}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePostBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{State: BatchStatePreBatchHook, StartTime: &metav1.Time{Time: time.Now()}}},
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Breakpoint: true,
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStateSucceeded ||
					newProgressingStatus.CurrentBatchState != BatchStateSucceeded {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)=2, Pause=true, CurrentBatchState=PostBatchStepHook,}, Context={}, Output={CurrentBatchState=Succeeded}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePostBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     BatchStatePreBatchHook,
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
					Breakpoint: true,
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].State != BatchStateSucceeded ||
					newProgressingStatus.CurrentBatchState != BatchStateSucceeded {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[1].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[1].FailureCount != 0 ||
					newProgressingStatus.Records[0].Webhooks[1].HookType != rolloutv1alpha1.HookTypePostBatchStep ||
					newProgressingStatus.Records[0].Webhooks[1].CodeReasonMessage.Reason != "Success" ||
					newProgressingStatus.Records[0].Webhooks[1].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[2].Name != "wh-02" ||
					newProgressingStatus.Records[0].Webhooks[2].FailureCount != 0 ||
					newProgressingStatus.Records[0].Webhooks[2].HookType != rolloutv1alpha1.HookTypePostBatchStep ||
					newProgressingStatus.Records[0].Webhooks[2].CodeReasonMessage.Reason != "Success" ||
					newProgressingStatus.Records[0].Webhooks[2].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==1, len(Webhooks)=2, Pause=false, CurrentBatchState=PostBatchStepHook,}, Context={}, Output={CurrentBatchState=Succeeded}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0, CurrentBatchState: BatchStatePostBatchHook,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{{
						State:     BatchStatePreBatchHook,
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
					Breakpoint: false,
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{Rollout: rollout, RolloutRun: rolloutRun, NewStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.CurrentBatchState != BatchStateSucceeded {
					return false, nil
				}

				if newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].FinishTime == nil ||
					newProgressingStatus.Records[0].State != BatchStateSucceeded {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[0].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[0].FailureCount != 2 ||
					newProgressingStatus.Records[0].Webhooks[0].HookType != rolloutv1alpha1.HookTypePreBatchStep ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Reason != "WebhookExecuteError" ||
					newProgressingStatus.Records[0].Webhooks[0].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeError {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[1].Name != "wh-01" ||
					newProgressingStatus.Records[0].Webhooks[1].FailureCount != 0 ||
					newProgressingStatus.Records[0].Webhooks[1].HookType != rolloutv1alpha1.HookTypePostBatchStep ||
					newProgressingStatus.Records[0].Webhooks[1].CodeReasonMessage.Reason != "Success" ||
					newProgressingStatus.Records[0].Webhooks[1].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
					return false, nil
				}

				if newProgressingStatus.Records[0].Webhooks[2].Name != "wh-02" ||
					newProgressingStatus.Records[0].Webhooks[2].FailureCount != 0 ||
					newProgressingStatus.Records[0].Webhooks[2].HookType != rolloutv1alpha1.HookTypePostBatchStep ||
					newProgressingStatus.Records[0].Webhooks[2].CodeReasonMessage.Reason != "Success" ||
					newProgressingStatus.Records[0].Webhooks[2].CodeReasonMessage.Code != rolloutv1alpha1.WebhookReviewCodeOK {
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
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
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
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 0 ||
					newProgressingStatus.Error != nil ||
					len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.CurrentBatchState != BatchStateSucceeded {
					return false, nil
				}

				if newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].FinishTime == nil ||
					newProgressingStatus.Records[0].State != BatchStateSucceeded {
					return false, nil
				}

				return true, nil
			},
		},
		{
			name: "Input={len(Batches)==2, CurrentBatchState=Succeed,}, Context={}, Output={CurrentBatchState=Pending}",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					State:   rolloutv1alpha1.RolloutProgressingStateRolling,
					Context: map[string]string{},
					Error:   nil,
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
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}

				newProgressingStatus := rolloutRun.Status.BatchStatus
				if newProgressingStatus.CurrentBatchIndex != 1 ||
					newProgressingStatus.Error != nil ||
					newProgressingStatus.CurrentBatchState != BatchStateInitial {
					return false, nil
				}

				if len(newProgressingStatus.Records) != 1 ||
					newProgressingStatus.Records[0].StartTime == nil ||
					newProgressingStatus.Records[0].FinishTime == nil ||
					newProgressingStatus.Records[0].State != BatchStateSucceeded {
					return false, nil
				}

				return true, nil
			},
		},
	}

	runTestCase(t, testcases)
}
