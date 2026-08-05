# ADR-0001：手机控制面的持久化边界

日期：2026-08-04
状态：已接受
适用阶段：手机交互控制与多 CLI 协同计划阶段 1–6

## 决策

使用 SQLite 作为 Interaction、Checkpoint、Response、Decision、Outbox、Delivery 和幂等记录的唯一可写事实源。现有 JSON/WAL 状态文件继续保存 Project、Workstream、Task、Artifact、Tool Run 和其他 Project Control 数据，但不再保存或修改已经迁移的审批事实。

这是一条按实体所有权切分的迁移边界，不是双写：

```text
SQLite control-plane database
├── interactions
├── checkpoints
├── responses
├── decisions
├── audit_events
├── outbox_events
├── deliveries
└── idempotency_keys

JSON/WAL project state
├── projects / workstreams / tasks
├── artifacts / phase attempts / tool runs
├── task timeline / memory
└── agent token（本阶段保持现状）
```

Project Control snapshot 通过兼容层组合两边的读取结果。Checkpoint 和 Decision 一旦切换到 SQLite，JSON 中的对应数组不再参与读取或写入。

SQLite 驱动采用不依赖 CGO 的实现，以保持 Linux、Windows 和交叉编译路径。第一版使用单机文件数据库，不引入 Redis、NATS 或外部数据库。

## 背景事实

当前 `projectControlStore` 将单个用户的完整状态序列化到一个 JSON 文件。`withStateLocked` 在进程内互斥锁下执行“读取—修改—保存”，`saveLocked` 先写 `.wal.json`，再原子替换主文件。这能保护当前单进程、低频 Project Control 操作，但手机控制面会新增以下写入模式：

- checkpoint 创建必须与 outbox event 同时提交；
- 同一请求可能被手机和桌面并发响应；
- 重复 HTTP 请求需要持久化幂等结果；
- delivery worker 会频繁更新重试次数、下次投递时间和 dead-letter 状态；
- 长轮询、Adapter 回答和服务重启必须读取同一个最终决定；
- 后续需要按 pending、expiry、session、adapter 和 delivery state 查询。

继续扩展整份 JSON 状态会把投递重试变成全量文件重写，也无法用数据库约束直接表达唯一决定、外键、过期查询和事务 outbox。

## 事务边界

以下操作必须在一个 SQLite 事务中完成：

1. 创建 Interaction/Checkpoint；
2. 写入对应 audit event；
3. 写入 `interaction.created` outbox event。

响应请求时，一个事务必须完成：

1. 校验 request 仍为 `pending`；
2. 校验 `expected_row_version`、`input_hash`、session 状态和 expiry；
3. 占用或读取 `idempotency_key`；
4. 插入 Response 和 Decision；
5. 把 Interaction/Checkpoint 改为最终状态并递增 row version；
6. 插入 audit event；
7. 插入 Adapter delivery 和 UI update outbox event。

数据库约束至少包括：

- `(adapter_kind, session_id, vendor_request_id)` 唯一；
- 每个 request 最多一个成功的最终 Response；
- 非空 `checkpoint_id` 与 request 一对一；
- `(actor_scope, idempotency_key)` 唯一；
- delivery 引用存在的 request/outbox event；
- 所有外键启用。

SQLite 使用 WAL journal、`foreign_keys=ON`、有限 `busy_timeout` 和耐久同步。写事务使用明确的短事务，不在事务内调用 Push provider、CLI、tmux 或其他外部进程。

## JSON Task 投影

Checkpoint 决定有时会改变现有 Task 状态。SQLite 决定事实和 JSON Task 投影无法组成跨文件原子事务，因此采用可恢复投影：

1. SQLite 先提交 Decision 和 `project_task_projection.requested` outbox event；
2. projector 读取 event，在 `projectControlStore` 锁内按 `decision_id` 幂等更新 Task；
3. projector 把完成状态记录回 SQLite；
4. 服务重启后重新处理未完成投影。

移动端和 Adapter 以 SQLite Decision 为最终结果。Task snapshot 可以短暂滞后，但不能产生相反决定。UI 在投影未完成时显示“决定已记录，任务状态同步中”。

需要依赖 Task 证据的批准，在创建 Interaction 时保存 Task row version 和 `input_hash`。响应事务提交前重新读取并验证；发生变化则返回冲突，要求用户重新审阅。所有代码路径按固定顺序获取 Project Control 锁，再开启 SQLite 写事务，禁止反向锁序。

## 迁移步骤

阶段 1 的第一次迁移按以下顺序实施：

1. 在 `.project-control/` 下创建权限为 `0600` 的 control-plane SQLite 文件；
2. 执行带版本号的 schema migration；
3. 在一个 SQLite 事务中导入 JSON 里的 Checkpoint、Decision 和相关 audit event，保留原 ID；
4. 保存来源 JSON 的 hash、`updatedAt` 和迁移版本，重复运行时必须幂等；
5. 验证导入数量、引用和最终状态；
6. 保存只读的迁移前备份；
7. 原子重写 JSON 状态，移除已经迁移的 Checkpoint/Decision；
8. 启用 SQLite-backed compatibility repository；
9. 启动 outbox dispatcher 和 projector。

如果数据库提交成功但 JSON 清理前进程退出，下一次启动根据 migration record 和来源 hash 继续清理；不得再次生成新 ID。迁移完成后，旧二进制不能继续写这个状态目录。

## 回滚与恢复

- 在第一条 SQLite-only 新记录写入前，可以恢复迁移前 JSON 备份并删除未启用的数据库；
- 产生新记录后，运行时回滚必须使用仍理解 SQLite schema 的修复版本；
- 恢复到旧二进制只能使用迁移前备份，会丢失切换后的决定，因此不是正常回滚路径；
- 发布前必须验证数据库备份、完整性检查和从备份恢复；
- schema migration 只前进，不在应用启动时自动执行破坏性 downgrade。

## 被拒绝的方案

### 继续把所有控制面数据写入 JSON/WAL

它能保持最小代码改动，但 delivery retry、幂等索引、唯一决定和查询都会依赖手写扫描及全量文件重写。该方案不满足后续投递和并发模型的维护成本要求。

### SQLite 与 JSON 同时写 Checkpoint/Decision

两个文件没有共同事务，崩溃会产生相反状态。即使增加补偿逻辑，也无法回答哪一边是事实源，因此禁止。

### 阶段 1 前迁移全部 Project Control 数据

一次性迁移 Project、Task、Artifact、Tool Run、Memory 和审批会扩大回归面，推迟手机交互闭环。本 ADR 先迁移需要事务和查询能力的控制面实体，其余实体保留既有所有权。

### 立即引入外部 Broker

当前范围是单用户、自托管、单节点。SQLite outbox 已能提供持久化、重试和恢复；外部 Broker 会增加部署与故障面，留到多节点需求出现后评估。

## 阶段 1 实施门槛

- migration fixture 覆盖空状态、现有 pending checkpoint、已解决 checkpoint 和中断恢复；
- 并发响应测试证明最多一个最终决定；
- 决定事务失败时不产生 outbox 或部分 audit；
- projector 重复运行不会重复改变 Task；
- 兼容 snapshot 与现有 approvals inbox 的行为保持一致；
- 备份恢复演练可以重建 pending 和最终决定。
