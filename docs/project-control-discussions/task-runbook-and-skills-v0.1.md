# Task Runbook And Skills v0.1

## 1. 设计目标

本文件补齐 Task 管理和多个 agent 之间的操作逻辑缺口。

已有文档已经定义：

- `Project / Workstream / Task / Session / Runtime` 的对象层级
- `Policy`、`Checkpoint`、`Claim / Review / Decision` 的控制与审计链
- agent-neutral 的 Agent 协议与 adapter 方向

但还需要明确：

> 一个 Task 如何在多个 agent、多个 session、多个工具能力之间按固定流程闭环执行，并且只在真正需要人类判断时通知人。

核心目标不是让 agent 自由发挥，而是让系统用可执行流程、规则和证据约束 agent。

## 2. 核心分工

建议把任务执行分成四层：

```text
Task = 要完成什么
Skill / Runbook = 按什么流程完成
Policy = 什么权限、预算、质量门槛、何时升级人类
Session = 某个 agent 执行某一步
```

对应关系：

| 概念 | 作用 | 不负责 |
|---|---|---|
| `Task` | 业务目标、范围、风险、状态、验收生命周期 | 不写具体执行步骤 |
| `Skill` | 可复用的任务类型能力包，例如 bug fix、refactor、test repair | 不决定是否越权 |
| `Runbook` | Skill 里的可执行阶段流程，例如 plan → implement → test → review | 不绕过 Policy |
| `Policy` | 权限、预算、质量门槛、审批和升级规则 | 不替代具体执行方法 |
| `Session` | 某个 agent 在某个 runtime/workspace 上执行某一阶段 | 不拥有项目级自治权 |
| `Tool Gateway` | 统一代理 MCP、本地工具、文件系统、测试 runner 等能力 | 不让 agent 直接绕过审计 |

### 2.1 Runbook 与 Skill 的最终边界

Runbook 和 Skill 不应二选一。

更合理的架构是：

```text
Project / Workstream / Task
        ↓
Runbook / State Machine：规定任务必须怎么推进
        ↓
Phase：plan / implement / test / review / fix_or_replan / final_validation
        ↓
Skill：每个 phase 具体怎么做、需要什么工具、产出什么 artifact
        ↓
Agent / Runtime：谁来执行
        ↓
Artifact / Evidence / Checkpoint：怎么证明完成，什么时候需要人批准
```

因此：

- Runbook 是控制内核，负责阶段顺序、状态转换、循环、完成规则和 gate。
- Skill 是执行策略，负责某类任务的做法、工具建议、artifact schema、常见失败恢复路径。
- Policy 是硬约束，负责权限、预算、风险升级和审批边界。
- Agent 只执行某个 phase，不拥有跳过 gate 或改变全局状态的权力。
- Artifact / Evidence / Checkpoint 是责任链，不是 agent final answer 的附属物。

不能只用 Skill 取代 Runbook，原因是 Skill 不天然负责全局状态、审计链、跨 agent 协同、权限边界和最终验收。否则系统会退回到“agent 说它做完了”的模式。

也不能只保留一个固定 Runbook，原因是不同任务类型需要不同流程。代码修改、文档更新、研究任务、依赖升级、前端视觉修改、基础设施变更不应被强行套进同一个 phase 列表。

推荐结论：

> Runbook 管流程，Skill 管方法，Policy 管边界，Agent 管执行，Artifact / Checkpoint 管证据和责任。

## 3. Task 执行闭环

默认代码任务不应是“agent 一次性做完并报告”，而应是可回放的阶段循环：

```text
Plan
-> Implement
-> Test
-> Review
-> Fix / Replan
-> Test
-> Review
-> Final Validation
-> Ready For Human Acceptance
```

直到满足 completion rules，才允许进入人类验收。

最小闭环：

1. `plan`
   - 只读 repo、docs、issue、现有 diff
   - 输出 plan artifact
   - 不改代码

2. `implement`
   - 启动 worker session
   - 只能写 task scope 内允许路径
   - 输出 changed files、diff summary、implementation notes

3. `test`
   - 运行相关测试、构建、静态检查
   - 输出 test artifact
   - 失败时不能直接进入 ready_for_acceptance

4. `review`
   - 启动 reviewer session 或验证 session
   - 默认只读，或使用隔离 worktree
   - 输出 Review，判断 claim 是否成立、证据是否足够、风险是否低估

5. `fix_or_replan`
   - 如果测试失败、review objection、证据不足或任务漂移：
     - 更新 plan
     - 记录 failure artifact
     - 回到 implement/test/review

6. `final_validation`
   - 检查 completion rules、quality gates、review rules
   - 输出 `Claim(type=ready_for_acceptance)`

