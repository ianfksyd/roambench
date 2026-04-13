# Data Model v0.2

## 1. 建模目标

这版模型的中心判断是：

> 模型要围绕 `project`、`task`、`state`、`evidence`、`decision` 来建，不能再围绕 terminal tab 或聊天线程来建。

它服务的不是数据库实现细节，而是系统的规范对象模型。后续 API、存储、事件流、页面结构、策略引擎和 replay 都应围绕它展开。

v0.2 相对 v0.1 的主要变化：
- `Task.state` 中 `completed` 重命名为 `execution_complete`，消除与 `acceptance_status` 的歧义
- 新增 Task 级依赖建模
- `Session.role` 拆分为 `execution_role` + `system_role`
- 明确 `Claim.confidence` 的语义边界
- 新增 Event 管理指导（分页、保留策略）
- 新增 `Task.policy_version_id` 字段

## 2. 建模原则

### 2.1 项目高于会话

顶层对象必须是 `Project`，不是 `Session`。

### 2.2 任务高于终端

`Task` 是最小业务单位；terminal 只是执行视图。

### 2.3 历史由事件驱动

不要事后补历史；每个关键动作都应落为 `Event`。

### 2.4 主张与证据分离

`Claim` 不能只是“AI 说了一段话”，必须绑定证据。

### 2.5 协作要结构化

核心协议是 `Claim -> Review -> Decision`。

### 2.6 规则优先于自由发挥

能由 policy 决定的，不交给 agent 自由判断。

### 2.7 长期连续性必须外部化

不要假设单次上下文能支撑大项目。

## 3. 核心对象总览

建议第一版定义 12 个核心对象：

- `Project`
- `Workstream`
- `Task`
- `Session`
- `Runtime`
- `Policy`
- `Claim`
- `Review`
- `Decision`
- `Artifact`
- `Checkpoint`
- `Event`

## 4. 对象定义

## 4.0 共享 Actor Identity 约束

为避免 audit、权限控制和按 actor 聚合查询出现多套命名字典，凡是表示“谁拥有 / 谁发起 / 谁复核 / 谁裁决”的字段，都应复用同一套 actor identity 枚举：

- `enum(human, agent, orchestrator, system, policy_engine)`
- 对应的 `*_ref` 字段应保存该 actor 在 user registry、agent registry、runtime registry 或 policy engine registry 中的稳定标识
- 下列字段都应直接复用这套枚举，而不是各自发明同义值：
  `Project.created_by_type`
  `Workstream.owner_type`
  `Policy.created_by_type`
  `Review.reviewer_type`
  `Decision.made_by_type`
  `Checkpoint.requested_by_type`
  `Event.actor_type`

## 4.0B 共享 Row Version 约束

为支撑乐观锁与原子状态迁移，所有可变核心对象都应包含：

- `row_version: integer`
- 每次成功写入时递增
- 所有会修改状态或关键字段的 API 都必须携带 `expected_row_version`
- append-only 对象（如 `Event`、`Decision`、`Artifact`、`Review`）不依赖该字段做并发控制

## 4.1 Project

作用：项目级容器，对应真实工程目标和共享代码空间。

关键字段：

- `id`
- `key`
- `name`
- `description`
- `repo_uri`
- `default_branch`
- `repo_root`
- `project_type`
- `status: enum(active, paused, archived)`
- `current_goal`
- `policy_id`
- `default_runtime_id`
- `created_by_type: enum(human, agent, orchestrator, system, policy_engine)`
- `created_by_ref`
- `row_version`
- `created_at`
- `updated_at`

设计要点：

- `policy_id` 挂项目，不挂 session
- `current_goal` 是项目目标，不等于具体 task goal
- `repo_root` 是共享文件层入口
- `created_by_type / created_by_ref` 应复用共享 actor identity 约束

## 4.2 Workstream

作用：项目内的一条主题工作线。

关键字段：

- `id`
- `project_id`
- `parent_workstream_id`
- `title`
- `description`
- `owner_type: enum(human, agent, orchestrator, system, policy_engine)`
- `owner_ref`
- `priority: enum(low, medium, high, critical)`
- `status: enum(planned, running, blocked, waiting_human, failed, completed, archived)`
- `scope_summary`
- `dependencies`
- `row_version`

设计要点：

