# 手机交互控制与多 CLI 协同：阶段 1 进展

日期：2026-08-05
状态：实施中
范围：Interaction/Decision Gateway 第一至第四切片

## 当前结论

阶段 1 的新请求闭环、旧审批事实迁移、durable Task projector 和运行时 Checkpoint 单一事实源已经建立。当前代码可以让 Adapter 创建结构化 Interaction，由另一个已登录浏览器作答，并让 Adapter 通过读取或最长 30 秒的有限长轮询取得最终结果。决定、审计、outbox 和待投影记录在同一 SQLite 事务中提交；并发作答最多一个成功。

这不是阶段 1 完成声明。历史 Checkpoint/Decision 会在启动时从 JSON/WAL 幂等导入 SQLite，原文件保留只读迁移前备份，JSON 中的审批数组随后清空。运行期间旧 Project Control 路径创建、解决或过期 Checkpoint 时，会先把事实和 outbox 写入 SQLite，再保存不含审批数组的 JSON Task 投影；下一次旧路径变更会先从 SQLite 恢复兼容投影。决定后的 Task 更新由可恢复 projector 执行。完整 POST 幂等、自动 expiry/session cancel 和故障注入仍未完成；完成前不进入阶段 2。

## 已完成

- 新增纯 Go SQLite control-plane repository，数据库位于 `<persist_dir>/.project-control/control-plane.sqlite`，主文件权限为 `0600`。
- 启用 WAL、外键、`busy_timeout` 和耐久同步。
- 建立 Interaction、Response、Decision、Audit、Outbox、Delivery、Idempotency 和 schema migration 表。
- 创建 Interaction 时，`interaction.created` audit 与 outbox event 和请求记录原子提交。
- 响应时校验 pending 状态、row version、input hash、允许动作、选项集合及反馈长度。
- 相同响应幂等键返回原结果；不同客户端竞争同一 row version 时，一个成功，另一个得到 conflict。
- 支持单选、多选、文本和动作型 response schema 的仓储层校验。
- 支持 `GET`、最长 30 秒 `wait` 和版本化 `cancel`；服务重启后可继续读取最终或取消状态。
- 新增结构化 Agent API：
  - `POST /api/agent/v1/interactions`
  - `GET /api/agent/v1/interactions/:id`
  - `GET /api/agent/v1/interactions/:id/wait`
  - `POST /api/agent/v1/interactions/:id/cancel`
- 新增浏览器会话 API：
  - `GET /api/mobile/interactions`
  - `GET /api/mobile/interactions/:id`
  - `POST /api/mobile/interactions/:id/respond`
- 新 Interaction 会投影进现有 Project Control approvals inbox；旧 `/api/project-control/checkpoints/:id/decision` 入口处理这类请求时改走 SQLite Gateway。
- 启动时迁移旧 JSON/WAL Checkpoint、Decision 和关联 audit event，并记录来源 hash、更新时间和导入数量。
- 首次迁移前生成 `.pre-control-plane-v1.bak` 只读备份；SQLite 提交后清空 JSON 审批字段。
- 如果进程在 SQLite 提交后、JSON 清理前退出，重启会幂等重放迁移，不重复 Interaction 或 outbox。
- 决定事务原子写入 `task_projections` 和 `project_task_projection.requested` outbox event。
- projector 按 decision ID 幂等更新 final acceptance/archive override Task；成功后标记 applied，失败记录错误并保留重试状态。
- 浏览器响应会在 HTTP 返回前尝试投影；服务启动会续跑 pending/failed projection。
- 旧 Project Control 运行时写入会在持有用户状态锁时，把 Checkpoint/Decision 同步到 SQLite 后再写 JSON；SQLite 失败时 JSON 不提交。
- JSON 运行时文件不再保存 Checkpoint/Decision；旧读写路径在变更前从 SQLite 水合兼容投影。
- pending Checkpoint 过期或取消时会更新同一 Interaction 的状态和 row version，并生成幂等 `interaction.expired` 或 `interaction.cancelled` outbox event。
- SQLite 中已有的终态不会被迟到的旧 pending 投影重新打开。
- 旧 approvals inbox 决策改走 Gateway 后，仍投影 `decision_made`、`checkpoint_resolved` 和原有决定类型事件，保持 replay、筛选和 HTTP 错误语义。

## 已验证行为

- 创建后关闭并重新打开 repository，请求和 `interaction.created` outbox event 仍存在。
- 两个 goroutine 同时 approve/reject，最终只有一条 Response 和一条 Decision。
- 相同 idempotency key 重试返回同一个 `response_id`。
- 未声明选项被拒绝，声明的单选答案原样保存。
- wait 在请求解决后返回最终 action。
- cancel 状态跨服务重启保持。
- 未认证请求不能读取移动 Interaction API。
- 第二个浏览器决定返回 HTTP `409`，并携带当前最终 Interaction。
- 结构化请求可以从现有 approvals inbox 查看和决定。
- pending 与 resolved 旧审批迁移后仍通过兼容快照保持原 ID、状态和决定类型。
- 模拟“数据库已提交但 JSON 未清理”后重启，JSON 被继续清理且 outbox 不重复。
- final acceptance 决定在服务重启后可从 pending projection 恢复为 accepted Task。
- 重复运行 projector 不会再次增加 Task row version。
- projector 定向测试通过 Go race detector。
- `go test ./...` 通过。
- 运行时 agent checkpoint 通过旧 API 创建后，SQLite 可读取请求和 `interaction.created` outbox，JSON 审批数组为空。
- 第二次旧状态变更能从 SQLite 找回 pending checkpoint，将它过期为 row version 2，并只生成一次 `interaction.expired` outbox。
- 重放相同终态不会重复 outbox；更高 row version 的迟到 pending 写入也不能覆盖终态。
- 旧决策事件、task replay、checkpoint 事件筛选和二次决策状态码回归通过。

## 尚未完成的阶段 1 门槛

1. 为 Interaction 创建和 cancel 增加完整的持久化 POST 幂等结果；当前创建依靠 vendor request 唯一键去重，cancel 依靠 row version 防止重复状态迁移。
2. 增加 expiry 和 session-ended 自动取消处理。
3. 补齐空状态 fixture，并对 migration、projector 和事务故障执行 fault-injection，证明失败时没有部分 audit/outbox。

## 下一切片

下一切片补齐 Interaction 创建和 cancel 的持久化 POST 幂等结果，明确同 key 同 payload 返回原结果、同 key 不同 payload 返回 conflict。随后实现 expiry/session 自动取消和 fault-injection。满足全部退出条件后才把阶段 1 标记为完成。
