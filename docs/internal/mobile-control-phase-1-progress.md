# 手机交互控制与多 CLI 协同：阶段 1 进展

日期：2026-08-05
状态：实施中
范围：Interaction/Decision Gateway 第一切片

## 当前结论

阶段 1 的新请求闭环已经建立，但旧审批事实迁移尚未完成。当前代码可以让 Adapter 创建结构化 Interaction，由另一个已登录浏览器作答，并让 Adapter 通过读取或最长 30 秒的有限长轮询取得最终结果。决定、审计和 outbox 在同一 SQLite 事务中提交；并发作答最多一个成功。

这不是阶段 1 完成声明。现有 JSON/WAL 中的历史 Checkpoint/Decision 仍待一次性迁移到 SQLite，Task 状态投影仍待实现为可恢复 projector。在这两项完成前，不进入阶段 2。

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
- `go test ./...` 通过。

## 尚未完成的阶段 1 门槛

1. 把已有 JSON/WAL Checkpoint、Decision 和相关 audit event 一次性导入 SQLite，记录源 hash、迁移版本和只读备份。
2. 数据库提交后原子清理 JSON 中已迁移的审批字段，处理中断恢复，彻底消除两个可写事实源。
3. 实现 `project_task_projection.requested` outbox consumer，按 `decision_id` 幂等更新 JSON Task，并可在重启后续跑。
4. 为 Interaction 创建和 cancel 增加完整的持久化 POST 幂等结果；当前创建依靠 vendor request 唯一键去重，cancel 依靠 row version 防止重复状态迁移。
5. 增加 expiry 和 session-ended 自动取消处理。
6. 完成空状态、pending、resolved、迁移中断和备份恢复 fixture。
7. 对 migration、projector 和事务故障执行 fault-injection，证明失败时没有部分 audit/outbox。

## 下一切片

下一切片只处理迁移和 projector：先用失败测试覆盖四类 migration fixture 与中断恢复，再实现导入、备份、JSON 清理和 Task 投影。完成后重新执行并发、重启、旧 approvals inbox 与完整回归测试，满足全部退出条件后才把阶段 1 标记为完成。