- Workstream 表达“方向线”
- Task 表达“可执行单元”
- UI 上更适合先展示 workstream，再展开 tasks
- `owner_type / owner_ref` 应复用共享 actor identity 约束
- `running workstreams` 应直接来自该枚举，而不是额外发明查询层术语

## 4.3 Task

作用：系统最小业务执行单位，真正被派发给 agent / runtime。

关键字段：

- `id`
- `project_id`
- `workstream_id`
- `parent_task_id`
- `title`
- `goal`
- `scope`
- `success_criteria`
- `agent_strategy: enum(single_agent, worker_reviewer, multi_path, manual)`
- `preferred_agent_type`
- `preferred_runtime_id`
- `state: enum(planned, queued, running, waiting_review, waiting_human, blocked, failed, execution_complete, archived)`
- `risk_level: enum(low, medium, high, critical)`
- `priority: enum(low, medium, high, critical)`
- `spawn_reason`
- `acceptance_status: enum(not_ready, ready_for_acceptance, under_human_review, accepted, rejected)`
- `accepted_by_ref`
- `accepted_at`
- `acceptance_decision_id`
- `policy_version_id`
- `dependencies: [{target_task_id, dependency_type: enum(blocks, informs, shares_artifact)}]`
- `row_version`
- `created_at`
- `updated_at`

设计要点：

- `parent_task_id` 支持自动拆子任务
- `spawn_reason` 记录任务为什么被自动生成
- 最终验收不应只是布尔值；必须有显式 `Decision`
- `Task.state` 跟踪执行生命周期，`acceptance_status` 跟踪业务验收生命周期
- `Task.state=execution_complete` 且 `acceptance_status=under_human_review` 是合法状态
- `acceptance_decision_id` 让任务能回指最终验收裁决
- 未通过最终验收的任务不能直接归档，除非存在显式 archive override decision
- 如果验收被拒绝，任务执行状态必须能从 `execution_complete` 回到 `running` 或 `blocked`
- `policy_version_id` 绑定任务创建时的生效策略版本，任务生命周期内不自动迁移（参见 policy-and-decision-rules-v0.2.md Section 11）
- `dependencies` 中 `blocks` 类型的依赖会阻止任务进入 `queued`，直到目标任务达到 `execution_complete`
- `informs` 类型的依赖仅供参考，不阻塞状态迁移
- `shares_artifact` 表示两个任务共享产物引用，用于追踪数据流
- `blocks` 依赖是 level-triggered，而不是“一次满足后永久放行”
- 如果依赖失效导致任务离开 `ready_for_acceptance` 或 `under_human_review`，pending 的 `final_acceptance` checkpoint 必须同步失效

## 4.4 Session

作用：Task 在某个 runtime 上的一次执行实例。

关键字段：

- `id`
- `task_id`
- `project_id`
- `runtime_id`
- `agent_type`
- `agent_version`
- `execution_role: enum(implement, test, verify, review, alternative_path, rollback_candidate)`
- `system_role: enum(worker, orchestrator)`
- `state: enum(queued, starting, active, paused, waiting_human, waiting_review, reconnecting, crashed, completed, terminated)`
- `workspace_ref`
- `branch_name`
- `worktree_path`
- `row_version`
- `started_at`
- `updated_at`
- `completed_at`
- `parent_session_id`

设计要点：

- `Task` 是业务对象，`Session` 是执行对象
- `execution_role` + `system_role` 拆分避免将业务角色（implement、review）与系统角色（orchestrator）混在同一枚举中
- `execution_role` 可防止重复开出等价 session
- `workspace_ref` 支持共享代码空间与隔离 worktree 并存
- `parent_session_id` 支持失败接管或重试接续
- 占用执行槽位的 session 状态至少包括 `starting`、`active`、`paused`、`waiting_review`、`waiting_human`、`reconnecting`

## 4.5 Runtime

作用：统一抽象本地机、远程机、容器或托管执行环境。

关键字段：

- `id`
- `project_id`
- `name`
- `kind: enum(local, ssh_remote, container, cloud_devbox, managed_agent_backend)`
- `host_label`
- `status: enum(online, offline, degraded, busy)`
- `capabilities`
- `writable_paths`
- `protected_paths`
- `max_concurrent_sessions`
- `health_summary`
- `metadata_json`
- `row_version`

设计要点：