7. `ready_for_acceptance`
   - `Task.acceptance_status -> ready_for_acceptance`
   - 创建或准备 `Checkpoint(trigger_type=final_acceptance)`
   - 通知人类进入最终验收

## 4. Skill

### 4.1 定义

`Skill` 是 agent-neutral 的可复用过程包，不绑定某一家 agent。

OpenClaw 的 `SKILL.md`、skill discovery、per-agent allowlist、ClawHub registry 值得借鉴，但 RoamBench 的 Skill 不能直接授予自治权。OpenClaw 对比见 [openclaw-comparison-v0.1.md](./openclaw-comparison-v0.1.md)。

Skill 应描述：

- 适用任务类型
- 默认 runbook
- 所需工具能力
- 输入和输出 schema
- 必须产出的 artifact
- 推荐 reviewer 类型
- 常见失败恢复路径

Skill 不应描述：

- 项目全局目标
- 人类最终验收结果
- 绕过 policy 的权限例外
- 某个 agent 私有 prompt 细节

### 4.2 推荐内置 Skill

第一版建议内置以下 Skill：

| Skill | 用途 |
|---|---|
| `code_change` | 常规代码修改 |
| `bug_fix` | 复现、修复、验证 bug |
| `test_repair` | 修复失败测试或补测试 |
| `refactor` | 行为保持的结构调整 |
| `dependency_upgrade` | 依赖升级、兼容性验证 |
| `docs_update` | 文档更新 |
| `release_prepare` | 发布前检查与打包 |

### 4.3 Skill 与 Runbook 的关系

一个 Skill 可以有多个 Runbook。

Skill 不直接调度任务，也不直接推进状态。Skill 应作为 Runbook 的配置来源和执行说明来源。

也就是说：

```text
Task.selected_skill
-> Skill Registry
-> default_runbook / allowed_runbooks
-> Runbook phases
-> PhaseAttempt
-> Artifact / Review / Checkpoint
```

这样可以保持 RoamBench agent-neutral：所有 agent 使用同一套 task、runbook、artifact、checkpoint 文件体系；不同 agent 的差异只体现在 runtime adapter、tool capability 和执行质量上。

示例：

```yaml
skill:
  id: code_change
  version: "0.1"
  default_runbook: code_change_default
  required_artifacts:
    - plan
    - diff_summary
    - test_result
    - review_result
  recommended_reviewer: code_reviewer
```

更完整的 Skill 配置应表达：

```yaml
id: code_change
name: Code Change
version: "0.1"

default_runbook: code_change_default
allowed_runbooks:
  - code_change_default
  - code_change_fast_path
  - code_change_high_risk

permissions:
  plan: read_only
  implement: scoped_write
  test: read_only
  review: read_only
  fix_or_replan: scoped_write
  final_validation: read_only

artifacts:
  plan:
    required: true
    outcome: recorded
  diff_summary:
    required: true
    outcome: recorded
  test_result:
    required: true
    outcome: pass
  review_result:
    required: true
    outcome: pass
  completion_check:
    required: true
    outcome: pass

rules:
  - implementation_requires_plan
  - final_acceptance_requires_all_artifacts
  - failed_test_routes_to_fix_or_replan
  - failed_review_routes_to_fix_or_replan
```

不同 Skill 可以选择不同 Runbook：

| Skill | 典型 Runbook |
|---|---|
| `code_change` | plan → implement → test → review → final_validation |
| `docs_update` | plan → write → review → final_validation |
| `research` | scope → collect → synthesize → review |
| `dependency_upgrade` | plan → upgrade → compatibility_test → rollback_plan → review |
| `frontend_change` | design_check → implement → visual_check → test → review |
| `infra_change` | plan → implement → smoke_test → rollback_plan → review → final_validation |

## 5. Runbook

### 5.1 结构

Runbook 是可执行阶段定义。

示例：

```yaml
runbook:
  id: code_change_default
  version: "0.1"

  phases:
    - id: plan
      execution_role: implement
      write_access: false
      required_artifacts: [plan]

    - id: implement
      execution_role: implement
      write_access: scoped
      required_artifacts: [diff_summary]

    - id: test
      execution_role: test
      write_access: false
      required_artifacts: [test_result]

    - id: review
      execution_role: review
      write_access: false
      required_artifacts: [review_result]

    - id: fix_or_replan
      execution_role: implement
      write_access: scoped
      when:
        any:
          - tests_failed
          - high_severity_review_objection
          - evidence_incomplete

    - id: final_validation
      execution_role: verify
      write_access: false
      required_artifacts: [completion_check]

  loop_until:
    - relevant_tests_pass
    - no_high_severity_review_objection
    - evidence_complete

  notify_human_only_on:
    - final_acceptance
    - checkpoint
    - budget_exceeded
    - repeated_failure
    - goal_ambiguity
```

