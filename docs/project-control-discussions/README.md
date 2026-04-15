# 项目控制讨论整理

整理时间：`2026-04-13` UTC
更新时间：`2026-04-15` UTC（v0.2 + 扩展文档）

来源：

- 你提供的分享页：<https://chatgpt.com/share/69dc6437-d0b8-83a0-8660-6ec67257c76a>
- 本轮对话中围绕该分享页形成的补充整理
- v0.2 审查反馈与完善
- 产品定位、UI 架构、自治策略、实施计划、商业策略的深度讨论

## 这批讨论的核心结论

这组讨论的中心不是"如何做一个更好的 terminal 管理器"，也不是"如何做一个更聪明的单 agent"，而是：

> 如何为复杂软件项目提供一个面向多工作线、多 agent、多 runtime、跨多天执行的控制系统，让人始终不迷失方向。

更直接地说：

> RoamBench 要工程化推进复杂项目，而不是做一个多 agent 聊天平台。

由此收束出的几条主线是：

- 顶层对象必须是 `Project / Workstream / Task`，而不是 terminal 或聊天线程。
- 系统要默认自动处理可规则化、可验证、可回放的问题，只把真正需要判断力的部分升级给人类。
- 协作协议应该是 `Claim -> Review -> Decision`，而不是 agent 群聊。
- 证据和历史必须结构化：前者落到 `Artifact`，后者落到 `Event`。
- 自治是手段，不是目标；系统的最终目标是保持方向感、证据感、决策感。

## 文件结构

### 核心设计文档

- [system-requirements-v0.2.md](./system-requirements-v0.2.md)
  面向复杂软件项目的控制台定义、产品目标、非目标、需求优先级、核心判断、迁移路径与 Agent 接口契约。
- [policy-and-decision-rules-v0.2.md](./policy-and-decision-rules-v0.2.md)
  规则文件、自动化边界、Decision Classifier（含 Class B 边界判定）、质量门槛、审批逻辑、策略版本绑定与规则冲突解决。
- [task-runbook-and-skills-v0.1.md](./task-runbook-and-skills-v0.1.md)
  Task 执行闭环、Runbook 作为控制内核、Skill 作为执行策略层、阶段权限、Tool Gateway / MCP 接入、共享文件体系、定时规则与通知规则。
- [data-model-v0.2.md](./data-model-v0.2.md)
  规范对象模型、核心实体、关系链、状态机（含 `execution_complete` 重命名）、Task 依赖、Session 角色拆分、Event 管理与实施顺序。
- [cross-cutting-concerns-v0.1.md](./cross-cutting-concerns-v0.1.md)
  存储后端、API 设计方向、并发与竞态、Agent 协议、测试策略、迁移映射表。

### 产品与策略文档

- [product-positioning-v0.1.md](./product-positioning-v0.1.md)
  产品定位、竞争分析（vs Codex/Anthropic/JetBrains/Hermes/OpenClaw）、八条产品原则、共享层设计、Hermes 集成策略、三层摘要原则。
- [hermes-agent-comparison-v0.1.md](./hermes-agent-comparison-v0.1.md)
  Hermes Agent 对比、可借鉴能力、不可照搬边界、RoamBench 的创新方向与 HermesAdapter 集成规则。
- [openclaw-comparison-v0.1.md](./openclaw-comparison-v0.1.md)
  OpenClaw 对比、自动开发边界、skills/standing orders/subagents 可借鉴点，以及为什么 chat 只能作为交互层而非控制层。
- [information-architecture.md](./information-architecture.md)
  顶层双模式（Terminal / Project Panel）、Project Panel 逐层递进结构、Workstream Board / Task Detail / Session Detail / Approvals Inbox 的当前设计稿。
- [ui-information-architecture-v0.1.md](./ui-information-architecture-v0.1.md)
  较早期的平铺导航草图，保留作参考；当前应以 `information-architecture.md` 为准。
- [autonomy-policy-v0.1.md](./autonomy-policy-v0.1.md)
  自治层级、Task spawn / Session spawn / Review trigger 策略、预算约束、checkpoint 触发条件、merge 与 replay 规则。
