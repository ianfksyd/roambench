# Hermes Agent Comparison v0.1

## 1. 结论

Hermes Agent 值得研究，也值得接入，但 RoamBench 不应该做成 Hermes 的同类替代品。

更合理的边界是：

> Hermes 是可插拔的 agent runtime / worker body。
> RoamBench 是 agent-neutral 的项目控制面 / execution control plane。

也就是说，Hermes 可以成为 RoamBench 下的一类 worker 或 reviewer；但 RoamBench 的核心状态、权限、证据、审查、验收、回放不应依附于 Hermes。

我们要借鉴 Hermes 的 runtime 能力，但创新点应该放在项目级控制、结构化协作、证据化审查和可恢复执行上。

## 2. Hermes 的核心定位

基于 Hermes Agent 公开仓库与文档（核对时间：2026-04-15 UTC），它更像一个自托管、长期运行、可扩展的 agent runtime。

它的核心能力大致包括：

- agent 会话与任务执行
- skills / procedural memory
- long-term memory
- MCP 工具接入
- cron / scheduled task
- API server 与前端
- agent 自我改进方向

这些能力对单个 agent 的持续执行很有价值，但它们的重心仍然是：

> 让一个 agent 更会工作、更会调用工具、更会记住经验。

RoamBench 的重心不同：

> 让多个 agent、多个 runtime、多个工作线在一个项目控制面里可分派、可监督、可审查、可恢复、可验收。

## 3. 核心差异

| 维度 | Hermes Agent | RoamBench |
|---|---|---|
| 顶层对象 | agent / session / memory / skill | Project / Workstream / Task |
| 产品重心 | agent runtime 与自我改进 | 项目级执行控制面 |
| agent 关系 | 以自身 agent 能力为中心 | agent-neutral，可接入 Hermes、Codex、OpenHands、CLI agent 等 |
| Skills | agent 的过程能力或记忆 | 系统拥有的 versioned Skill / Runbook |
| Memory | agent 长期记忆 | Event / Artifact / Claim / Review / Decision / Checkpoint |
| MCP | agent 调工具的扩展能力 | Tool Gateway 背后的工具标准之一 |
| Cron | 定时触发 agent 行为 | 绑定 Task / Session / Phase 状态的 Schedule Rules |
| 权限 | runtime / agent 侧能力管理 | Policy + PermissionGrant + Tool Gateway 强制执行 |
| UI | agent 使用与运行界面 | 态势、工作线、审批、证据、回放 |
| 成功标准 | agent 更自治、更会做事 | 人类更少迷航，系统更可控、更可追责 |

最关键的差异不是功能清单，而是主语不同：

- Hermes 的主语是 **Agent**
- RoamBench 的主语是 **Project Execution**

## 4. 应该借鉴的部分

### 4.1 MCP 接入方式

Hermes 对 MCP 的使用说明 MCP 很适合做工具扩展标准。

RoamBench 可以借鉴这一点，但不能把 MCP 直接暴露为核心控制层。正确边界是：

```text
Agent Runtime
-> RoamBench Adapter
-> Tool Gateway
-> MCP servers / local tools / runtime tools
```

MCP 提供能力，Tool Gateway 负责：

- capability 映射
- permission grant 检查
- policy enforcement
- audit event 记录
- artifact capture
- checkpoint 触发

### 4.2 Skills / Procedural Memory

Hermes 的 skills 思路值得借鉴，但 RoamBench 的 Skill 不应是某个 agent 的私有 prompt 或个人经验。

RoamBench 的 Skill 应该是：

- agent-neutral
- system-owned
- versioned
- 可审计
- 可绑定 Policy
- 可编排为 Runbook

因此 `Skill` 更接近项目控制系统里的“任务类型能力包”，例如 `bug_fix`、`code_change`、`test_repair`、`dependency_upgrade`。

### 4.3 API Server / Frontend 分离

Hermes 的 API server 与前端分离方向可以借鉴。

RoamBench 也应保持：

- 控制面 API
- runtime adapter
- 前端态势 UI
- tool gateway
- persistence layer

