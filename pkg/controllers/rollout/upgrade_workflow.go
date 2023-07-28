package rollout

import (
	"fmt"
	"math"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/KusionStack/rollout/api"
	rolloutv1alpha1 "github.com/KusionStack/rollout/api/v1alpha1"
	"github.com/KusionStack/rollout/pkg/controllers/rollout/types"
	workflowutil "github.com/KusionStack/rollout/pkg/workflow"
	"github.com/KusionStack/rollout/pkg/workload"
)

const (
	BatchNameSuffixBreakPoint = "breakpoint"
	BatchNameSuffixPause      = "pause"
	BatchNameSuffixPre        = "pre"
	BatchNameSuffixPost       = "post"

	BatchNameBeta = "beta"
)

func parseBatchStrategy(canary *rolloutv1alpha1.BatchStrategy, workloadWrappers []workload.Interface) ([]*types.UpgradeBatchDetail, error) {
	if canary.BatchTemplate == nil {
		return nil, fmt.Errorf("invalid batchTemplate")
	}

	if canary.BatchTemplate.FixedBatchPolicy != nil {
		batchCount := canary.BatchTemplate.FixedBatchPolicy.BatchCount
		if batchCount < 1 {
			return nil, fmt.Errorf("invalid batchCount: %d", batchCount)
		}

		per := int(math.Floor(float64(100) / float64(batchCount)))
		if per < 1 {
			per = 1
		}
		breakPoints := map[int32]bool{}
		if canary.BatchTemplate.FixedBatchPolicy.Breakpoints != nil {
			for i := range canary.BatchTemplate.FixedBatchPolicy.Breakpoints {
				breakPoints[canary.BatchTemplate.FixedBatchPolicy.Breakpoints[i]] = true
			}
		}

		hasBeta := false
		ret := make([]*types.UpgradeBatchDetail, 0)
		if canary.BatchTemplate.FixedBatchPolicy.BetaBatch != nil {
			betaPerUnit := int32(1)
			if canary.BatchTemplate.FixedBatchPolicy.BetaBatch.ReplicasPerUnit != nil {
				betaPerUnit = *canary.BatchTemplate.FixedBatchPolicy.BetaBatch.ReplicasPerUnit
			}
			if betaPerUnit < 1 {
				return nil, fmt.Errorf("invalid betaPerUnit: %d", betaPerUnit)
			}

			item := types.UpgradeBatchDetail{
				IsBeta:    true,
				BatchNum:  0,
				BatchName: BatchNameBeta,
				Replicas:  intstr.FromInt(int(betaPerUnit)),
				NeedPause: true, // need pause after beta
				Workloads: workloadWrappers,
			}
			if canary.TolerationPolicy != nil {
				item.FailureThreshold = canary.TolerationPolicy.FailureThreshold
				item.WaitTimeSeconds = canary.TolerationPolicy.WaitTimeSeconds
			}
			if canary.Analysis != nil {
				item.AnalysisRules = canary.Analysis.Rules
			}
			ret = append(ret, &item)
			hasBeta = true
		}
		for i := int32(1); i <= batchCount; i++ {
			item := types.UpgradeBatchDetail{}
			item.BatchNum = i
			item.BatchName = fmt.Sprintf("%d", i)
			if _, ok := breakPoints[i]; ok {
				item.HasBreakPoint = true
			}
			if canary.PauseMode == rolloutv1alpha1.PauseModeTypeFirstBatch {
				if !hasBeta && i == 1 {
					item.NeedPause = true
				}
			} else if canary.PauseMode == rolloutv1alpha1.PauseModeTypeEachBatch {
				if i < batchCount {
					item.NeedPause = true
				}
			}

			// percent
			if i < batchCount {
				item.Replicas = intstr.FromString(fmt.Sprintf("%d%s", int(i)*per, "%"))
			} else {
				item.Replicas = intstr.FromString("100%")
			}
			if canary.TolerationPolicy != nil {
				item.FailureThreshold = canary.TolerationPolicy.FailureThreshold
				item.WaitTimeSeconds = canary.TolerationPolicy.WaitTimeSeconds
			}
			item.Workloads = workloadWrappers
			if canary.Analysis != nil {
				item.AnalysisRules = canary.Analysis.Rules
			}
			ret = append(ret, &item)
		}

		return ret, nil
	} else if canary.BatchTemplate.CustomBatchPolicy != nil {
		return nil, fmt.Errorf("only support FixedBatchPolicy for now")
	} else {
		return nil, fmt.Errorf("only support FixedBatchPolicy and CustomBatchPolicy for now")
	}
}

