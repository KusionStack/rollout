# RolloutRun Controller 目录逻辑说明

## 概述

RolloutRun Controller 负责执行单次渐进式发布过程（RolloutRun），是 Rollout 的一次具体运行实例。它通过 Executor 来编排 Canary（金丝雀）和 Batch（分批）两种发布策略的执行流程。

## 核心职责

1. **执行发布流程** - 管理 RolloutRun 生命周期状态转换
2. **协调执行器** - 调用 Executor 执行具体发布逻辑
3. **等待工作负载就绪** - 监控工作负载状态，确保发布安全
4. **管理流量路由** - 协调 TrafficManager 处理流量切换
5. **支持 Webhook 钩子** - 在关键节点调用外部 Webhook

## 目录结构

```
rolloutrun/
├── rolloutrun_controller.go    # 主控制器逻辑
├── initializer.go              # 控制器初始化
└── executor/                   # 执行器模块
    ├── default.go              # 主执行器
    ├── canary.go               # 金丝雀发布执行器
    ├── batch.go                # 分批发布执行器
    ├── context.go              # 执行上下文
    ├── step_lifecycle.go       # 步骤生命周期
    ├── command.go              # 命令处理
    └── webhook/                # Webhook 模块
        ├── manager.go          # Webhook 管理器
        ├── worker.go           # Webhook 工作线程
        └── do_hook.go          # Webhook 执行器
```

## 核心数据结构

### RolloutRunReconciler

```go
type RolloutRunReconciler struct {
    *mixin.ReconcilerMixin
    workloadRegistry registry.WorkloadRegistry
    rvExpectation   expectations.ResourceVersionExpectationInterface
    executor        *executor.Executor
}
```

### Executor

```go
type Executor struct {
    logger logr.Logger
    canary  *canaryExecutor   // 金丝雀执行器
    batch   *batchExecutor    // 分批执行器
}
```

## Reconcile 流程

### SetupWithManager

```go
func (r *RolloutRunReconciler) SetupWithManager(mgr ctrl.Manager) error
```

监听的资源：
- `RolloutRun` - 使用 `ResourceVersionChangedPredicate` 和 `RolloutClassMatchesPredicate`

### Reconcile 步骤

```go
func (r *RolloutRunReconciler) Reconcile(ctx context.Context, req ctrl.Request)
```

1. **获取 RolloutRun 对象**
2. **检查期望满足** - 资源版本期望
3. **管理 Finalizer**
4. **跳过已完成** - `IsCompleted()` 则直接返回
5. **查找工作负载** (跨集群) - 普通负载 + 金丝雀负载
6. **同步 RolloutRun** - 调用 Executor.Do()
7. **清理 Annotation**
8. **更新状态**

## 生命周期状态机

```
┌──────────────┐
│  Initial     │
└──────────────┘
       ↓
┌──────────────┐
│ PreRollout   │ ← PreRolloutHook
└──────────────┘
       ↓
┌──────────────┐
│ Progressing  │ ← 执行 Canary + Batch
└──────────────┘
       ↓
┌──────────────┐
│ PostRollout  │ ← PostRolloutHook
└──────────────┘
       ↓
┌──────────────┐
│ Succeeded    │ ← 终态
└──────────────┘

┌──────────────┐
│ Pausing      │ ──→ Paused ← 暂停/继续命令
└──────────────┘

┌──────────────┐
│ Canceling    │ ──→ Canceled ← 终态
└──────────────┘
```

## Executor 执行流程

### 生命周期处理

```go
func (e *Executor) lifecycle(ctx *ExecutorContext) (done bool, result ctrl.Result, err error)
```

1. **命令优先检查** - 如果存在手动命令，先执行命令
2. **删除检测** - 触发 Canceling 状态
3. **状态转换**

### Processing 处理

```go
func (e *Executor) doProcessing(ctx *ExecutorContext) (bool, ctrl.Result, error)
```

按顺序执行：
1. ** Canary 发布** (如果配置)
2. **Batch 分批发布**

## Canary 发布状态机

```
┌──────────────┐
│  None        │
└──────────────┘
       ↓
┌──────────────┐
│  Pending     │
└──────────────┘
       ↓
┌──────────────┐
│PreCanaryHook │ ← 调用 PreCanaryStepHook Webhook
└──────────────┘
       ↓
┌──────────────┐
│  Running     │ ──→ 流量分叉 ──→ 创建金丝雀资源 ──→ 添加金丝雀路由
└──────────────┘                │                │              │
                               ↓                ↓              ↓
                            等待就绪        等待就绪      等待就绪
       ↓
┌──────────────┐
│PostCanaryHook│ ← 调用 PostCanaryStepHook Webhook (完成后暂停)
└──────────────┘
       ↓
┌──────────────┐
│ResourceRecyc │ ← 删除金丝雀路由 ──→ 删除金丝雀资源 ──→ 重置路由 ──→ 删除分叉后端
└──────────────┘
       ↓
┌──────────────┐
│  Succeeded   │
└──────────────┘
```

### Canary 执行步骤