这些层要解耦，避免某个 agent runtime 决定整个系统结构。

### 4.4 Scheduled Task 思路

Hermes 的 cron 能力说明长时运行 agent 需要定时触发。

RoamBench 应借鉴“可定时触发”的能力，但规则不应只是 cron。RoamBench 的定时规则应绑定状态：

- `no_progress_minutes`
- `session_review_minutes`
- `stale_waiting_review_hours`
- `retry_backoff_minutes`
- `daily_project_digest`

定时触发如果需要人类处理，应物化为 `Checkpoint`，而不是只发一条普通消息。

## 5. 不应该照搬的部分

### 5.1 不把 self-improving 当核心卖点

Self-improving 对 agent runtime 有吸引力，但 RoamBench 的核心不是让某个 agent 越来越像一个个人助理。

RoamBench 的核心是：

- task 是否按计划推进
- claim 是否有证据
- review 是否完成
- policy 是否被执行
- 人类是否只在关键节点被打断
- 失败后是否能恢复与回放

### 5.2 不把 Hermes Memory 作为 canonical history

Agent memory 可以作为执行辅助，但不能成为项目事实来源。

RoamBench 的 canonical records 必须是：

- `Event`
- `Artifact`
- `Claim`
- `Review`
- `Decision`
- `Checkpoint`

Hermes memory 最多映射为：

- `external_memory_ref`
- 辅助 artifact
- agent-local context

它不能替代 RoamBench 的审计历史。

### 5.3 不做 agent 群聊或个人助理入口

RoamBench 不应该把方向扩散成“所有消息入口的超级 agent”。

产品边界应该收紧在：

- developer project
- repo / diff / tests / artifacts
- workstream / task / session
- approval / replay / recovery

这也是区别于广义个人 agent runtime 的关键。

## 6. RoamBench 的创新方向

我们当然需要创新，但创新点不应是“再做一个 Hermes”。

RoamBench 应该创新在以下方向：

### 6.1 Project-Native Execution Graph

把长期任务建模为：

```text
Project
-> Workstream
-> Task
-> Runbook Phase
-> Session
-> Event / Artifact / Claim / Review / Decision
```

这比 agent session 更贴近真实软件项目。

### 6.2 Claim / Review / Decision 协议

多个 agent 不应该靠群聊协作，而应该靠结构化协议协作：

```text
Claim
-> Evidence
-> Review
-> Decision
```

Worker 负责提出 claim 和证据，Reviewer 负责质疑与验证，PolicyEngine 决定是否继续、重试、升级或等待人类。

### 6.3 Policy-Bound Runbook

Hermes skills 更偏 agent 能力；RoamBench skills 应该和 Policy 绑定。

默认任务闭环应是：

```text
Plan
-> Implement
-> Test
-> Review
-> Fix / Replan
-> Final Validation
-> Human Acceptance
```

没有通过测试、审查、证据和 completion rules，就不能进入最终验收。

### 6.4 Evidence-Native UI

RoamBench 的 UI 不应该先呈现长篇 agent 输出，而应该先呈现：

- 当前状态
- 阻塞点
- 修改范围
- 测试结果
- review objection
- 关键证据
- 下一步需要谁决策

原始日志保留，但不作为第一层信息。

### 6.5 Agent-Neutral Skill Registry

Skill Registry 应该独立于 agent。

同一个 `bug_fix` skill 可以由不同 runtime 执行：

- Hermes
- Codex
- OpenHands
- local CLI agent
- remote worker

这样 RoamBench 才不会被任何一个 agent 的 prompt、memory 或工具语义锁死。

### 6.6 Tool Gateway As Control Boundary

Tool Gateway 是 RoamBench 的关键创新边界。

它不只是工具代理，而是负责把所有外部工具调用转换成可审计事实：

- tool call event
- permission decision
- captured artifact
- policy violation
- checkpoint trigger

这样 MCP、runtime tool、本地测试、浏览器、CI、issue tracker 都能进入同一套控制模型。

### 6.7 Workspace Isolation Model

RoamBench 应显式区分：

