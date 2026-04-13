# Implementation Plan v0.1

## 1. 实施总原则

不是推倒重来，而是"重新命名 + 重新抽象 + 增量迭代"。保留现有 terminal 能力作为 P3 执行层基础设施。

## 2. 12 周实施计划

### 第 1 阶段：第 1-2 周 — 定义与边界

**目标**：把产品定义固定，停止功能发散。

#### 完成清单

1. **定名称与叙事**：产品主语从 terminal 改成 task / session / runtime / checkpoint
2. **重写 README**：第一屏先说任务控制、agent-neutral、local+remote、timeline/evidence/replay
3. **画产品信息架构**：一页 IA 图，顶层双模式（Terminal / Project Panel）+ Project Panel 内层级导航
4. **明确 MVP 不做什么**：不做通用 IDE、不做多人协作编辑、不做 browser agent、不做企业权限、不做 marketplace、不做 DAG 第一版
5. **定名词表**：所有接口和数据库字段命名统一

**交付物**：新版 README 草稿、产品 IA 图、MVP spec v0.1、术语表

### 第 2 阶段：第 3-4 周 — 数据模型与本地 Runtime

**目标**：把数据模型搭起来，打通本地 runtime。

#### 数据层

实现基础对象（参见 data-model-v0.2.md）：

- Project
- Task
- Session
- Runtime
- Event
- Policy（最小 schema）
- Checkpoint（最小 record schema）

#### 本地 Runtime

1. **Local runtime launcher**：启动 agent 进程、绑定工作目录、采集 stdout/stderr
2. **Session state machine**：queued → starting → active → {waiting_review, waiting_human, paused, reconnecting, crashed, completed, terminated}（参见 data-model-v0.2.md Section 6.3）
3. **Event ingestion**：所有关键动作写成 event
4. **Timeline 页面**：每个 event 显示时间、actor、action、target、result、risk、next step
5. **Evidence 页面**：changed files、diff summary、test results、recent commands
6. **Terminal attach**：保留但只放在 Task Detail 页，不放首页

**交付物**：本地可运行 demo、一个完整 task flow、timeline/evidence 初版、checkpoint 初版

### 第 3 阶段：第 5-6 周 — 统一远程 Runtime

**目标**：把"本地任务"和"远程任务"统一成同一种产品对象。

1. **Runtime abstraction**：统一接口 `start_task / stop_session / attach_terminal / collect_artifacts`
2. **SSH / tmux integration**：复用现有 tmux 基础设施
3. **Runtime manager 页面**：展示 local/remote、online/offline、running tasks、health
4. **Remote reconnect**：网络中断、session still alive、reattach、artifact sync
5. **Runtime-scoped policies**：allowed paths、read-only vs read-write、dangerous commands require checkpoint

**交付物**：本地/远程统一任务流、runtime manager 初版、reconnect 能力、简单 policy 初版

### 第 4 阶段：第 7-8 周 — 控制台

**目标**：把产品真正做成控制台，而不是底层工具集合。

1. **Project Dashboard**：running tasks、blocked tasks、pending approvals、recent failures、recent completions、high-risk tasks、runtime health
2. **Workstream Board**：planned / running / waiting_human / blocked / execution_complete + task 卡片 + acceptance badge
3. **Task Detail**：Overview / Timeline / Evidence / Files & Diff / Sessions / Audit（6 tabs），Session Detail 提供 capability-gated `Attach Terminal`
4. **Approvals Inbox**：approve / reject / ask agent to revise / take over manually
5. **Recovery UI**：restart / resume / clone task / reopen terminal / reopen interactive attach（if supported）

**交付物**：可演示的控制台 UI、dashboard / board / task detail / approvals 初版

### 第 5 阶段：第 9-10 周 — 历史与回放

**目标**：做出核心差异点。

1. **Task history**：任务从创建到完成的完整轨迹
2. **Decision history**：AI 提议、人拒绝、路线变更、人工接管
3. **Evidence history**：每轮 diff、测试、摘要、命令、失败证据
4. **Runtime history**：在哪个 runtime 上，经历了哪些切换与重试
5. **Replay 页面**：step through events、inspect diff at each checkpoint、inspect approval decision

**交付物**：history 初版、replay 初版、decision log 初版、structured summaries 初版

### 第 6 阶段：第 11-12 周 — 打磨与发布

**目标**：发布一个方向清晰的开源版本。

1. **README 与 landing narrative**：Why / What / How / Quickstart / Demo GIF / Roadmap
2. **Demo 脚本**：创建 project → 3 个并行 tasks → 本地+远程 → checkpoint → 批准/拒绝 → evidence → replay
3. **Alpha 发布**
4. **验证闭环**：用户是否按 task 使用、是否用 timeline/evidence、是否 local/remote 切换、是否认为 replay 有价值