func constructWorkflow(instance *rolloutv1alpha1.Rollout, strategy *rolloutv1alpha1.BatchStrategy, workloadWrappers []workload.Interface, rolloutId string) (*rolloutv1alpha1.Workflow, error) {
	workflow := &rolloutv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    instance.Namespace,
			GenerateName: fmt.Sprintf("%s-", instance.Name),
			Labels: map[string]string{
				api.LabelRolloutControl:   "true",
				api.LabelRolloutReference: instance.Name,
				api.LabelRolloutID:        rolloutId,
			},
			Annotations: map[string]string{},
		},
		Spec: rolloutv1alpha1.WorkflowSpec{},
	}

	// construct tasks
	batchDetails, err := parseBatchStrategy(strategy, workloadWrappers)
	if err != nil {
		return nil, err
	}
	if batchDetails == nil || len(batchDetails) < 1 {
		return nil, fmt.Errorf("invalid batchDetails")
	}

	tasks := make([]*rolloutv1alpha1.WorkflowTask, 0)
	var lastBatchSuspendTask *rolloutv1alpha1.WorkflowTask
	var lastBatchEndTasks []*rolloutv1alpha1.WorkflowTask
	taskID := int32(1)
	for pos, batchDetail := range batchDetails {
		if batchDetail.Workloads == nil || len(batchDetail.Workloads) < 1 {
			return nil, fmt.Errorf("invalid workloads for batch: %d", batchDetail.BatchNum)
		}

		var breakPointTask *rolloutv1alpha1.WorkflowTask
		var preTasks []*rolloutv1alpha1.WorkflowTask
		var mainTasks []*rolloutv1alpha1.WorkflowTask
		var postTasks []*rolloutv1alpha1.WorkflowTask
		var pauseTask *rolloutv1alpha1.WorkflowTask

		if batchDetail.HasBreakPoint && lastBatchSuspendTask == nil {
			fromBatchName := ""
			if pos > 0 {
				fromBatchName = batchDetails[pos-1].BatchName
			}
			breakPointTask = workflowutil.InitSuspendTask(batchDetail.BatchName, BatchNameSuffixBreakPoint, fromBatchName, batchDetail.BatchName, taskID)
			taskID++
		}
		for _, rule := range batchDetail.AnalysisRules {
			if rule.CheckPoints == nil {
				continue
			}
			for _, checkPoint := range rule.CheckPoints {
				if checkPoint == rolloutv1alpha1.CheckPointTypePreBatch {
					if preTasks == nil {
						preTasks = make([]*rolloutv1alpha1.WorkflowTask, 0)
					}
					preTasks = append(preTasks, workflowutil.InitAnalysisTask(batchDetail.BatchName, rule, BatchNameSuffixPre, taskID))
					taskID++
				}
			}
		}
		for i := range batchDetail.Workloads {
			if mainTasks == nil {
				mainTasks = make([]*rolloutv1alpha1.WorkflowTask, 0)
			}
			mainTasks = append(mainTasks, workflowutil.InitUpgradeTask(batchDetail.BatchName, batchDetail.Workloads[i], batchDetail.Replicas, batchDetail.WaitTimeSeconds, batchDetail.FailureThreshold, taskID))
			taskID++
		}

		for _, rule := range batchDetail.AnalysisRules {
			if rule.CheckPoints == nil {
				continue
			}
			for _, checkPoint := range rule.CheckPoints {
				if checkPoint == rolloutv1alpha1.CheckPointTypePostBatch {
					if postTasks == nil {
						postTasks = make([]*rolloutv1alpha1.WorkflowTask, 0)
					}
					postTasks = append(postTasks, workflowutil.InitAnalysisTask(batchDetail.BatchName, rule, BatchNameSuffixPost, taskID))
					taskID++
				}
			}
		}

		// init pauseTask and update lastBatchSuspendTask for next forEach item
		if batchDetail.NeedPause && pos < len(batchDetails)-1 {
			pauseTask = workflowutil.InitSuspendTask(batchDetail.BatchName, BatchNameSuffixPause, batchDetail.BatchName, batchDetails[pos+1].BatchName, taskID)
			taskID++
			lastBatchSuspendTask = pauseTask
		} else {
			lastBatchSuspendTask = nil
		}

		if mainTasks == nil || len(mainTasks) < 1 {
			return nil, fmt.Errorf("empty mainTasks for batch: %d", batchDetail.BatchNum)
		}

		// construct edge
		workflowutil.BindTaskFlow(lastBatchEndTasks, []*rolloutv1alpha1.WorkflowTask{breakPointTask}, preTasks, mainTasks, postTasks, []*rolloutv1alpha1.WorkflowTask{pauseTask})
		// add to tasks
		tasks = workflowutil.AggregateTasks(tasks, []*rolloutv1alpha1.WorkflowTask{breakPointTask}, preTasks, mainTasks, postTasks, []*rolloutv1alpha1.WorkflowTask{pauseTask})

		// update lastBatchEndTask for next forEach item
		if pauseTask != nil {
			lastBatchEndTasks = []*rolloutv1alpha1.WorkflowTask{pauseTask}
		} else if postTasks != nil && len(postTasks) > 0 {
			lastBatchEndTasks = postTasks
		} else {
			lastBatchEndTasks = mainTasks
		}

		batchDetail.PauseSuspendTask = pauseTask
		batchDetail.BreakPointSuspendTask = breakPointTask
	}

	workflow.Spec.Tasks = make([]rolloutv1alpha1.WorkflowTask, 0)
	for i := range tasks {
		workflow.Spec.Tasks = append(workflow.Spec.Tasks, *tasks[i])
	}

	// add annotations
	workloadBasicInfos := make([]*types.WorkloadBasicInfo, 0)
	for _, wrapper := range workloadWrappers {
		// TODO: check workload type here
		workloadBasicInfos = append(workloadBasicInfos, &types.WorkloadBasicInfo{
			Name:    wrapper.GetObj().GetName(),
			Kind:    wrapper.GetTypeMeta().Kind,
			Cluster: wrapper.GetCluster(),
		})
	}
	workloadBasicInfoBytes, _ := json.Marshal(workloadBasicInfos)
	workflow.Annotations[api.AnnoWorkflowWorkloadBasicInfo] = string(workloadBasicInfoBytes)
	initRolloutWorkflowInfo(instance, workflow, batchDetails)

	return workflow, nil
}