```text
shared_repo
isolated_worktree
read_only_snapshot
```

这比简单“给 agent 一个目录”更适合多 agent 并行开发。

Reviewer 默认不应和 Worker 共用可写 workspace。多个 Worker 修改同一文件集合时也应触发隔离、锁定或 checkpoint。

### 6.8 Replay And Recovery As Product Surface

RoamBench 的优势不只是“任务做完”，而是任务失败或跨天后仍能回答：

- 发生过什么
- 哪个 agent 做了什么
- 为什么这么做
- 哪些证据支持结论
- 哪个 decision 改变了状态
- 从哪个 checkpoint 可以恢复

这部分应该成为核心产品界面，而不是日志附件。

## 7. 推荐集成架构

Hermes 应作为 `RuntimeAdapter` 接入。

```text
Task
-> selected Skill / Runbook
-> Runbook Phase
-> RuntimeService
-> HermesAdapter
-> Hermes API Server
-> Hermes Agent
```

回流方向：

```text
Hermes output / tool call / memory ref
-> HermesAdapter
-> Event / Artifact / Claim
-> Review / Decision / Checkpoint
-> Project Dashboard
```

### 7.1 Adapter 映射

| Hermes 侧对象/行为 | RoamBench 映射 |
|---|---|
| session / task run | `Session` + `PhaseAttempt` |
| skill invocation | `PhaseAttempt.skill_ref` / `Artifact` |
| tool call | `Event(type=tool_call)` + optional `Artifact` |
| MCP result | canonical `Artifact` 或 `Event` |
| final response | `Claim` + summary artifact |
| memory write | `external_memory_ref`，不作为 canonical history |
| cron trigger | `ScheduleRule` 或 external trigger event |
| error / failure | `Event(type=failure)` + `failure_reason` |

### 7.2 Adapter 不应该拥有的权力

HermesAdapter 不应直接决定：

- Task 是否完成
- 是否进入 human acceptance
- 是否扩大文件权限
- 是否修改 protected paths
- 是否跳过 review
- 是否把 Hermes memory 写成 canonical project history

这些必须由 RoamBench 的 PolicyEngine、Review、Decision、Checkpoint 处理。

## 8. 设计规则

为避免被某个 agent runtime 绑死，建议写入系统规则：

1. 所有 agent runtime 都通过 Adapter 接入，不直接写核心状态。
2. Task 状态只能由 RoamBench 服务层根据 Event / Artifact / Claim / Review / Decision 推进。
3. Skill / Runbook 属于 RoamBench，不属于单个 agent。
4. MCP 只作为 Tool Gateway 背后的工具协议，不作为项目历史来源。
5. Agent memory 可被引用，不可替代 canonical records。
6. Reviewer 默认只读或隔离 workspace。
7. 定时任务必须绑定状态规则，不能绕过 Policy。
8. 最终验收必须经过 completion rules 和 checkpoint，不由 agent final answer 直接完成。

## 9. 产品判断

Hermes 的价值在于增强 agent runtime。RoamBench 的价值在于让多个 runtime 在项目语义下协作。

因此产品战略应该是：

> 借鉴 Hermes 的执行能力，但不让 Hermes 拥有 RoamBench 的项目控制主权。

Hermes 可以成为强 worker；但 RoamBench 必须拥有项目级大脑：

- task graph
- policy engine
- skill registry
- runbook state
- tool gateway
- evidence protocol
- decision log
- checkpoint workflow
- replay surface

这才是 RoamBench 与 Hermes 的根本区别，也是值得创新的地方。

## 10. 参考资料

后续重新评估 Hermes 时，应优先核对官方来源：

- GitHub: <https://github.com/NousResearch/hermes-agent>
- Memory: <https://hermes-agent.nousresearch.com/docs/user-guide/features/memory/>
- Skills: <https://hermes-agent.nousresearch.com/docs/user-guide/features/skills/>
- MCP: <https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp/>
- Cron: <https://hermes-agent.nousresearch.com/docs/user-guide/features/cron/>
- Security: <https://hermes-agent.nousresearch.com/docs/user-guide/security/>