**交付物**：alpha 公开版、demo、onboarding 文档、issue 模板和反馈问卷

## 3. 后端目录结构

```text
cmd/
  agent-control-plane/
    main.go

internal/
  app/                          # 应用启动与配置
  config/                       # 配置加载
  httpapi/
    router.go
    middleware.go
    handlers/                   # HTTP handler 层
      projects.go
      workstreams.go
      tasks.go
      sessions.go
      claims.go
      reviews.go
      decisions.go
      checkpoints.go
      policies.go
      runtimes.go
      timeline.go
      replay.go

  domain/                       # 领域对象定义
    project.go
    workstream.go
    task.go
    session.go
    runtime.go
    policy.go
    claim.go
    review.go
    decision.go
    checkpoint.go
    artifact.go
    event.go

  repository/                   # 数据访问层
    interfaces.go
    postgres/                   # 或 sqlite/
      project_repo.go
      task_repo.go
      session_repo.go
      ...

  services/                     # 业务逻辑层
    project_service.go
    task_service.go
    session_service.go
    runtime_service.go
    policy_service.go
    claim_service.go
    review_service.go
    decision_service.go
    checkpoint_service.go
    artifact_service.go
    timeline_service.go
    replay_service.go
    orchestration_service.go
    evaluation_service.go

  policyengine/                 # 策略引擎
    evaluator.go
    rules/
      completion.go
      review.go
      approval.go
      autonomy.go
      escalation.go
      budget.go

  orchestration/                # 编排层
    planner.go
    task_spawner.go
    session_spawner.go
    reviewer_dispatcher.go
    merge_manager.go

  runtime/                      # Runtime 适配层
    manager.go
    adapters/
      local.go
      ssh.go
      hermes.go
      shell.go

  events/                       # 事件系统
    publisher.go
    recorder.go
    types.go

  state/                        # 状态机
    task_state_machine.go
    session_state_machine.go
    claim_state_machine.go
    checkpoint_state_machine.go

  queries/                      # 查询视图
    dashboard_query.go
    task_detail_query.go
    approvals_query.go
    replay_query.go

  auth/
  util/

migrations/
  0001_init.sql
  0002_indexes.sql
  0003_constraints.sql
```

## 4. 服务层职责划分

### 4.1 核心服务

| 服务 | 职责 | 不该做的事 |
|---|---|---|
| **ProjectService** | 创建项目、更新目标、绑定 policy、查询 dashboard | 不管 task/session 细节 |
| **TaskService** | 创建 task/subtask、状态迁移、完成判断、失败/阻塞/重试 | 不直接启动进程、不执行 policy 细则 |
| **SessionService** | 创建 session、状态迁移、attach/pause/resume/handover、心跳 | 不改业务状态 |
| **PolicyService** | 读取 policy、调用 evaluator、返回判定结果、版本切换 | 不直接决定业务状态，只输出判定 |
| **EvaluationService** | Decision Classifier：分类为 policy-resolvable / evidence-resolvable / judgment-required / goal-redefinition | 不执行裁决，只分类 |
| **OrchestrationService** | 自动拆 task、开 session、触发 reviewer、发起 checkpoint、走规则化决策 | 不直接操作数据库底表 |
| **RuntimeService** | 选择 runtime、检查状态、发给 adapter、attach/reconnect/stop | - |
| **ClaimService** | 主张提交、状态变更、证据绑定 | - |
| **ReviewService** | reviewer 提交复核结论 | - |
| **DecisionService** | 最终裁决，推动 task/session/checkpoint 状态变化 | - |
| **TimelineService** | 给 UI 返回"项目发生了什么" | 不自己拼逻辑 |
| **ReplayService** | 给 UI 返回"为什么走到这里" | 从 events + artifacts + decisions 推导 |

### 4.2 服务依赖规则

```text
HTTP Handlers
  → Services
    → Repository
    → PolicyEngine
    → Runtime Adapters
    → EventRecorder
```

- 只有 service 可以改业务状态
- 只有 EventRecorder 统一写 events
- 只有 PolicyService / PolicyEngine 解释规则
- 只有 RuntimeService 调底层 adapter

### 4.3 PolicyEngine 最小接口

```go
type PolicyEngine interface {
    EvaluateTaskSpawn(ctx, policy, task) TaskSpawnEval
    EvaluateSessionSpawn(ctx, policy, task) SessionSpawnEval
    EvaluateReviewNeed(ctx, policy, task, claim) ReviewEval
    EvaluateCompletion(ctx, policy, taskID) CompletionEval
    EvaluateQuestion(ctx, question) PolicyQuestionResult
}
```

