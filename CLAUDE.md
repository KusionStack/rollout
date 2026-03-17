# CLAUDE.md - Rollout 开发上下文

## 快速参考

| 主题 | 规则文件                                     |
|------|------------------------------------------|
| 编码规范 | `.claude/rules/01-coding-conventions.md` |
| 测试规范 | `.claude/rules/02-testing.md`            |
| 代码审查 | `.claude/rules/03-code-review.md`        |

---

## Claude 开发工作流 (SDD)

使用 Specification-Driven Development (SDD) 方式与 Claude 协作开发，确保开发过程有序可控。

**⚠️ 绝对禁止：在任何阶段未完成时进入下一阶段。Claude 必须等待用户明确确认后才能继续。**

**推荐技能：**

```bash
/superpowers:brainstorming         # 需求讨论和设计
/superpowers:writing-plans         # 编写设计文档
/superpowers:using-git-worktrees   # 创建隔离工作区
/superpowers:test-driven-development # TDD开发
/superpowers:requesting-code-review # 代码审查
/skill create-dima-issue           # 创建 DIMA 工单
/skill gitpr                       # 创建 PR
```

---

### Phase 1: 需求澄清 (Requirement Clarification)

**触发条件**：用户提出任何需求、想法或问题时

**Claude 必须执行：**
1. 使用 `/superpowers:brainstorming` skill 启动头脑风暴
2. 通过提问充分理解需求，一次只问一个问题
3. 明确功能目标、验收标准、影响范围

**完成标准**：
- [ ] 需求已充分澄清
- [ ] 验收标准已定义
- [ ] 用户确认需求理解正确

**阶段输出**：需求澄清总结（文字描述即可）

**用户确认话术**："需求澄清完毕，请确认我的理解是否正确：[总结]。确认无误后，我将进入 Dima Issue 创建阶段。"

**⚠️ 禁止**：未完成需求澄清前进入 Dima 创建阶段

---

### Phase 2: Dima Issue 关联 (Issue Tracking)

**触发条件**：需求澄清完成且用户确认后

**Claude 必须执行：**
1. 询问用户是否已有 Dima Issue
2. 如无，调用 `/skill create-dima-issue` 创建
3. 记录 Dima ID，后续所有工作必须关联此 ID

**完成标准**：
- [ ] Dima Issue 已存在或已创建
- [ ] Dima ID 已记录

**阶段输出**：Dima Issue 链接/ID

**用户确认话术**："Dima Issue 已关联：[DIMA-XXXX]。确认后，我将进入方案设计阶段。"

**⚠️ 禁止**：无 Dima Issue 关联时进入设计阶段

---

### Phase 3: 方案设计 (Design)

**触发条件**：Dima Issue 关联完成后

**Claude 必须执行：**
1. 输出设计文档到 `docs/plans/YYYY-MM-DD-<feature>.md`，包含：
   - 背景和目标
   - 技术方案（含接口设计、数据模型变更）
   - 影响分析
   - 风险和对策
2. 使用 `/superpowers:writing-plans` skill 辅助生成规范的计划文档

**完成标准**：
- [ ] 设计文档已创建并写入文件
- [ ] 技术方案已详细描述
- [ ] 用户已审阅设计文档

**阶段输出**：`docs/plans/YYYY-MM-DD-<feature>.md`

**用户确认话术**："设计方案已输出到 [文件路径]，请审阅。确认无误后，我将进入开发阶段。"

**⚠️ 禁止**：设计文档未经用户确认前编写任何业务代码

---

### Phase 4: 开发实现 (Implementation)

**触发条件**：设计方案经用户确认后

**4.1 环境准备**
- 询问用户是否需要创建隔离工作区以及独立分支
- 若需要，使用 `/superpowers:using-git-worktrees` 创建隔离工作区
- 切换到独立分支

**4.2 TDD 开发流程**

**Step 1: Red - 编写失败的测试**
- 先编写测试用例（覆盖正常路径、边界条件、错误处理）
- 运行测试确保失败（验证测试有效性）
- **检查点**：测试文件已创建，测试失败符合预期

**Step 2: Green - 最小化实现**
- 编写刚好让测试通过的最小代码
- 运行测试确保通过
- **检查点**：所有测试通过

**Step 3: Refactor - 重构优化**
- 优化代码质量
- 确保测试仍然通过
- 运行 `make lint` 确保规范
- 运行 `go build -o bin/manager kusionstack.io/rollout/cmd/rollout` 确保编译通过
- 运行 `go test` 确保测试通过
- **检查点**：确保编译测试全部通过

**完成标准**：
- [ ] 单元测试已编写并通过
- [ ] 实现代码已完成
- [ ] 编译测试全部通过

**阶段输出**：实现代码 + 测试代码

---

### Phase 5: 代码审查与提交 (Review & Commit)

**触发条件**：开发实现完成后

**Claude 必须执行：**
1. 使用 `/superpowers:requesting-code-review` 进行自我审查
2. 使用 `/skill gitpr` 创建 PR
3. PR 描述中必须包含：`Relates to: DIMA-XXXX`

**PR 格式要求：**
```
[DIMA-XXXX] <type>(<scope>): <subject>

- 变更点 1
- 变更点 2

Relates to: DIMA-XXXX
```

**完成标准**：
- [ ] 代码审查完成
- [ ] PR 已创建并关联 Dima Issue

**阶段输出**：PR 链接

---

### 阶段转换检查清单

**每个阶段转换前，Claude 必须：**

1. **明确询问用户**："[当前阶段] 已完成，是否进入 [下一阶段]？"
2. **等待用户明确回复** "是/确认/可以" 后才能继续
3. **不允许假设用户同意**

**阶段依赖关系：**
```
需求澄清 → Dima关联 → 方案设计 → 开发实现 → 代码审查
   ↑           ↑          ↑          ↑          ↑
必须等待     必须等待   必须等待   必须等待   必须等待
用户确认     用户确认   用户确认   用户确认   用户确认
```

---

### Rule: 严格禁止的行为

| 禁止行为 | 违反后果 |
|---------|---------|
| 跳过用户确认进入下一阶段 | 可能导致方向错误，需回滚重做 |
| Planning 完成前编写业务代码 | 违反 SDD 原则 |
| 无 Dima Issue 关联进行开发 | 无法追溯需求来源 |
| 一次提交超过 500 行代码（不含测试） | 难以 Review，容易引入 Bug |
| 修改与当前功能无关的代码 | 引入不可控变更 |
| 跳过测试直接提交 | 质量无法保证 |
| 假设用户同意而非明确确认 | 可能误解需求 |

---

### 工作流程总结

```
┌─────────────────────────────────────────────────────────────────┐
│                     强制用户确认点                                │
├─────────────────────────────────────────────────────────────────┤
│ 1. 需求澄清 → [等待用户确认: 理解是否正确?] → Dima关联            │
│ 2. Dima关联 → [等待用户确认: Issue是否创建?] → 方案设计          │
│ 3. 方案设计 → [等待用户确认: 设计文档是否可接受?] → 开发实现     │
│ 4. 开发实现 → [TDD循环: Red→Green→Refactor] → 代码审查          │
│ 5. 代码审查 → [等待用户确认: 是否创建PR?] → 完成                 │
└─────────────────────────────────────────────────────────────────┘
```