func initRolloutWorkflowInfo(instance *rolloutv1alpha1.Rollout, workflow *rolloutv1alpha1.Workflow, batchDetails []*types.UpgradeBatchDetail) {
	stages := make([]rolloutv1alpha1.Stage, 0)

	for i := range batchDetails {
		if batchDetails[i].BreakPointSuspendTask != nil {
			curStage := rolloutv1alpha1.Stage{
				Type:      rolloutv1alpha1.StageTypeSuspend,
				Name:      batchDetails[i].BreakPointSuspendTask.Name,
				NextStage: batchDetails[i].BatchName,
			}
			if i > 0 {
				curStage.PrevStage = batchDetails[i-1].BatchName
			}
			stages = append(stages, curStage)
		}
		stages = append(stages, rolloutv1alpha1.Stage{
			Type: rolloutv1alpha1.StageTypeBatch,
			Name: batchDetails[i].BatchName,
		})
		if batchDetails[i].PauseSuspendTask != nil && i < len(batchDetails)-1 {
			stages = append(stages, rolloutv1alpha1.Stage{
				Type:      rolloutv1alpha1.StageTypeSuspend,
				Name:      batchDetails[i].PauseSuspendTask.Name,
				PrevStage: batchDetails[i].BatchName,
				NextStage: batchDetails[i+1].BatchName,
			})
		}
	}

	info := rolloutv1alpha1.WorkflowInfo{
		Name: workflow.Name,
	}
	instance.Status.Stages = stages
	instance.Status.WorkflowInfo = info
	stageBytes, _ := json.Marshal(stages)
	workflow.Annotations[api.AnnoWorkflowBatchInfo] = string(stageBytes)
}