返回值必须结构化，不只是 true/false：

```go
type CompletionEval struct {
    Ready                  bool
    RequireHumanAcceptance bool
    PolicyVersion          string
    Reason                 string
    MissingRequirements    []string
}
```

## 5. 核心状态流

### 5.1 主链：人创建任务 → agent 执行 → reviewer 复核 → policy 决策 → task 完成

```text
HTTP Handler
  → TaskService.CreateTask
  → EventRecorder.Record(task_created)

用户点击 Start / 系统自动启动
  → SessionService.CreateSession
  → RuntimeService.StartSession
  → EventRecorder.Record(session_started)

session 运行并产出结果
  → ArtifactService.Attach(...)
  → ClaimService.SubmitClaim(...)
  → EventRecorder.Record(claim_submitted)

ReviewService.TriggerOrSubmit(...)
  → EventRecorder.Record(review_submitted)

DecisionService.Decide(...)
  → TaskService.TransitionToExecutionComplete(...)
  → EventRecorder.Record(decision_made)
```

### 5.2 分类器：session 遇到问题时

```text
EvaluationService.ClassifyQuestion(question)
  → PolicyResolvable   → OrchestrationService.handlePolicyResolvable
  → EvidenceResolvable → OrchestrationService.handleEvidenceResolvable (开 reviewer)
  → JudgmentRequired   → OrchestrationService.raiseCheckpoint (给人类)
  → GoalRedefinition   → OrchestrationService.raiseCheckpoint (给人类)
```

## 6. 后端约束（第一版必须强制）

1. 所有可变核心对象必须包含 `row_version` 字段，所有状态迁移 API 必须携带 `expected_row_version`，不匹配时返回 409 Conflict（参见 data-model-v0.2.md Section 4.0B）
2. session 创建前先跑 policy 检查（active session 数、runtime 可用性、是否允许 parallel write）
2. task 进入 `execution_complete` 前先跑 completion check（tests、coverage、reviewer objection、evidence）
3. claim 没 evidence 不允许 submitted
4. reviewer 冲突必须触发 checkpoint 或 escalation
5. 所有状态迁移必须生成 event
6. policy 变更必须版本化，不能直接覆盖
7. `Task.acceptance_status` 生命周期（`not_ready → ready_for_acceptance → under_human_review → accepted/rejected`）必须独立于 `Task.state` 实现，`accepted` 和 `rejected` 必须对应显式 `Decision`（参见 data-model-v0.2.md Section 6.5）
8. `blocks` 类型的 Task 依赖是 level-triggered：上游任务状态回退时，下游任务必须级联回退（例如 `queued → planned`、`running → blocked`），pending 的 `final_acceptance` checkpoint 必须同步失效（参见 data-model-v0.2.md Section 4.3）
9. 所有规则触发的升级（conflicting reviews、budget exceeded 等）必须物化为 `Checkpoint`，以保持 Approvals Inbox 的唯一 canonical record source（参见 policy-and-decision-rules-v0.2.md Section 5.6）

## 7. 第一版异步 Worker

| Worker | 职责 |
|---|---|
| **Heartbeat Monitor** | 检查 session heartbeat、发现 crashed/reconnecting、写 event |
| **Policy Reevaluator** | task/session 状态变化后重跑 policy、自动触发 review/checkpoint/completion check |
| **Replay Materializer** | 从 events + artifacts + decisions 生成 replay snapshot |

## 8. 后端实现顺序

### 第一批

- Project、Task、Session、Runtime、Event
- 目标：能看见多条任务线和执行状态

### 第二批

- Artifact、Claim、Review、Decision、Checkpoint
- 目标：能看见证据、互审与裁决

### 第三批

- Policy evaluate endpoint、auto transition hooks、orchestration hooks
- 目标：能自动处理大部分规则化问题

## 9. 成功判断标准

### 产品是否成立

- 用户是否真的创建 task 而不是只开 terminal
- 用户是否真的使用 checkpoint
- 用户是否真的在 local 和 remote 间切换

### 方向是否成立

- 用户是否把它当作 agent 控制台，而不是 shell UI
- 是否有人反馈"终于不用一直盯着 CLI 了"

### 商业是否有希望

- 是否开始出现多 runtime、多 agent、共享、审计类需求
- 是否有人问团队部署、权限、审批、日志留存

## 10. 每周自检四问

1. 这个功能是在加强**任务控制**，还是只是加强**终端体验**？
2. 这个页面是在帮助用户**做决策**，还是只是帮助用户**读日志**？
3. 这条能力能不能沉淀成**历史 / 证据 / 回放**？
4. 这一步是否让产品更像 **agent runtime console**，而不是更像 **IDE 壳**？