- `kind` 应预留托管 agent 后端
- `capabilities` 记录 build、test、network、browser 等能力
- `max_concurrent_sessions` 是 runtime-local 硬上限；与 policy scope 的 `autonomy_limits.max_concurrent_sessions` 共同决定 session 是否可启动

## 4.6 Policy

作用：项目执行宪法，决定 completion、review、approval、autonomy、budget。

关键字段：

- `id`
- `project_id`
- `version`
- `name`
- `yaml_body`
- `notes_md`
- `active`
- `created_by_type: enum(human, agent, orchestrator, system, policy_engine)`
- `created_by_ref`
- `row_version`
- `created_at`
- `superseded_by_policy_id`

设计要点：

- 策略必须可版本切换
- 不能就地覆盖
- `created_by_type / created_by_ref` 应复用共享 actor identity 约束
- `row_version` 用于乐观锁；policy 语义版本仍由 `version` 字段表达

## 4.7 Claim

作用：某个 session 提出的结构化主张。

关键字段：

- `id`
- `project_id`
- `task_id`
- `session_id`
- `claim_type: enum(bug_fixed, test_passed, refactor_complete, migration_safe, docs_updated, ready_for_acceptance, rollback_needed, other)`
- `title`
- `statement`
- `confidence`
- `status: enum(drafted, submitted, under_review, validated, rejected, superseded)`
- `evidence_refs`
- `row_version`
- `created_at`
- `updated_at`
- `superseded_by_claim_id`

设计要点：

- 证据应绑定到 `Artifact`
- 主张状态必须允许被后续主张覆盖
- `confidence` 是 agent 自报值（0.0–1.0），仅用作 review 触发信号（配合 `review_on_low_confidence_below` 策略规则），不作为质量指标
- 系统不会因 confidence 高而自动接受 claim；confidence 仅决定是否触发 review 流程
- 如果 agent 不报告 confidence，系统应默认视为需要 review

## 4.8 Review

作用：另一个 agent、人或系统对 `Claim` 的复核。

关键字段：

- `id`
- `project_id`
- `task_id`
- `claim_id`
- `session_id`
- `reviewer_type: enum(human, agent, orchestrator, system, policy_engine)`
- `reviewer_ref`
- `verdict: enum(support, reject, uncertain, needs_more_evidence)`
- `reasoning`
- `confidence`
- `additional_evidence_refs`
- `created_at`

设计要点：

- `uncertain` 与 `needs_more_evidence` 不应合并
- reviewer 可以是 agent、human 或 system
- `reviewer_type / reviewer_ref` 应复用共享 actor identity 约束
- 当 `reviewer_type=agent` 时，`session_id` 不能缺失；human/system review 可为空

## 4.9 Decision

作用：对 claim、review 或 task 的最终裁决，是状态跃迁的开关。

关键字段：

- `id`
- `project_id`
- `task_id`
- `claim_id`
- `review_id`
- `checkpoint_id`
- `made_by_type: enum(human, agent, orchestrator, system, policy_engine)`
- `made_by_ref`
- `decision_type`
- `summary`
- `rationale`
- `evidence_snapshot_refs`
- `created_at`

设计要点：

- `decision_type` 应直接映射状态机
- `spawn_subtask`、`spawn_session` 也应明确记为 decision
- 最终验收、checkpoint 审批、claim 裁决都必须显式落为 `Decision`
- `made_by_type / made_by_ref` 应复用共享 actor identity 约束
- `decision_type` 至少应覆盖 `claim_validated`、`claim_rejected`、`checkpoint_approved`、`checkpoint_rejected`、`final_acceptance_approved`、`final_acceptance_rejected`、`archive_override_approved`、`archive_override_rejected`
- `evidence_snapshot_refs` 用于冻结裁决时实际参考的证据集合
- `task_id` 是 task-scope decision 的上下文字段，不应被视为与 `claim_id`、`review_id`、`checkpoint_id` 互斥
- 每条 `Decision` 必须有且仅有一个“主语类型”：
  `claim_id`、`review_id`、`checkpoint_id` 三者恰好一个，或三者全空表示纯 task-level decision
- 因此，合法组合通常是：
  `task_id + claim_id`
  `task_id + review_id`
  `task_id + checkpoint_id`
  `task_id only`
