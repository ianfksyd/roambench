# Cross-Cutting Concerns v0.1

本文档覆盖跨越需求、策略和数据模型三份文档的共性问题。

## 1. 存储后端

### 当前阶段：SQLite

- 当前 RoamBench 是单用户系统，SQLite 足以支撑
- 所有核心对象（Project、Task、Session、Event 等）用单库存储
- Event 表预计增长最快，应提前设计分区或归档策略

### 未来扩展：PostgreSQL

- 如果引入多人类协作，需要迁移到 PostgreSQL
- 迁移路径：保持 SQL schema 兼容，使用标准 SQL 子集，避免 SQLite 特有语法
- 建议从第一天开始使用 migration 工具（如 golang-migrate）管理 schema 变更

## 2. API 设计方向

### 基本原则

- REST API，路径结构对齐数据模型层级
- 查询视图（data-model-v0.2.md Section 8）直接映射为 API endpoint

### 建议端点结构

```text
GET  /api/v1/projects/:project_id/dashboard        → Project Dashboard Query
GET  /api/v1/projects/:project_id/tasks/:task_id   → Task Detail Query
GET  /api/v1/projects/:project_id/approvals        → Approvals Inbox Query
GET  /api/v1/projects/:project_id/events           → Replay Query (cursor-based)

POST /api/v1/projects/:project_id/tasks            → 创建 Task
POST /api/v1/projects/:project_id/tasks/:task_id/claims      → 提交 Claim
POST /api/v1/projects/:project_id/tasks/:task_id/reviews     → 提交 Review
POST /api/v1/projects/:project_id/tasks/:task_id/decisions   → 创建 Decision
POST /api/v1/projects/:project_id/checkpoints/:checkpoint_id/resolve → 解析 Checkpoint
```

### 实时通信

- 现有 WebSocket 基础设施可复用
- Event 流可通过 WebSocket 推送到前端，驱动 dashboard 实时更新
- 建议使用 server-sent 模式：后端推送 Event，前端订阅特定 project/task 的事件流

## 3. 并发与竞态条件

### 核心问题

多个 Session 可能同时尝试修改同一个 Task 的状态（例如两个 agent 同时报告完成）。

### 解决方案：乐观锁

- 每个可变核心对象增加 `row_version` 字段（整数递增）
- 状态迁移 API 必须携带 `expected_row_version`
- 如果 `expected_row_version` 与当前 `row_version` 不匹配，返回 409 Conflict
- 调用方必须重新读取最新状态后重试

### 状态迁移的原子性

- 一次状态迁移 = 一次数据库事务
- 事务内同时：更新对象状态、写入 Event、更新 `row_version`
- 不允许跨事务的"两步迁移"（先更新状态，再补 Event）

### Session 级并发

- `Runtime.max_concurrent_sessions` 控制 runtime-local 的硬上限
- `policy.autonomy_limits.max_concurrent_sessions` 控制 policy / project scope 的调度上限
- Session 创建前必须同时检查这两类上限；只有两者都满足时才允许进入实际执行
- 占用执行槽位的 session 状态至少包括：
  `starting`、`active`、`paused`、`waiting_review`、`waiting_human`、`reconnecting`
- `queued`、`crashed`、`completed`、`terminated` 不计入占槽并发
- 如果任一上限不满足，Session 进入 `queued` 等待，而不是被拒绝
- 当 runtime 和 policy 两侧都释放出槽位时，`queued` session 再推进到 `starting`

## 4. Agent 协议

### 概览

Agent 协议定义了系统与 agent 之间的最小通信契约。详见 system-requirements-v0.2.md Section 12。

### 通信模式

建议采用 agent 轮询模式（pull-based），而不是系统推送模式（push-based）：

- Agent 启动后，定期向系统查询"我应该做什么"
- Agent 完成工作后，主动向系统提交 Claim 和 Artifact
- 系统不需要维护到 agent 的长连接

这样做的好处：

- Agent 崩溃后，系统不需要处理连接断开
- 新 agent 实现只需要实现 HTTP client，不需要实现 server
- 与现有 CLI 工具（Claude Code、Codex 等）的适配更自然

### Agent 适配层

对于不原生支持该协议的 agent（如现有 CLI 工具），系统应提供 adapter：

- Adapter 是一个 wrapper process，负责将 CLI agent 的 stdout/stderr 转化为结构化 Claim 和 Event
- Adapter 本身作为一个 Session 运行

## 5. 测试策略

### 最高价值测试目标

1. **状态机正确性**
   - 所有状态迁移路径的合法性验证
   - 非法迁移的拒绝验证
   - 建议使用 property-based testing：随机生成状态迁移序列，验证不变量

2. **策略引擎**
   - 规则评估的正确性
   - 规则冲突解决的优先级（参见 policy-and-decision-rules-v0.2.md Section 12）
   - 边界条件（阈值恰好等于、刚好超过）

3. **并发安全**
   - 乐观锁的冲突检测
   - 同一 Task 的并发状态迁移
   - Session 并发上限的强制执行

### 不需要优先测试的

- UI 布局和样式
- Agent 的具体实现
- 日志格式

## 6. 从当前 RoamBench 的迁移映射

### 详细概念映射

| 当前 RoamBench 概念 | 新系统概念 | 迁移说明 |
|---|---|---|
| Go HTTP server | 继续作为 API server | 新增 REST endpoint |
| WebSocket 连接 | Runtime 通信通道 | 复用现有基础设施 |
| tmux session | Session | 成为 Task 下的执行实例 |
| tmux pane | 执行视图（嵌入 Task Detail） | 不再作为产品首页 |
| workspace tab | Project 级 UI 视图 | 语义从"布局"变为"项目" |
| 文件浏览器 | Artifact URI 入口 | 继续存在 |
| TOML 配置 | Policy YAML | 配置从"系统设置"升级为"项目执行宪法" |
| bcrypt 认证 | 单用户身份 + Actor Identity | 认证层可复用，新增 actor 类型区分 |
| session metadata (disk) | Event + Artifact (DB) | 从文件存储迁移到结构化数据库 |

### 迁移阶段

1. **阶段一（数据层）**：在现有 Go 后端新增 SQLite 数据库、核心对象 schema、REST API，以及最小 `Checkpoint` record schema
2. **阶段二（控制层）**：新增 Project Dashboard 前端视图，终端视图嵌入 Task Detail
3. **阶段三（智能层）**：引入策略引擎、Decision Classifier、自动 Checkpoint 路由与 richer approval workflow
