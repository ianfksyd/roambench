# OpenClaw Comparison v0.1

## 1. 结论

OpenClaw 值得借鉴，但 RoamBench 不应该做成 OpenClaw 的同类产品。

OpenClaw 的强项是：

> 让一个 always-on agent 从聊天入口、定时任务、skills、hooks、subagents 中自动做事。

RoamBench 的强项应该是：

> 工程化推进一个复杂软件项目，让多工作线、多 agent、多 runtime 在项目语义下可分派、可验证、可审查、可恢复、可验收。

因此，OpenClaw 可以作为 agent automation runtime 的参考，但不能成为 RoamBench 的产品主语。

核心判断：

- 借鉴 OpenClaw 的自动化原语。
- 不借鉴“多 agent 聊天平台”作为核心控制模型。
- Skills 可以承载自动开发的任务知识，但不能单独替代自动开发系统。
- RoamBench 的核心不是聊天协作，而是工程项目执行控制。

## 2. OpenClaw 的核心定位

基于 OpenClaw 公开站点、仓库与文档（核对时间：2026-04-15 UTC），它更像一个个人自托管的 agent automation platform。

它的主要能力包括：

- 多消息入口：Telegram、WhatsApp、Discord、Slack 等
- always-on agent
- skills / ClawHub
- standing orders
- cron / scheduled tasks
- background tasks
- task flow
- sub-agents
- hooks / webhooks
- 本地或自托管运行
- 广泛工具接入

这些能力解决的问题是：

> 如何让 agent 随时从自然语言入口收到指令，并在用户授权边界内自动行动。

RoamBench 要解决的问题不同：

> 如何把复杂软件项目中的计划、实现、测试、审查、修复、验收、回放工程化，并让不同 agent/runtime 在统一控制面下协作。

## 3. 核心差异

| 维度 | OpenClaw | RoamBench |
|---|---|---|
| 顶层对象 | agent / channel / session / standing order | Project / Workstream / Task |
| 产品主语 | 个人自动化 agent | 复杂项目执行控制面 |
| 主要入口 | 聊天渠道、cron、hooks | 项目态势、任务图、审批、证据、回放 |
| 多 agent 模型 | subagents、thread binding、announce | manager / worker / reviewer + Claim / Review / Decision |
| Skills | agent 能力扩展与 slash command | agent-neutral 的 Skill / Runbook / Policy 组合 |
| 自动化 | standing orders + cron + hooks | ScheduleRule + Policy + RunbookState + Checkpoint |
| 历史 | session transcript / logs / task records | Event / Artifact / Claim / Review / Decision / Checkpoint |
| 权限 | agent/workspace/tool/sandbox 配置 | PolicyEngine + PermissionGrant + Tool Gateway |
| UI 重点 | 从聊天触发并接收结果 | 看清项目状态、证据、风险、阻塞、下一步 |
| 成功标准 | agent 能持续替用户做事 | 项目能被工程化推进而不迷航 |

最关键差异：

- OpenClaw 的主语是 **Agent Automation**
- RoamBench 的主语是 **Engineering Project Execution**

## 4. 应该借鉴的部分

### 4.1 Skill Discovery / Precedence / Allowlists

OpenClaw 的 skills 体系值得认真借鉴。

可借鉴点：

- `SKILL.md` 作为能力包入口
- workspace / project / personal / bundled / managed 等多来源加载
- skill 优先级与覆盖规则
- per-agent skill allowlist
- 第三方 skill registry
- 安装、更新、同步命令
- 对第三方 skill 的安全提醒

RoamBench 的改造方向：

```text
OpenClaw Skill
-> RoamBench Skill
-> Runbook
-> Policy-bound Phase
-> Evidence-producing Session
```

也就是说，RoamBench 可以借鉴 skill 的文件格式、发现机制和生态思路，但 Skill 不能直接拥有执行权。执行权必须由 Policy、Runbook phase 和 PermissionGrant 决定。

### 4.2 Standing Orders

OpenClaw 的 standing orders 很接近 RoamBench 需要的长期授权模型。

可借鉴结构：

- scope
- trigger
- approval gate
- escalation rule
- execution steps
- what not to do

RoamBench 应把它升级成项目级 `Program` 或 `Project Rule`：

```text
Standing Order
-> Project Program
-> ScheduleRule / EventTrigger
-> Runbook
-> Policy
-> Checkpoint
```

尤其值得借鉴的是 `Execute -> Verify -> Report` 纪律。RoamBench 中应该升级为：

```text
Execute
-> Verify
-> Record Evidence
-> Review
-> Decide
```

不能只让 agent 汇报“完成了”，必须要求 artifact、test result、review result、decision log。

### 4.3 Cron / Heartbeat / Scheduled Tasks

OpenClaw 证明 always-on agent 需要定时触发和周期性检查。

RoamBench 应借鉴触发机制，但不照搬 cron 作为核心语义。

推荐映射：

