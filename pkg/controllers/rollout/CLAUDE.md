# Rollout Controller 目录逻辑说明

## 概述

Rollout Controller 是 KusionStack Rollout 的核心控制器之一，负责管理 Rollout 资源的生命周期，协调工作负载的渐进式发布流程。基于 [渐进式交付 Rollout 设计文档](https://yuque.antfin.com/antcloud-paas/dp4wap/ae5myagvwh96cugn)，Rollout Controller 实现了跨集群的发布管理功能。

## 核心职责

1. **管理 Rollout 生命周期** - 监听 Rollout 资源变化，计算状态
2. **关联工作负载** - 根据 WorkloadRef 匹配并标记受管工作负载
3. **触发发布流程** - 根据触发策略创建 RolloutRun
4. **协调 RolloutRun** - 监控 RolloutRun 状态，处理用户命令
5. **资源清理** - 清理历史 RolloutRun，处理 Finalizer

## 目录结构

```
rollout/
├── rollout_controller.go    # 主控制器逻辑
├── event_handler.go         # 事件处理器
├── utils.go                 # 工具函数
└── initializer.go           # 控制器初始化
```

## 核心数据结构

### RolloutReconciler

```go
type RolloutReconciler struct {
    *mixin.ReconcilerMixin                 // 混入基础控制器能力
    workloadRegistry registry.WorkloadRegistry  // 工作负载注册表
    expectation   expectations.ControllerExpectationsInterface  // 控制器期望
    rvExpectation expectations.ResourceVersionExpectationInterface  // 资源版本期望
}
```

## Reconcile 流程

### SetupWithManager

```go
func (r *RolloutReconciler) SetupWithManager(mgr ctrl.Manager) error
```

监听的资源：
- `Rollout` - 使用 `ResourceVersionChangedPredicate` 过滤
- `RolloutRun` (跨集群) - 使用创建观察处理器
- `RolloutStrategy` - 策略变化触发
- 所有已注册的 workload 类型 (Deployment, CloneSet, CollaSet 等)

### Reconcile 八步流程

```go
func (r *RolloutReconciler) Reconcile(ctx context.Context, req ctrl.Request)
```

1. **检查期望满足**
   - 资源版本期望
   - 控制器期望

2. **计算新状态**
   - 根据对象状态计算 Phase: `Initialized`/`Progressing`/`Disabled`/`Terminating`

3. **管理 Finalizer**
   - 添加/删除 `rollout.kusionstack.io/cleanup` finalizer

4. **查找工作负载** (跨集群)
   - 根据资源匹配规则查找关联的工作负载

5. **获取 RolloutRun 列表**
   - 当前运行的 RolloutRun
   - 历史完成的 RolloutRun

6. **清理历史记录**
   - 根据 `HistoryLimit` 删除旧的完成 RolloutRun

7. **按阶段处理**
   - `Disabled`: 仅同步状态
   - `Terminating`: 执行清理
   - `Progressing`/`Waiting`: 处理发布逻辑

8. **更新状态**
   - 更新 RolloutStatus

## 关键函数

### 状态计算

| 函数 | 说明 |
|------|------|
| `calculateStatus()` | 根据对象状态计算 Phase |
| `updateStatusOnly()` | 仅更新状态，不触发 reconcile 回头 |

### Finalizer 管理

| 函数 | 说明 |
|------|------|
| `ensureFinalizer()` | 添加/删除 Finalizer |
| `handleFinalizing()` | 清理工作负载标签 |

### 发布触发

| 函数 | 说明 |
|------|------|
| `shouldTrigger()` | 判断是否应该触发发布 |
| `syncRun()` | 同步 RolloutRun 创建/状态 |

### 触发策略

#### Auto (自动触发) - 默认
- 当所有关联的工作负载都标记为 `waiting for rollout` 时触发

#### Manual (手动触发)
- 禁止自动触发，完全由用户控制

#### Annotation (主动触发)
- 添加 annotation `rollout.kusionstack.io/trigger: "triggerName"`
- 强制创建 RolloutRun，即使工作负载未全部就绪

### 工作负载管理

| 函数 | 说明 |
|------|------|
| `findWorkloadsCrossCluster()` | 跨集群查找工作负载 |
| `ensureWorkloadsLabelAndAnnotations()` | 添加 rollout 管理标签 |

### 历史清理

| 函数 | 说明 |
|------|------|
| `leanupHistory()` | 根据 HistoryLimit 删除旧记录 |
| `getAllRolloutRun()` | 获取所有 RolloutRun |

### 命令处理

| 函数 | 说明 |
|------|------|
| `handleRunManualCommand()` | 处理用户手动命令 |
| `applyOneTimeStrategy()` | 应用一次性策略 |

## Phase 状态机

```
┌──────────────┐
│  Disabled    │ ← spec.disabled = true
└──────────────┘
       ↓
┌──────────────┐
│ Initialized  │ ← 默认初始状态
└──────────────┘
       ↓ (触发)
┌──────────────┐
│ Progressing  │ ← RolloutRun 运行中
└──────────────┘
       ↓
┌──────────────┐
│ Terminating  │ ← deletionTimestamp != nil
└──────────────┘
```

## Annotation 用途

| Annotation | 说明 |
|------------|------|
| `rollout.kusionstack.io/trigger` | 主动触发 RolloutRun |
| `rollout.kusionstack.io/manual-command` | 手动命令传递 |
| `rollout.kusionstack.io/one-time-strategy` | 一次性策略 |

## Label 用途

| Label | 说明 |
|-------|------|
| `rollout.kusionstack.io/workload` | 标记被 Rollout 管理的工作负载 |
| `rollout.kusionstack.io/disabled` | 跳过 Rollout 流程 |

## 事件记录

通过 `recordCondition()` 记录以下条件：

| Condition | 说明 |
|-----------|------|
| `Available` | 依赖资源是否就绪 |
| `Trigger` | 触发状态 |
| `Progressing` | 进度状态 |
| `Terminating` | 终止状态 |

## Finalizer 用途

```
rollout.kusionstack.io/cleanup
```

保证 Rollout 被删除时能正确清理工作负载的标签和 annotation。

## 关键设计决策

1. **期望机制** - 使用期望减少不必要的 reconcile
2. **ResourceVersion 过滤** - 只处理实际变化的资源
3. **跨集群支持** - 通过 clusterinfo 管理多集群
4. **createCommand 优先** - 处理命令优先于状态检查