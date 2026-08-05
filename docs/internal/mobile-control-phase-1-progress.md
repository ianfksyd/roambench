# 手机交互控制与多 CLI 协同：阶段 1 进展

日期：2026-08-05
状态：已完成
范围：Interaction/Decision Gateway 第一至第七切片

## 当前结论

阶段 1 的退出条件已经满足。Adapter 可以创建结构化 Interaction，由另一个已登录浏览器作答，并通过读取或最长 30 秒的有限长轮询取得最终 Interaction 和原始 Response。单选答案、文本反馈、`responseId` 和最终动作会原样返回；等待期间服务重启后，客户端重试仍得到同一结果。决定、审计、outbox、幂等键和待投影记录在同一 SQLite 事务中提交；并发作答最多一个成功。

历史 Checkpoint/Decision 会在启动时从 JSON/WAL 幂等导入 SQLite，原文件保留只读迁移前备份，JSON 中的审批数组随后清空。运行期间旧 Project Control 路径创建、解决或过期 Checkpoint 时，会先把事实和 outbox 写入 SQLite，再保存不含审批数组的 JSON Task 投影；下一次旧路径变更会先从 SQLite 恢复兼容投影。决定后的 Task 更新由可恢复 projector 执行。空状态 fixture 与创建、响应、expiry、session cancel、迁移和 projector 的故障恢复矩阵均已通过，阶段 2 可以开始。

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
- 新增持久化 POST 幂等表，按 username、actor scope、operation 和 idempotency key 隔离记录。
- Interaction 创建和 cancel 在各自业务事务内保存规范化请求 hash 与原始 Interaction JSON；业务提交和幂等结果不会部分成功。
- 同 key、同 payload 在服务重启后仍返回第一次 HTTP 结果；同 key、不同 payload 返回 conflict。
- 创建幂等重放返回第一次创建时的 pending/row version 1 快照，不会被资源后来 resolved 或 cancelled 的当前状态替换。
- 到达 `expires_at` 的 pending Interaction 会原子转为 expired、写入 `interaction.expired` audit/outbox，并把 row version 增加一次。
- 服务启动时先恢复停机期间到期的请求；运行期间每秒扫描一次，pending list 和 wait 也会主动收敛到期状态。
- 手机响应事务会再次检查截止时间；截止时间后的响应不能抢在 expiry 之后形成 Decision。
- terminal manager 提供 session-ended 生命周期回调；HTTP 删除、空闲清理、存储上限清理和 tmux 消失均会取消该 session 的 pending Interaction。
- session cancel 和手机决定通过 pending/row-version 条件更新竞争；已 resolved、expired 或 cancelled 的终态不会被覆盖。
- Agent `GET`/`wait` 保持原有顶层 Interaction JSON 字段，并在 resolved 时附带唯一的结构化 `response`，供 Adapter 取得选择项、文本反馈和稳定 `responseId`。
- 增加显式空 Project Control JSON fixture，空状态启动后不会生成 Interaction，也不会把审批事实写回 JSON。
- 使用真实 SQLite `RAISE(ABORT)` trigger 覆盖创建、响应、expiry、session cancel 和旧审批迁移的末端写入故障；每个用例均验证整笔事务回滚并可安全重试。
- projector 在目标 Task 缺失时记录 failed/attempt count，不在 JSON 留下半成品事件；Task 恢复后可重试并幂等完成。

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
- Interaction 创建后即使资源随后取消，重启后的同 key 重试仍返回第一次 `201` 的原始 pending 快照。
- cancel 在重启后使用同 key、同 payload 重试返回第一次 `200` 结果，不重复状态迁移。
- 创建摘要或取消原因改变但复用原 key 时返回 HTTP `409`。
- 截止时间后的手机响应返回 conflict，Interaction 变为 expired/row version 2，且不产生 Response。
- 请求在服务停机期间到期后，首次重启生成一次 `interaction.expired` outbox；再次重启不重复事件。
- terminal session 结束后，同 session 的 pending Interaction 变为 cancelled，已经批准的 Interaction 保持 resolved。
- terminal manager 直接清理 session 时同样触发取消，不依赖 HTTP DELETE 路径。
- 单选和文本问题经 Agent API 创建、浏览器回答后，`wait` 分别返回原始 `selectedOptionIds` 和 `feedback`。
- Interaction 在等待期间经历服务重启、决定后再次重启，客户端重试仍得到相同 `responseId` 和文本答案。
- 空 fixture 启动得到健康且为空的 Gateway，JSON 中 Checkpoint/Decision 数组保持为空。
- 注入创建 outbox 失败后，Interaction、audit、outbox 和 POST 幂等键均为零；移除故障后同 key 可成功创建。
- 在响应事务最后一个 projector outbox 处注入失败后，Interaction 仍为 pending/row version 1，Response、Decision、幂等键和 Task projection 均未遗留；重试只生成一个终态。
- expiry 和 session 批量取消的 outbox 失败会回滚状态、audit 和同批较早写入；移除故障后可完整收敛。
- 旧审批迁移 outbox 失败会连迁移标记表一同回滚；重试后只导入一次。
- 缺失 Task 导致的 projector 失败会持久化为 failed；补回 Task 后重试成功，后续重放不增加 row version。

## 阶段 1 退出条件审查

1. Adapter 创建请求后能在另一个浏览器批准并收到结果：已由 Agent → browser → `wait` API 闭环验证。
2. question request 能按 schema 接收选择或文本并原样映射回 Adapter：已验证单选和文本 Response 回传。
3. 服务在等待期间重启，客户端重试后仍得到同一个最终决定：已验证两次重启后的 `responseId` 与反馈不变。
4. 两个客户端同时决定时只有一个成功：仓储并发测试与 HTTP `409` 测试均通过。

## 后续阶段

阶段 2 从 Generic Adapter CLI 开始：实现 `roambench-agent request --wait --json` 的参数契约、结构化输出、超时与退出码。CLI 闭环稳定后，再把 OSC ingest 从浏览器 WebSocket 生命周期中拆出并接入 tmux 常驻 observer。