### 5.2 Phase Attempt

每次执行 phase 都应生成一次 attempt 记录，至少包含：

- `task_id`
- `phase_id`
- `session_id`
- `agent_type`
- `runtime_id`
- `workspace_ref`
- `started_at`
- `completed_at`
- `status`
- `artifacts`
- `failure_reason`

这可以作为独立表，也可以先作为 `Event + Artifact` 的组合实现。

## 6. Policy 与 Runbook 的边界

Runbook 负责“应该按什么流程做”。

Policy 负责“是否允许继续做”。

规则优先级：

```text
Escalation Rules
> Approval Rules
> Review Rules
> Quality Gates
> Completion Rules
> Autonomy Limits（调度类）
```

硬安全约束始终前置：

- `writable_paths`
- `protected_paths`
- `allowed_runtimes`
- `internet_access`
- destructive command policy
- workspace isolation requirement

因此：

- Skill 不能授予权限
- Runbook 不能绕过 checkpoint
- Agent 不能因为 prompt 要求而越权
- Tool Gateway 必须执行 Policy

## 7. 权限模型

权限不应是“给足够权限”，而应是“按阶段给最小足够权限”。

推荐默认：

| Phase | 文件权限 | 工具权限 | 说明 |
|---|---|---|---|
| `plan` | read-only | read repo, inspect history | 不改代码 |
| `implement` | scoped write | edit, git diff, local tests | 只写 task scope |
| `test` | read-only 或 test-output write | test/build runner | 不做新功能改动 |
| `review` | read-only | diff/test/log inspection | reviewer 默认不写 |
| `fix_or_replan` | scoped write | edit + relevant tests | 只修复已识别问题 |
| `final_validation` | read-only | tests/build/evidence checks | 不引入新改动 |

如果需要扩大权限，应创建 `Checkpoint`，而不是静默升级。

## 8. 共享文件体系与 workspace

agent-neutral 不等于所有 agent 无差别共享同一个可写目录。

建议把 workspace 显式建模：

```text
workspace_ref = shared_repo
workspace_ref = isolated_worktree
workspace_ref = read_only_snapshot
```

规则：

- Worker 可以在 `shared_repo` 或 `isolated_worktree` 写 scoped paths。
- Reviewer 默认使用 `read_only_snapshot` 或只读 worktree。
- 多个写 session 不允许无隔离修改同一文件集合。
- 高风险路径必须触发 checkpoint。
- 跨模块重构、大规模迁移、删除必须触发 checkpoint 或进入隔离 worktree。
- 所有文件变更必须沉淀为 Artifact，而不是只存在终端输出中。

## 9. Tool Gateway 与 MCP

可以借鉴 MCP，但不应把 MCP 直接暴露成产品核心。

Hermes 这类 agent runtime 也应遵守同一边界：它可以通过 adapter 使用 MCP、skills、memory 和 runtime tools，但所有工具调用、权限扩大、证据沉淀和状态推进都必须回到 RoamBench 的 Tool Gateway 与 canonical records。详细比较见 [hermes-agent-comparison-v0.1.md](./hermes-agent-comparison-v0.1.md)。

建议架构：

```text
Agent
-> RoamBench Adapter
-> Tool Gateway
-> MCP servers / local tools / runtime tools / filesystem / test runner
```

Tool Gateway 负责：

- 工具发现
- capability 映射
- policy enforcement
- permission grant 检查
- audit event 记录
- artifact capture
- checkpoint 触发

MCP 适合作为工具扩展标准：

- 文件资源
- repo 状态
- test runner
- browser / network tool
- issue / PR / CI provider
- docs / knowledge resources

但 RoamBench 的 canonical record 仍应是：

- `Event`
- `Artifact`
- `Claim`
- `Review`
- `Decision`
- `Checkpoint`

MCP 返回的数据应被转换成这些 canonical records，而不是成为第二套历史来源。

## 10. Schedule Rules

定时规则应和 Task / Session / Phase 状态绑定，不只是 cron。

OpenClaw 的 cron、heartbeat、standing orders 说明 always-on agent 需要稳定触发机制。RoamBench 应借鉴触发机制，但触发结果必须创建或推进 `Task / PhaseAttempt / Checkpoint`，而不是只向聊天线程发送一条 announce。

推荐第一版：

```yaml
schedule_rules:
  no_progress_minutes: 30
  session_review_minutes: 90
  stale_waiting_review_hours: 4
  retry_backoff_minutes: [5, 15, 30]
  daily_project_digest: "18:00"
```