| 步骤 | 操作 | 说明 |
|------|------|------|
| `doInit()` | 初始化 | 添加金丝雀标记，初始化流量管理 |
| `doPreStepHook()` | 前置钩子 | 调用外部 Webhook |
| `doCanary()` | 金丝雀执行 | 1. Fork 流量后端 <br> 2. 初始化路由 <br> 3. 创建金丝雀资源 <br> 4. 添加金丝雀路由 |
| `doPostStepHook()` | 后置钩子 | 调用外部 Webhook (完成后自动暂停) |
| `release()` | 资源回收 | 删除金丝雀路由、资源、流量后端 |

## Batch 发布状态机

```
┌──────────────┐
│  None        │
└──────────────┘
       ↓
┌──────────────┐
│  Pending     │
└──────────────┘
       ↓
┌──────────────┐
│PreBatchHook  │ ← 调用 PreBatchStepHook Webhook
└──────────────┘
       ↓
┌──────────────┐
│  Running     │ ──→ 更新 Partition ──→ 等待就绪
└──────────────┘
       ↓
┌──────────────┐
│PostBatchHook │ ← 调用 PostBatchStepHook Webhook
└──────────────┘
       ↓
┌──────────────┐
│ResourceRecyc │ ← 清理进度标记 (最后一批)
└──────────────┘
       ↓
┌──────────────┐
│  Succeeded   │
└──────────────┘
```

### Batch 执行步骤

| 步骤 | 操作 | 说明 |
|------|------|------|
| `doPausing()` | 暂停检查 | 初始化进度信息，检查 breakpoint |
| `doPreStepHook()` | 前置钩子 | 调用外部 Webhook |
| `doBatchUpgrading()` | 分批升级 | 更新 Partition，等待副本就绪 (支持滑动窗口) |
| `doPostStepHook()` | 后置钩子 | 调用外部 Webhook |
| `doRecycle()` | 资源回收 | 清理进度标记 (仅最后一批) |

## 流量管理

Canary 发布涉及的流量操作：

| 操作 | 说明 |
|------|------|
| `ForkBackends` | 分叉流量后端 |
| `InitializeRoute` | 初始化路由 |
| `AddCanaryRoute` | 添加金丝雀路由 |
| `DeleteCanaryRoute` | 删除金丝雀路由 |
| `ResetRoute` | 重置路由 |
| `DeleteForkedBackends` | 删除分叉的后端 |

## 手动命令

| 命令 | 说明 |
|------|------|
| `pause` | 暂停发布 |
| `resume`/`continue` | 从暂停状态继续 |
| `retry` | 从错误状态重试 |
| `skip` | 跳过当前批次 (错误时) |
| `cancel` | 取消发布 |
| `forceSkipCurrentBatch` | 强制跳过当前批次 |

## Webhook 系统

### 支持的钩子类型

| HookType | 触发时机 |
|----------|----------|
| `PreCanaryStepHook` | 金丝雀发布前 |
| `PostCanaryStepHook` | 金丝雀发布后 (完成后暂停) |
| `PreBatchStepHook` | 每个批次开始前 |
| `PostBatchStepHook` | 每个批次完成后 |

### Webhook 协议

请求: `RolloutWebhookReview`
返回状态: `OK`/`Error`/`Processing`

## Step 状态引擎

```go
type stepStateEngine struct {
    lifecycle []stepLifecycle
}
```

每个步骤定义 `do()` 和 `cancel()` 函数：
- `do()` 返回 `(done bool, retry time.Duration, error)`
- `cancel()` 支持优雅取消

## Control 模块

### BatchReleaseControl

| 方法 | 说明 |
|------|------|
| `Initialize()` | 预检查，添加进度标记 |
| `UpdatePartition()` | 更新工作负载 partition |
| `Finalize()` | 删除进度标记 |

### CanaryReleaseControl

| 方法 | 说明 |
|------|------|
| `Initialize()` | 预检查，添加进度标记 |
| `Finalize()` | 删除金丝雀资源，清理标记 |
| `CreateOrUpdate()` | 创建或更新金丝雀资源 |

## 关键设计决策

1. **状态机模式** - 使用状态机管理复杂的状态转换
2. **执行器分离** - Canary 和 Batch 独立执行器，支持组合
3. **Webhook 扩展** - 提供标准化的 Webhook 集成点
4. **滑动窗口** - Batch 支持渐进式副本调整
5. **流量解耦** - 通过 TrafficManager 抽象流量操作

## Annotation 用途

| Annotation | 说明 |
|------------|------|
| `rollout.kusionstack.io/manual-command` | 手动命令传递 |

## Finalizer 用途

```
rollout.kusionstack.io/cleanup          # RolloutRun 保护
rollout.kusionstack.io/canary Protection # 金丝雀资源保护
```

## ExecutorContext

```go
type ExecutorContext struct {
    context.Context
    Client         client.Client
    Recorder       record.EventRecorder
    Accessor       workload.Accessor
    OwnerKind      string
    OwnerName      string
    RolloutRun     *rolloutv1alpha1.RolloutRun
    NewStatus      *rolloutv1alpha1.RolloutRunStatus
    Workloads      *workload.Set
    TrafficManager *trafficcontrol.Manager
}
```

执行上下文封装了执行所需的所有资源。

## 参考

- [渐进式交付 Rollout 设计文档](https://yuque.antfin.com/antcloud-paas/dp4wap/ae5myagvwh96cugn)
- Rollout Controller - 负责触发和管理 RolloutRun
- Traffic Control - 流量路由管理模块
- Workload Registry - 工作负载适配层