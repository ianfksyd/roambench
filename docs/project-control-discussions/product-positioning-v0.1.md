# Product Positioning v0.1

## 1. 产品一句话定义

一个让用户在本地或远程机器上启动、分发、监督、恢复、接管 AI agent 任务的轻量控制台。

英文定位：

> A lightweight, self-hostable runtime console for developer agents.
> Model-neutral. Local + remote. Timeline, evidence, replay, and human checkpoints.

中文内部定义：

> 开发者 agent 的运行控制台。不是 IDE，不是聊天窗，不是 terminal 壳。

## 2. 它解决的不是"怎么敲命令"

而是这五个问题：

1. 任务发到哪里执行
2. 哪个 agent 在执行
3. 当前状态是什么
4. 什么时候需要人介入
5. 失败后如何恢复或接管

## 3. 产品原则（内部宪法）

### 3.1 八条核心原则

1. **项目高于会话。** 顶层对象必须是 Project / Workstream / Task，而不是 terminal 或 chat thread。
2. **任务高于终端。** 每条工作线必须先成为一个 Task。Terminal 只是执行视图。
3. **共享代码空间高于共享聊天。** 对开发任务，真实共享上下文首先是 repo、diff、tests、artifacts。
4. **跨 agent 历史必须持久且可追责。** 历史要跨 agent、跨 session、跨天持续存在。
5. **输出先给状态与证据，不先给长文。** 人不应先读长篇汇报，而应先看结构化状态与关键证据。
6. **协作采用 Claim / Review / Decision 协议。** 不是 agent 群聊，而是结构化裁决。
7. **人类只在关键节点裁决和接管。** 默认 agent 持续工作，人在 checkpoint / 冲突 / 高风险时介入。
8. **Local + Remote + 多 agent 必须统一在一个控制面。**

### 3.2 对象原则

- 项目高于会话。任务高于终端。共享代码空间高于共享聊天。
- 主张、证据、审查、裁决必须独立建模。
- 历史必须跨 agent 持久共享。

### 3.3 页面原则

- 首页先给态势，不先给日志。
- 详情页先给时间线和证据，再给终端。
- 人类必须有统一审批入口。
- 回放与历史必须是一等页面。

### 3.4 状态原则

- 任务、会话、主张、裁决都有独立状态机。
- Checkpoint 必须推动真实状态变化。
- 所有关键动作都生成事件。
- 默认任务会中断、会偏航、会跨天。

## 4. 产品定位与竞争分析

### 4.1 竞争格局

| 竞争者 | 定位 | 强项 | 与我们的差异 |
|---|---|---|---|
| **OpenAI Codex app** | 自家生态的多 agent 开发工作台 | 项目、并行线程、diff 审阅、人工接管 | 偏 OpenAI 闭环生态 |
| **Anthropic Managed Agents** | 托管式 agent harness | 长时异步任务、受管基础设施 | 偏云端托管 |
| **JetBrains Central** | 组织级 control and execution plane | agent-driven software production | 偏大组织、跨 IDE |
| **OpenClaw** | 个人自托管 agent automation platform | 跨消息入口、skills、standing orders、subagents、持续运行 | 偏广义个人助理与聊天入口自动化 |
| **Hermes** | 自托管 agent runtime / 自进化 agent | skills、memory、MCP、cron、runtime decoupling | 偏 agent 本体与执行智能 |
| **OpenHands** | Model-agnostic coding-agent 平台 | 可自托管、可审计、子 agent 委派 | 偏 coding-agent 平台 |
| **LangGraph / LangSmith** | Workflow 框架 + 可观测层 | 图结构、state、tracing | 是框架层，不是终端产品 |

### 4.2 我们的空位

真正还没被充分占住的是：**轻量、模型中立、开发者优先、local + remote 统一、强调历史与证据的 project execution control plane。**

更直接地说：

> RoamBench 要工程化推进复杂软件项目，而不是做一个更热闹的 agent 聊天平台。

### 4.3 五个竞争力点

1. **更轻** — 安装、启动、接入 runtime 的成本压到最低
2. **更可控** — 审批点、风险拦截、人工接管、失败恢复、回放
3. **更开放** — 开源、插件化、允许接入不同 agent 和 runtime
4. **更贴近实际工程流** — 围绕 repo、tests、diff、logs、session、rollback、human approval
5. **更适合从个人走向小团队** — 先把个人和 2-10 人小团队吃透

### 4.4 不该正面打的方向

- 不和 Codex app 比"谁更像 AI IDE"
- 不和 Anthropic Managed Agents 比"谁的托管 harness 更省心"
- 不和 JetBrains Central 比"谁更适合大组织"
- 不和 OpenClaw 比"谁更适合做通用个人助理或聊天入口自动化"

### 4.5 护城河

不是"我们也支持多 agent"。不是"我们也能长时运行"。

真正的护城河应该是：

> **开发者任务语义 + runtime 控制 + 证据化审阅 + 历史回放**

## 5. 设计初衷

在复杂项目（100 万行代码以上）上经常有多条线同时进行，有时需要好几天。每条线都需要计划、代码撰写、实施、测试、循环等过程。人控制这么多 agent（agent 中立，会用不同 agent）非常累，也很容易迷失方向。

核心痛点不是缺 agent，而是缺一个能让自己不迷航的上层控制面。

### 5.1 vibe coding 的核心问题

CLI 给的建议经常是长篇幅汇报，人类的作用变成纠正 AI 的错误。但 AI 无法像人类一样形成新假设和做创造性决策。