触发行为：

| 规则 | 行为 |
|---|---|
| `no_progress_minutes` | 触发 review 或 checkpoint |
| `session_review_minutes` | 将 session 推入 review / summary 阶段 |
| `stale_waiting_review_hours` | 升级给人类或重新派 reviewer |
| `retry_backoff_minutes` | 控制失败重试节奏 |
| `daily_project_digest` | 生成项目摘要，不等于审批打断 |

Schedule Rules 不应绕过 Policy。

如果定时规则触发人类处理，应物化成 `Checkpoint`。

## 11. 通知规则

默认不应每个阶段都通知人类。

通知人类的 canonical 入口应是 `Checkpoint` 或最终验收。

推荐通知条件：

- `final_acceptance`
- destructive command
- protected path change
- budget exceeded
- repeated failure `>= 3`
- reviewer conflict
- goal ambiguity
- scope expansion
- no progress timeout
- policy migration request

非通知但应记录的内容：

- phase started / completed
- test result
- review result
- retry
- local fix loop
- evidence update

这些应写入 `Event` / `Artifact`，不应打断用户。

## 12. 建议数据模型增量

第一版不一定全部落表，但概念上应预留：

```text
Skill
Runbook
RunbookPhase
PhaseAttempt
ToolCapability
PermissionGrant
ScheduleRule
WorkspaceRef
```

与现有对象关系：

```text
Project
  -> Policy
  -> Skill Registry
  -> Schedule Rules

Task
  -> selected_skill
  -> runbook_id
  -> runbook_state
  -> policy_version_id

Session
  -> phase_attempt
  -> agent_type
  -> execution_role
  -> system_role
  -> workspace_ref
  -> tool_capabilities

Tool Gateway
  -> MCP tools
  -> local filesystem
  -> test runner
  -> git
  -> browser / network if allowed
```

## 13. 默认 Code Change Runbook

第一版建议内置一个默认 runbook：

```yaml
runbook:
  id: code_change_default
  skill: code_change

  phases:
    - plan
    - implement
    - test
    - review
    - fix_or_replan
    - final_validation
    - ready_for_acceptance

  completion_rules:
    require_plan_artifact: true
    require_diff_summary: true
    require_relevant_tests_pass: true
    require_review_result: true
    block_on_high_severity_objection: true

  loop:
    max_fix_iterations: 3
    on_test_failure: fix_or_replan
    on_review_objection: fix_or_replan
    on_repeated_failure: checkpoint

  permissions_by_phase:
    plan: read_only
    implement: scoped_write
    test: read_only
    review: read_only
    fix_or_replan: scoped_write
    final_validation: read_only

  notify_human:
    - final_acceptance
    - checkpoint
```

## 14. 推荐落地顺序

当前实现方向应继续保留 runbook / evidence / checkpoint 作为主干，不应推倒重做成纯 skills。

推荐推进顺序：

1. 保持当前 `current_phase`、`runbook_state`、`phase_attempts`、`artifacts`、`missing_evidence`、`checkpoint` 作为控制内核。
2. 新增 `Skill Registry` 和 `Runbook Registry`。
3. 把当前硬编码的 `code_change_default` runbook 挪成 skill-backed runbook。
4. 允许创建 Task 时选择 `selected_skill`，默认使用 `code_change`。
5. 让 `required_artifacts`、phase 权限和 completion rules 从 Skill / Runbook 定义读取。
6. 增加 `docs_update`、`research`、`frontend_change`、`dependency_upgrade` 等 Skill。
7. 再把 MCP、runtime tools、本地测试、浏览器、CI、issue tracker 接入 Tool Gateway，作为 Skill 可声明的 capability，而不是让它们成为任务状态系统。

这条路径可以避免两个极端：

- 只靠固定 Runbook：流程过硬，无法适配不同任务类型。
- 只靠 Skill：状态、证据、权限和验收不可控。

## 15. 最终判断

RoamBench 不应做 agent 群聊平台，也不应把 skills 变成 agent 私有 prompt 集。

更合理的定位是：

> Runbook 负责按什么流程干，Skills 负责怎么干，Policy 负责能不能干，Tool Gateway 负责用什么工具干，Checkpoint 负责什么时候必须问人。

这样才能保持 agent-neutral，同时让不同 agent 共用一套文件体系、证据体系、权限体系和历史体系。

自动开发也应遵守这个边界：它不是一个 `auto_develop` 大 skill，而是由 Skill、Runbook、Policy、WorkspaceRef、Artifact、Review、Decision、Checkpoint 共同组成的项目执行系统。