- `task_id only` decision 仅用于 task-level 裁决，例如 `spawn_subtask`、`spawn_session` 或 archive override
- 最终验收 decision 合法地同时带 `task_id + checkpoint_id`：
  `task_id` 表示被验收的任务上下文，`checkpoint_id` 表示进入审批队列的那次具体验收事件

## 4.10 Artifact

作用：结构化证据容器。

关键字段：

- `id`
- `project_id`
- `task_id`
- `session_id`
- `artifact_type`
- `uri`
- `inline_text`
- `metadata_json`
- `created_at`

设计要点：

- `uri` 与 `inline_text` 可并存
- 关键结论不应只存在于 stdout

## 4.11 Event

作用：系统历史的骨架，timeline、replay、audit 都从这里生成。

关键字段：

- `id`
- `project_id`
- `workstream_id`
- `task_id`
- `session_id`
- `actor_type: enum(human, agent, orchestrator, system, policy_engine)`
- `actor_ref`
- `event_type: enum(project_created, project_state_changed, workstream_created, workstream_state_changed, task_created, task_state_changed, session_started, session_state_changed, runtime_registered, runtime_state_changed, runtime_assigned, claim_submitted, claim_state_changed, review_submitted, decision_made, checkpoint_raised, checkpoint_resolved, artifact_created, policy_attached, policy_version_superseded, acceptance_state_changed)`
- `summary`
- `payload_json`
- `created_at`

设计要点：

- `summary` 用于 timeline 一级展示
- `payload_json` 支持 replay 和调试
- `actor_type / actor_ref` 应复用共享 actor identity 约束
- `Project.status` 和 `Runtime.status` 的变化至少应分别映射到 `project_state_changed` 与 `runtime_state_changed`
- 上面这组 `event_type` 是 replay / audit 的最小公共词汇表；后续可以扩展，但不应随实现自由发明同义事件名

### Event 管理指导

- **分页**：Event 查询必须支持基于 cursor 的分页（cursor = `event.id` 或 `event.created_at`），不使用 offset-based 分页
- **保留策略**：建议对超过 N 天的事件进行摘要聚合（digest event），原始事件可归档但不立即删除
- **Replay Query 过滤**：Replay Query 应支持按时间范围（`since` / `until`）和 `event_type` 过滤，避免强制加载全量事件流
- **写入保证**：Event 是 append-only，不可修改或删除（除归档外）

## 4.12 Checkpoint

建议第一版单独建模，而不是完全混入 `Event`。

关键字段：

- `id`
- `project_id`
- `task_id`
- `session_id`
- `trigger_type: enum(destructive_command, budget_exceeded, conflicting_reviews, protected_path_change, final_acceptance, goal_redefinition, manual_pause)`
- `status: enum(pending, approved, rejected, expired)`
- `requested_by_type: enum(human, agent, orchestrator, system, policy_engine)`
- `requested_by_ref`
- `prompt_summary`
- `resolved_by_decision_id`
- `row_version`
- `created_at`
- `resolved_at`

适用触发场景：

- `destructive_command`
- `budget_exceeded`
- `conflicting_reviews`
- `protected_path_change`
- `final_acceptance`
- `goal_redefinition`
- `manual_pause`

设计约束：

- `trigger_type=final_acceptance` 不是另一套平行机制；它就是最终人类验收进入审批队列时使用的 checkpoint 形式
- `trigger_type` 应使用上面的规范枚举值，不应在实现层自由改写成空格分词或其他同义命名
- `requested_by_type / requested_by_ref` 应复用共享 actor identity 约束
- 当 `Task.acceptance_status=under_human_review` 时，应存在且仅存在一个处于 `pending` 的 `final_acceptance` checkpoint
- `final_acceptance_approved` 或 `final_acceptance_rejected` decision 必须解析对应的 `final_acceptance` checkpoint
- `approved` 和 `rejected` 是由显式 decision 解析后的终态；`expired` 是未获裁决自然失效的终态
- 如果任务因为依赖失效、策略失效或其他系统性回退而离开 `under_human_review`，对应的 pending `final_acceptance` checkpoint 必须转为 `expired`

## 5. 对象关系

核心关系建议如下：

```text
Project
 ├─ Workstreams
 ├─ Tasks
 │   ├─ child Tasks
 │   ├─ Sessions
 │   │   ├─ Claims
  │   │   ├─ Artifacts
  │   │   └─ Events
  │   ├─ Reviews
 │   ├─ Checkpoints
  │   ├─ Decisions
  │   └─ Events
 ├─ Runtimes
 ├─ Policy versions
 └─ Project-level Events
```