- [implementation-plan-v0.1.md](./implementation-plan-v0.1.md)
  12 周实施计划（6 阶段）、后端目录结构、服务层职责划分、PolicyEngine 接口、核心状态流、后端约束。
- [business-strategy-v0.1.md](./business-strategy-v0.1.md)
  开源价值分析、商业分层（Free/Pro/Team/Cloud）、变现路径、本地 App 方向、流量分析。

## 建议阅读顺序

### 第一轮：理解问题与系统定义

1. `product-positioning-v0.1.md` — 产品是什么、不是什么、竞争格局
2. `hermes-agent-comparison-v0.1.md` — 与 Hermes 的差异、借鉴边界与创新方向
3. `openclaw-comparison-v0.1.md` — 与 OpenClaw 的差异、自动化原语和 chat 边界
4. `system-requirements-v0.2.md` — 问题定义、边界、需求优先级
5. `policy-and-decision-rules-v0.2.md` — 哪些判断由系统接管、哪些升级给人类

### 第二轮：理解系统如何构建

6. `task-runbook-and-skills-v0.1.md` — Task 如何按 Skill / Runbook / Policy 闭环执行
7. `data-model-v0.2.md` — 核心对象模型与状态机
8. `autonomy-policy-v0.1.md` — 自治边界与控制规则
9. `cross-cutting-concerns-v0.1.md` — 存储、API、并发、协议

### 第三轮：理解如何实施与运营

10. `ui-information-architecture-v0.1.md` — 页面结构与交互设计
11. `implementation-plan-v0.1.md` — 12 周实施计划与后端架构
12. `business-strategy-v0.1.md` — 开源边界与商业策略

## v0.1 → v0.2 变更摘要

### system-requirements
- 新增 Section 5.2：明确单用户假设
- 新增 Section 11：从当前 RoamBench 的迁移路径（概念映射 + 迁移策略）
- 新增 Section 12：Agent 接口契约最小定义

### policy-and-decision-rules
- 细化 Section 7 Decision Classifier：Class B 边界判定规则与示例
- 新增 Section 11：策略版本绑定（Task 绑定创建时 Policy，不自动迁移）
- 新增 Section 12：规则冲突解决顺序（Escalation > Approval > Review > Quality Gate > Completion > Autonomy）

### data-model
- `Task.state` 中 `completed` → `execution_complete`，消除与验收状态的歧义
- 新增 `Task.policy_version_id` 和 `Task.dependencies` 字段
- `Session.role` 拆分为 `execution_role` + `system_role`
- 明确 `Claim.confidence` 仅用作 review 触发信号
- 新增 Event 管理指导（cursor 分页、保留策略、过滤）

### 新增文档（v0.1 → v0.2 阶段）
- `cross-cutting-concerns-v0.1.md`：存储、API、并发、Agent 协议、测试、迁移映射

### 新增文档（扩展阶段）
- `product-positioning-v0.1.md`：产品定位、竞争分析、产品原则、共享层、Hermes 策略
- `hermes-agent-comparison-v0.1.md`：Hermes Agent 对比、借鉴边界、创新方向、Adapter 集成规则
- `openclaw-comparison-v0.1.md`：OpenClaw 对比、自动开发边界、可借鉴自动化原语、chat 非控制层原则
- `ui-information-architecture-v0.1.md`：页面架构、三层摘要、历史记录、查询映射
- `autonomy-policy-v0.1.md`：自治规则、预算、checkpoint、merge
- `implementation-plan-v0.1.md`：12 周计划、后端架构、服务层、状态流
- `business-strategy-v0.1.md`：开源边界、变现路径、商业分层
- `task-runbook-and-skills-v0.1.md`：Task 执行闭环、Runbook / State Machine 与 Skills 的边界、Tool Gateway / MCP、权限与定时规则

## 后续可继续扩展的方向

- 数据库 DDL（基于 data-model-v0.2 的 SQLite/PostgreSQL schema）
- `EXECUTION_POLICY.yaml`、`SKILL.yaml`、`RUNBOOK.yaml` 完整示例文件
- Agent Adapter 实现规范（Hermes adapter 作为第一个样例）
- 前端组件设计与原型
- E2E 测试计划