| OpenClaw | RoamBench |
|---|---|
| cron job | `ScheduleRule` |
| heartbeat | `ProjectHealthCheck` / `NoProgressRule` |
| scheduled task | `TaskTrigger` |
| announce | `Event` + optional `Checkpoint` |
| retry | `RetryPolicy` |

RoamBench 的定时规则应绑定项目状态，例如：

- session 超过 90 分钟未产出 artifact
- task 连续 3 次 test failure
- review pending 超过 4 小时
- workstream 无进展超过 1 天
- 每日项目 digest

### 4.4 Background Tasks

OpenClaw 的 background task 账本值得借鉴。

RoamBench 中对应的是：

```text
PhaseAttempt
```

每次 plan、implement、test、review、fix、validation 都应该生成 attempt 记录，而不是只存在 session transcript 里。

### 4.5 Task Flow

OpenClaw 的 Task Flow 说明多步任务需要明确阶段。

RoamBench 应把它做成更强的 Runbook 状态机：

```text
Task Flow
-> Runbook
-> RunbookPhase
-> PhaseAttempt
-> CompletionRules
-> ReviewRules
-> CheckpointRules
```

重点不只是“按步骤做”，而是每一步都能被审计、重试、阻断、恢复。

### 4.6 Subagents

OpenClaw 的 sub-agent 设计有几个可借鉴点：

- sub-agent 是后台 run
- 独立 session
- 完成后 announce
- 支持 timeout
- 支持并发上限
- 支持嵌套深度限制
- 支持 sandbox 要求
- 支持 tool policy by depth
- 支持 cascade stop

RoamBench 应映射为：

```text
Session spawn policy
-> max_spawn_depth
-> max_concurrent_sessions
-> run_timeout
-> workspace isolation
-> tool capability restrictions
-> cascade stop
```

但结果不能只 announce 到聊天，而必须沉淀为 `Event / Artifact / Claim`。

### 4.7 Hooks / Webhooks

OpenClaw 的 hooks 和 webhooks 适合借鉴为外部触发机制。

RoamBench 可支持：

- GitHub issue created
- PR updated
- CI failed
- deployment failed
- dependency alert
- scheduled release check

但触发后不应直接让 agent 自由行动，而应创建或推进 `Task`，并绑定对应 `Policy`。

## 5. 不应该照搬的部分

### 5.1 不把聊天作为 source of truth

聊天适合交互，不适合作为项目控制核心。

原因：

- 状态不稳定
- 责任边界不清
- 证据和判断混在一起
- 长期回放困难
- 多 agent 噪音高
- 很难表达任务依赖、审批和阻塞

RoamBench 可以保留 chat/thread 作为干预界面，但 canonical history 必须是：

- `Event`
- `Artifact`
- `Claim`
- `Review`
- `Decision`
- `Checkpoint`

### 5.2 不把多 agent 群聊作为协作模型

OpenClaw 的多 agent 聊天与 thread binding 对个人助理场景有用，但不适合作为复杂项目的主模型。

RoamBench 应采用：

```text
Manager
-> Worker
-> Reviewer
-> PolicyEngine
-> Human Checkpoint
```

协作协议是：

```text
Claim
-> Evidence
-> Review
-> Decision
```

而不是多个 agent 在群里互相说服。

### 5.3 不把 persona / SOUL / chat memory 当项目模型

OpenClaw 的 agent workspace 文件、persona、SOUL、MEMORY 对个人助理有价值。

RoamBench 可以保留 agent profile，但项目事实必须来自项目对象：

- Project
- Workstream
- Task
- Runbook
- Artifact
- Review
- Decision

### 5.4 不授予宽泛 standing authority

OpenClaw 的 standing orders 鼓励给 agent 长期授权。这个方向可以借鉴，但 RoamBench 必须收紧。

所有长期授权都必须落到：

- scope
- allowed paths
- allowed tools
- budget
- approval gates
- escalation rules
- max retries
- test/review requirements
- protected paths

“你拥有这个项目，自己看着办”不是可接受的规则。

### 5.5 不把自动开发做成单个 Skill

自动开发不是一个 skill 能解决的问题。

如果把 `auto_develop` 做成一个大 prompt，会重新回到不可控 agent。

RoamBench 应该把自动开发拆成：

```text
Skill = 任务类型能力
Runbook = 阶段流程
Policy = 权限与边界
Scheduler = 触发与重试
WorkspaceRef = 执行隔离
Artifact = 证据
Review = 验证
Decision = 状态推进
Checkpoint = 人类裁决
```

## 6. 自动开发应该如何落到 RoamBench

OpenClaw 的“自动开发”可以启发 RoamBench，但不能照搬成“agent 自己找事、自己改、自己报喜”。

更合理的 RoamBench 自动开发 runbook：

```text
Intake
-> Triage
-> Plan
-> Scope Approval if needed
-> Implement
-> Test
-> Review
-> Fix / Replan
-> Final Validation
-> Ready For Human Acceptance
```

### 6.1 `issue_to_pr` Skill 示例