最关键的三条链：

- 执行链：`Task -> Session -> Artifact`
- 协作链：`Claim -> Review -> Decision`
- 历史链：`Any action -> Event`

## 6. 最小状态机

### 6.1 Task

```text
planned -> queued -> running
queued -> planned
running -> waiting_review
running -> waiting_human
running -> blocked
running -> failed
waiting_review -> blocked
waiting_human -> blocked
waiting_review -> running
waiting_human -> running
blocked -> running
failed -> running
running -> execution_complete
execution_complete -> running
execution_complete -> blocked
execution_complete -> archived
```

补充约束：

- `execution_complete` 只表示执行生命周期结束，不等于业务已验收
- `execution_complete -> running` 或 `execution_complete -> blocked` 主要发生在 `final_acceptance_rejected` 之后
- `execution_complete -> archived` 只有在 `acceptance_status=accepted` 或存在显式 archive override decision 时才允许
- 任务不能进入 `queued` 状态，如果存在 `blocks` 类型的依赖且目标任务尚未达到 `execution_complete`
- `blocks` 依赖的满足条件是 level-triggered，而不是一次性闸门
- 如果 blocking upstream 在 downstream 被最终验收前重新离开 `execution_complete`：
  `queued -> planned`
  `running / waiting_review / waiting_human -> blocked`
  `execution_complete` 且 `acceptance_status!=accepted` -> `blocked`，并将 `acceptance_status` 重置为 `not_ready`
- 只有已经 `accepted` 的 downstream task 才不会被系统自动回退；此时应通过显式 decision、新任务或人工裁决处理后续影响

### 6.2 Workstream

```text
planned -> running
running -> blocked
running -> waiting_human
running -> failed
running -> completed
blocked -> running
waiting_human -> running
failed -> running
completed -> archived
```

补充约束：

- `running workstreams` 应仅来自 `status=running`
- workstream 状态反映整条工作线的推进情况，不替代 task 级状态

### 6.3 Session

```text
queued -> starting
starting -> active
active -> waiting_review
active -> waiting_human
active -> paused
active -> reconnecting
active -> crashed
active -> completed
queued -> terminated
paused -> active
reconnecting -> active
crashed -> terminated
waiting_human -> active
waiting_review -> active
```

补充约束：

- `Session.state=completed` 是 Session 的终态，与 `Task.state=execution_complete` 是两套独立状态机，不应混淆
- Session 使用 `completed` 而 Task 使用 `execution_complete`，因为 Task 的"完成"需要与 `acceptance_status` 区分，而 Session 没有验收概念

补充约束：

- `queued` 表示 session 已创建但尚未获得 runtime 执行槽位
- `queued -> starting` 仅在 runtime 并发预算允许时发生
- `queued -> terminated` 表示排队中的 session 被取消，不进入实际执行
- `crashed`、`completed`、`terminated` 不应继续计入 runtime 或 policy 的占槽并发

### 6.4 Claim

```text
drafted -> submitted
drafted -> superseded
submitted -> under_review
submitted -> superseded
under_review -> validated
under_review -> rejected
under_review -> superseded
```

补充约束：

- `superseded` 可从 `drafted`、`submitted`、`under_review` 三种状态进入，因为后续 claim 可以在任何阶段替代先前 claim
- `superseded` 必须填写 `superseded_by_claim_id`

### 6.5 Acceptance

```text
not_ready -> ready_for_acceptance
ready_for_acceptance -> not_ready
ready_for_acceptance -> under_human_review
under_human_review -> not_ready
under_human_review -> accepted
under_human_review -> rejected
rejected -> not_ready
```

补充约束：

- `accepted` 和 `rejected` 必须对应显式 final acceptance decision
- `ready_for_acceptance -> not_ready` 或 `under_human_review -> not_ready` 只允许在系统性失效场景下发生，例如依赖失效、策略失效或证据被撤销
- `rejected -> not_ready` 表示当前验收尝试已经结束，后续必须经过返工或补证据才能再次申请验收
- `rejected` 后，任务执行状态通常需要重新回到 `running` 或 `blocked`
- 如果 `under_human_review -> not_ready` 发生，pending 的 `final_acceptance` checkpoint 必须同步转为 `expired`

