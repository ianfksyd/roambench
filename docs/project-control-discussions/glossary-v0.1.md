# Glossary v0.1

本表定义项目控制系统的 12 个核心对象和关键概念。所有代码、API、数据库字段、文档和 UI 文案必须统一使用本表中的术语。

## 核心对象（12 个）

| # | English | 中文 | 代码标识 | 一句话定义 |
|---|---|---|---|---|
| 1 | **Project** | 项目 | `project` | 顶层容器，对应一个真实工程目标和共享代码空间。所有工作线、任务、运行环境和策略都挂在 Project 下。 |
| 2 | **Workstream** | 工作线 | `workstream` | 项目内的一条主题方向线，例如"认证重构"或"依赖升级"。下辖多个 Task。 |
| 3 | **Task** | 任务 | `task` | 系统最小业务执行单位，有明确目标、范围、状态和风险。被分派给 agent 在 runtime 上执行。 |
| 4 | **Session** | 会话 | `session` | Task 在某个 Runtime 上的一次执行实例。一个 Task 可以有多次 Session（重试、接管、reviewer）。 |
| 5 | **Runtime** | 运行环境 | `runtime` | 统一抽象本地机、远程机、容器或托管 agent 后端。Session 在 Runtime 上启动和运行。 |
| 6 | **Policy** | 策略 | `policy` | 项目执行宪法。定义完成标准、审查规则、审批边界、自治限制和升级条件。可版本化，不可就地覆盖。 |
| 7 | **Claim** | 主张 | `claim` | 某个 Session/Agent 提出的结构化声明，例如"bug 已修复"或"重构完成"。必须绑定证据。 |
| 8 | **Review** | 复核 | `review` | 另一个 Agent 或人对 Claim 的复核结论：支持、反对、不确定、需要更多证据。 |
| 9 | **Decision** | 裁决 | `decision` | 对 Claim / Review / Checkpoint 的最终裁决。是状态跃迁的开关，驱动 Task/Session 向前推进。 |
| 10 | **Artifact** | 产物 | `artifact` | 结构化证据容器：diff、测试结果、构建产物、命令痕迹、摘要等。Claim 的证据引用指向 Artifact。 |
| 11 | **Checkpoint** | 检查点 | `checkpoint` | 需要人类介入的节点。由系统在高风险动作、冲突、超预算等场景自动创建，进入 Approvals Inbox。 |
| 12 | **Event** | 事件 | `event` | 系统历史的骨架。每个关键动作（状态变化、主张提交、裁决等）都生成 Event。Timeline、Replay、Audit 从这里生成。 |

## 关键协议与概念

| English | 中文 | 说明 |
|---|---|---|
| **Claim → Review → Decision** | 主张 → 复核 → 裁决 | 核心协作协议。Agent 提出主张，附证据；Reviewer 复核；系统或人类做最终裁决。 |
| **Decision Classifier** | 决策分类器 | 在 orchestrator 前面的分类层。判断问题属于 A（规则可解）、B（证据可解）、C（需人类判断）、D（需重定义目标）。 |
| **Acceptance Status** | 验收状态 | Task 的业务验收生命周期，独立于执行状态：`not_ready → ready_for_acceptance → under_human_review → accepted / rejected`。 |
| **execution_complete** | 执行完成 | Task 执行状态的终态之一。表示执行层面已完成，但不等于业务已验收（验收由 `acceptance_status` 跟踪）。 |
| **Actor Identity** | 参与者身份 | 统一枚举：`human / agent / orchestrator / system / policy_engine`。所有"谁拥有 / 谁发起 / 谁裁决"字段复用这套类型。 |
| **row_version** | 行版本 | 乐观锁字段。所有可变核心对象都包含 `row_version`，状态迁移 API 必须携带 `expected_row_version`。 |

## 状态枚举速查

### Task.state

`planned → queued → running → waiting_review / waiting_human / blocked / failed → execution_complete → archived`

### Task.acceptance_status

`not_ready → ready_for_acceptance → under_human_review → accepted / rejected`

### Session.state

`queued → starting → active → waiting_review / waiting_human / paused / reconnecting / crashed / completed / terminated`

### Claim.status

`drafted → submitted → under_review → validated / rejected / superseded`

### Checkpoint.status

`pending → approved / rejected / expired`

## 命名规则

### 代码层

- 对象名用 **snake_case 单数**：`project`、`task`、`session`、`claim`
- 数据库表名用 **snake_case 复数**：`projects`、`tasks`、`sessions`、`claims`
- API 路径用 **kebab-case 或 snake_case 复数**：`/projects/:project_id/tasks/:task_id`
- 枚举值用 **snake_case**：`execution_complete`、`waiting_human`、`under_review`

### 文档层

- 英文用首字母大写：Project、Task、Session、Claim
- 中文用本表中文名：项目、任务、会话、主张
- 不发明同义词（例如不用"工单"替代"任务"，不用"审批"替代"裁决"）

### UI 层

- 列表名必须直接映射底层状态枚举：`running tasks`（不用 `active tasks`）、`running workstreams`（不用 `active workstreams`）
- 避免使用未在枚举中定义的裸术语（例如不在 API 或查询层使用 `active`）

## 缩写表

| 缩写 | 全称 | 说明 |
|---|---|---|
| IA | Information Architecture | 信息架构 |
| MCP | Model Context Protocol | 模型上下文协议，用于工具扩展 |
| DAG | Directed Acyclic Graph | 有向无环图，用于任务依赖 |
| PR | Pull Request | 合并请求 |
| SSO | Single Sign-On | 单点登录 |
| SAML | Security Assertion Markup Language | 安全断言标记语言 |