```yaml
skill:
  id: issue_to_pr
  version: "0.1"
  description: Convert a scoped issue into a reviewed code change.
  default_runbook: issue_to_pr_default
  required_artifacts:
    - issue_triage
    - plan
    - diff_summary
    - test_result
    - review_result
    - final_claim
```

### 6.2 Runbook 示例

```yaml
runbook:
  id: issue_to_pr_default
  phases:
    - id: intake
      execution_role: plan
      write_access: false
      required_artifacts: [issue_triage]

    - id: plan
      execution_role: plan
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
          - review_objection
          - evidence_incomplete

    - id: final_validation
      execution_role: verify
      write_access: false
      required_artifacts: [final_claim]

  completion_rules:
    require_tests_pass: true
    require_review_result: true
    require_no_high_severity_objection: true
    require_diff_summary: true

  notify_human:
    - scope_expansion
    - protected_path_change
    - repeated_failure
    - final_acceptance
```

### 6.3 为什么 Skill 不能单独替代自动开发

Skill 描述的是“怎么做某类事”，但自动开发还需要回答：

- 能不能做
- 在哪里做
- 能改哪些文件
- 预算是多少
- 失败重试几次
- 谁 review
- 哪些证据才算足够
- 什么时候通知人
- 哪个状态可以被推进

这些不是 Skill 的职责，而是 Policy、Runbook、Workspace、Evidence、Review、Checkpoint 的职责。

## 7. RoamBench 应形成的工程化推进模型

我们要做的不是“让很多 agent 聊得更热闹”，而是把复杂项目推进工程化。

工程化推进意味着：

1. **任务可定义。** 每个目标都落到 Task，而不是散落在聊天里。
2. **范围可约束。** 每个 Task 有 allowed paths、protected paths、budget、risk。
3. **过程可执行。** 每个 Task 绑定 Skill / Runbook。
4. **动作可审计。** 每次工具调用、文件修改、测试运行都生成 Event 或 Artifact。
5. **结论可验证。** Worker 只能提出 Claim，不能自己裁定完成。
6. **审查可阻断。** Reviewer objection 可以阻止进入验收。
7. **失败可恢复。** PhaseAttempt、Checkpoint、Artifact 支持重试和回放。
8. **人类少被打断。** 只有 checkpoint、scope expansion、risk、final acceptance 才通知人。
9. **agent 可替换。** Hermes、OpenClaw、Codex、OpenHands、CLI agent 都只是 runtime/adapter。
10. **项目不迷航。** UI 先显示状态、阻塞、证据、下一步，而不是长日志。

## 8. 推荐设计规则

建议写入系统规则：

1. Chat/thread 只能作为交互与通知界面，不能作为 canonical state。
2. 自动开发必须通过 Skill + Runbook + Policy + Review + Checkpoint 组合实现。
3. Skills 不授予权限，只描述能力与流程建议。
4. Standing-order-like 规则必须绑定 scope、approval gate、escalation rule。
5. 定时触发必须创建或推进 Task / PhaseAttempt，不能绕过 Policy。
6. Subagent/session spawn 必须有 max depth、timeout、concurrency、tool policy。
7. Agent announce 必须转换成 Event / Artifact / Claim。
8. Reviewer 默认只读或隔离 workspace。
9. 任何 final answer 都不能直接完成 Task，必须通过 completion rules。
10. 复杂项目的主界面必须围绕 Project / Workstream / Task，而不是 agent 群聊。

## 9. 产品判断

OpenClaw 的价值在于把 agent 变成个人自动化入口。RoamBench 的价值在于把复杂软件项目变成可控的执行系统。

因此，RoamBench 应该借鉴：

- skills 生态
- standing orders
- cron / heartbeat
- background tasks
- task flow
- subagent controls
- hooks / webhooks
- sandbox / tool policy 思路

但必须坚持：

> Chat is an interface, not the control plane.
> Skills are capability packages, not autonomy grants.
> Automatic development is a project execution system, not a single agent prompt.

最终边界：

> OpenClaw helps an agent do things.
> RoamBench helps a complex project move forward with evidence, review, and control.

## 10. 参考资料

后续重新评估 OpenClaw 时，应优先核对官方来源：

- Home: <https://open-claw.org/>
- GitHub: <https://github.com/openclaw/openclaw>
- Skills: <https://docs.openclaw.ai/tools/skills>
- Creating Skills: <https://docs.openclaw.ai/tools/creating-skills>
- ClawHub: <https://docs.openclaw.ai/tools/clawhub>
- Automation & Tasks: <https://docs.openclaw.ai/automation>
- Scheduled Tasks: <https://docs.openclaw.ai/automation/scheduled-tasks>
- Background Tasks: <https://docs.openclaw.ai/automation/background-tasks>
- Task Flow: <https://docs.openclaw.ai/automation/taskflow>
- Standing Orders: <https://docs.openclaw.ai/automation/standing-orders>
- Sub-Agents: <https://docs.openclaw.ai/tools/subagents>
- Multi-Agent Routing: <https://docs.openclaw.ai/concepts/multi-agent>