### 6.6 Checkpoint

```text
pending -> approved
pending -> rejected
pending -> expired
```

补充约束：

- `approved` 和 `rejected` 必须带有 `resolved_by_decision_id`
- `expired` 不应伪装成人工裁决结果
- 同一 task/session 下，不应同时存在多个语义重复的 `pending` checkpoint
- 当任务因系统性失效离开 `under_human_review` 时，对应的 pending `final_acceptance` checkpoint 应转为 `expired`，而不是继续保留在 approvals inbox

## 7. 哪些对象是 P0，哪些可后补

### P0 模型层必须存在

- `Project`
- `Workstream`
- `Task`
- `Session`
- `Runtime`
- `Policy`
- `Claim`
- `Review`
- `Decision`
- `Artifact`
- `Checkpoint`
- `Event`

### P1 可后补

- `User / Agent Registry`
- `Policy Override Record`

## 8. 最小查询视图

### 8.1 Project Dashboard Query

应返回：

- running workstreams
- running tasks
- blocked tasks
- waiting_human tasks
- pending checkpoints
- recent decisions
- runtime health
- recent failures

说明：

- 避免在 API 或查询层使用未定义的裸术语 `active`
- 列表名称应直接映射到底层状态枚举，例如 `running tasks`

### 8.2 Task Detail Query

应返回：

- task
- current sessions
- latest claims
- latest reviews
- latest decision
- latest acceptance decision
- pending checkpoints
- relevant artifacts
- timeline tail

说明：

- `current sessions` 应表示该 task 下所有 non-terminal sessions，而不只是 `state=active`
- 至少应覆盖 `queued`、`starting`、`active`、`paused`、`waiting_review`、`waiting_human`、`reconnecting`、`crashed`
- 如果页面还需要“此刻真正正在执行”的更窄视图，可以再派生 `active sessions` 过滤结果，但不应替代任务详情里的主 session 列表

### 8.3 Approvals Inbox Query

应返回：

- pending checkpoints
- waiting_human tasks that are not already represented by a pending checkpoint
- conflicting reviews
- budget exceeded tasks

说明：

- `final acceptance candidates` 应视为 `pending checkpoints` 中 `trigger_type=final_acceptance` 的过滤视图，而不是额外重复的一组 payload
- 如果某个任务已经有 pending checkpoint，就不应再以重复待办的形式单独出现
- 如果同时返回原始 checkpoints 和 UI convenience groups，convenience groups 必须只返回 checkpoint refs / IDs，而不是重复拷贝整条待办记录
- `conflicting reviews` 和 `budget exceeded tasks` 也应遵守同样规则：
  如果它们已经物化成 pending checkpoint，就只能作为 `pending checkpoints` 的过滤/聚合视图存在，不能再作为第二份独立待办记录返回
- 规范上应把 `pending checkpoints` 视为审批收件箱的唯一 canonical record source，其他分组都只是面向 UI 的派生视图

### 8.4 Replay Query

应按 event 顺序返回：

- state changes
- claims
- reviews
- checkpoints
- decisions
- artifact snapshot refs

## 9. 实施顺序

建议按下面三步推进。这里的阶段是“能力上线顺序”，不是否定前面对象在模型层的必要性。

### 第一步

先实现基础控制层：

- `Project`
- `Workstream`
- `Task`
- `Session`
- `Runtime`
- `Event`
- `Policy` 的最小 schema 与项目绑定
- `Checkpoint` 的最小 record schema 与 pending/resolved 生命周期

目标是先让系统能“看见多条线、多 runtime、多执行实例”，同时不丢失策略引用和审批记录入口。

### 第二步

再实现证据与裁决层：

- `Artifact`
- `Claim`
- `Review`
- `Decision`

目标是让系统从“看见执行”升级到“看见证据与裁决”。

### 第三步

最后实现自动化能力层：

- policy engine
- Decision Classifier
- 自动 checkpoint 路由
- 审批与验收 UI

目标是让系统接管更多规则化问题。

## 10. 最终收束

这版模型的重点不是“如何保存 AI 输出”，而是：

> 如何把复杂项目中的多工作线、多 agent、多 runtime、长时执行、证据、复核和裁决，组织成一个可追踪、可回放、可控制的系统。