解决方案不是"让 agent 更像人"，而是：

- 把可规则化的决策自动化（`Automate policy`）
- 把需要证据的问题交给 reviewer（`Review evidence`）
- 把需要判断力的问题交给人类（`Escalate judgment`）

### 5.2 三层摘要原则

永远不要让人类先读全文：

- **Layer 1：一句话状态** — "已完成依赖升级，测试 2 项失败"
- **Layer 2：结构化摘要** — 修改文件 4 个、测试 8 过 / 2 失败、风险中
- **Layer 3：完整输出** — 只在需要时展开看原始 CLI 文本

### 5.3 工程化推进复杂项目

RoamBench 的目标不是让多个 agent 互相聊天，而是把复杂项目推进变成可管理的工程系统。

工程化推进意味着：

- 目标先落到 `Project / Workstream / Task`
- 每个 Task 绑定 `Skill / Runbook / Policy`
- 每次执行沉淀为 `Session / PhaseAttempt`
- 每个关键动作生成 `Event / Artifact`
- 每个完成主张必须通过 `Claim / Review / Decision`
- 需要人类判断时才生成 `Checkpoint`
- 最终验收必须满足 completion rules，而不是相信 agent final answer

因此，chat/thread 可以作为交互入口，但不能成为控制层或历史真相来源。

## 6. Hermes Agent 集成策略

### 6.1 定位

Hermes 值得接，不值得被当成底层命运共同体。应该站在 Hermes 之上，但不依附于 Hermes。

详细对比见 [hermes-agent-comparison-v0.1.md](./hermes-agent-comparison-v0.1.md)。

### 6.2 集成方式

- Hermes 作为一种 **可插拔 worker / reviewer agent**
- 上层控制台保持 agent-neutral
- 通过 adapter 接入 Hermes API server
- Hermes 负责执行子任务、调工具、利用技能/记忆、产出 claim 和 evidence
- 控制台负责 orchestration、visualization、approvals、evidence、replay

### 6.3 从 Hermes 借鉴的部分

- MCP 作为工具扩展标准
- skills / procedural memory 的思路（用于 task template / reusable recovery patterns）
- API server 与前端分离的架构
- Runtime decoupling 思路

### 6.4 不借鉴的部分

- 不把 self-improving 当中心卖点
- 不做多消息平台网关
- 不把持久化模型绑到 Hermes 的 memory 语义

### 6.5 创新边界

RoamBench 不应该创新成“另一个 Hermes”，而应该创新在 Hermes 没有站稳的项目控制层：

- Project / Workstream / Task 原生的执行图
- agent-neutral 的 Skill Registry 与 Runbook
- Policy-bound permissions，而不是 agent 自行决定权限
- Claim / Review / Decision 协作协议
- Evidence-native UI，而不是先呈现长篇 agent 输出
- Tool Gateway 统一 MCP、本地工具、runtime tools 与审计记录
- Workspace isolation：`shared_repo` / `isolated_worktree` / `read_only_snapshot`
- Checkpoint、final acceptance、replay 与 recovery 作为一等产品表面

边界原则：

> Hermes owns agent-local execution intelligence.
> RoamBench owns project-level control, audit, policy, and acceptance.

## 7. OpenClaw 借鉴边界

详细对比见 [openclaw-comparison-v0.1.md](./openclaw-comparison-v0.1.md)。

OpenClaw 值得借鉴，但不应该成为 RoamBench 的产品模型。

应借鉴：

- skills 发现、优先级、allowlist 与 registry 思路
- standing orders 中的 scope / trigger / approval gate / escalation rule
- cron、heartbeat、background tasks、task flow 等自动化原语
- subagent 的 timeout、并发上限、嵌套深度、tool policy、cascade stop
- hooks / webhooks 作为外部触发机制

不应照搬：

- chat 作为 source of truth
- 多 agent 群聊作为编排模型
- persona / memory 作为项目事实来源
- broad standing authority
- 把自动开发塞进一个超大 skill prompt

RoamBench 中的自动开发应该是：

```text
Skill
-> Runbook
-> Policy
-> WorkspaceRef
-> PhaseAttempt
-> Artifact / Claim
-> Review / Decision
-> Checkpoint
```

边界原则：

> Chat is an interface, not the control plane.
> Skills are capability packages, not autonomy grants.
> Automatic development is a project execution system, not a single agent prompt.

## 8. 共享层设计

### 8.1 四层共享范围

| 范围 | 内容 | 说明 |
|---|---|---|
| **Scope A: Workspace Memory** | repo、当前分支、文件树、未提交 diff、测试结果、构建结果 | 所有 agent 都能读到的公共事实层 |
| **Scope B: Task Memory** | 任务目标、约束、负责 agent、中间结论、失败记录 | 局部共享，不必让所有 agent 读全量 |
| **Scope C: Review Memory** | claim、证据、reviewer 结论、人类裁定 | 审查轨迹 |
| **Scope D: Project History** | 重要决策、关键失败、回滚原因、checkpoint 决策 | 项目级长期记忆 |

### 8.2 多 agent 协作模型

不做"所有 agent 在一个群聊里互相说话"。采用 **manager + workers + reviewers** 模式：

- **Manager**：拆任务、分派、汇总状态、触发 checkpoint、发起 review
- **Worker Agents**：直接对代码库工作（implement、test、docs、refactor）
- **Reviewer Agents**：审查而非主写（验证 patch、检查 claim 证据、审查风险、找回归）

共享的不是"长篇话术"，而是"结构化主张 + 证据包"。
