# 手机交互控制与多 CLI 协同工作计划 v0.1

创建日期：2026-08-04
状态：阶段 0、阶段 1 已完成；下一步实施阶段 2 Generic Adapter CLI
适用范围：RoamBench 单用户、自托管部署

## 1. 目标

让服务器上的 Codex、Claude Code、OpenCode、Kimi CLI 和通用 terminal-first CLI 在无人盯着浏览器时继续工作，并在确实需要人类判断时完成以下闭环：

1. CLI 产生结构化权限请求、问题、计划审阅或人工检查点；
2. RoamBench 持久化交互请求，并通过 Push 唤醒手机；
3. 用户在手机控制端查看上下文，批准、拒绝、选择方案、填写反馈、控制 Session 或接管 Terminal；
4. RoamBench 只接受一次有效响应或决定，记录审计证据；
5. 对应 CLI 收到决定并继续运行；
6. 多个 CLI 通过统一任务和消息接口协作，不直接操纵彼此的终端。

完成后，用户不需要保持 RoamBench 网页在线，也不需要在手机终端里寻找正确 pane 再输入 `y/n`。Push 只负责唤醒；手机 PWA、Interaction/Decision Gateway 和 CLI Adapter 共同完成双向交互。

## 2. 范围与非目标

### 2.1 本计划包含

- 浏览器关闭后仍持续采集服务器端通知和审批事件；
- 持久化的交互请求、响应、投递记录、超时和去重；
- 可安装的移动交互 PWA、标准 Web Push 和实时事件通道；
- 权限审批、结构化问答、计划/Diff 审阅、文本反馈和高风险操作重新认证；
- Session 暂停、继续、停止、取消等待与 Terminal 手动接管；
- Codex、Claude Code、OpenCode、Kimi CLI 的独立适配器；
- 通用 `roambench-agent request --wait` 接口；
- CLI Mailbox，用于任务交接、审查请求和结果回传；
- tmux 输出监听和有限的 TUI 兼容兜底；
- 状态机、并发、安全、故障恢复和审计测试。

### 2.2 本计划不包含

- 多用户组织、RBAC、SSO 或团队审批流；
- 当前阶段单独开发和维护 iOS、Android 原生客户端；
- 让多个 CLI 自发组成无中心控制的 agent swarm；
- 通过聊天软件直接执行高风险批准；
- 默认开放 RoamBench 到公网；
- 把 MCP 当作持久化消息队列；
- 为无法提供结构化协议的任意 TUI 承诺可靠自动批准；
- 支付、OTP、CAPTCHA、UAC、凭证修改等无人值守操作。

## 3. 当前基础与已知缺口

### 3.1 可以复用的能力

当前代码已经具备：

- `Checkpoint`、`Decision` 和 approvals inbox；
- `approve`、`reject`、`reroute` 决策校验；
- 决策后关闭 checkpoint，并写入任务状态和事件记录；
- Cookie 认证的 checkpoint decision API；
- Agent Bearer Token；
- `roambench-agent status/artifact/checkpoint/notify`；
- OSC 9、99、777 流式解析器；
- terminal 和通知 WebSocket；
- 手机可用的响应式网页。

这些能力应继续作为实施基础，不建立第二套审批对象或第二个任务所有者。

### 3.2 必须先修复的缺口

1. **通知采集依赖 terminal WebSocket。** `OSCScanner` 当前在浏览器 attach 后的 PTY 读取循环里创建。浏览器关闭时，tmux 任务继续运行，但 RoamBench 不再持续解析其输出。
2. **通知只存在于内存。** `notificationHub` 只向在线订阅者广播；没有订阅者或客户端消费过慢时，消息会丢失。
3. **当前不是后台 Web Push。** 前端通过在线 WebSocket 接收消息，再调用页面级 `Notification` API。移动浏览器冻结或页面关闭后无法保证送达。
4. **Checkpoint 尚未映射回供应商审批请求。** 手机决定能更新 RoamBench 任务，但还不能回答等待中的 Codex、Claude Code、OpenCode 或 Kimi 请求。
5. **Agent API 只有上报，没有通用等待通道。** CLI 能请求 checkpoint，却不能可靠等待并获取决定。
6. **CLI 之间没有正式消息契约。** 任务交接目前只能依赖 prompt、共享文件或人工操作。

## 4. 设计决策

### D1. Checkpoint 是唯一审批事实源，Interaction 是统一交互外壳

所有供应商审批、通用 CLI 判断请求和系统升级都转换为 `Checkpoint`，再通过 `Interaction Request` 展示和收集响应。普通信息性问题和 Session 控制可以只有 Interaction，不强制创建 Checkpoint。Push、ntfy、WebSocket 和邮件只是投递渠道，不能各自保存一份可独立修改的审批状态。

### D2. RoamBench 是唯一控制器

RoamBench 继续拥有 task、session、checkpoint、decision、策略和审计 ID。CLI Adapter 只负责协议转换和执行，不拥有跨任务调度或持久记忆。

### D3. 原生结构化协议优先

接入优先级固定为：

1. 供应商双向 RPC/HTTP/ACP/Wire；
2. 能返回 allow/deny 的生命周期 hook；
3. `roambench-agent request --wait` wrapper；
4. OSC/tmux 输出识别；
5. 版本锁定且经过验证的 `tmux send-keys`。

输出文本匹配不能决定高风险批准。

### D4. 手机推送不携带授权能力

Push payload 只携带不透明事件 ID、摘要、风险级别和深链接，不携带可直接批准的长期 bearer token。真正决定必须回到 RoamBench，在当前认证会话中完成。

### D5. CLI 不直接互发 PTY 输入

CLI 之间通过持久化 Mailbox 和 Artifact 引用通信。只有 Adapter 可以调用供应商的入站控制协议；通用 TUI 的 `send-keys` 不作为协作协议。

### D6. 单机先用 SQLite，不提前引入外部 Broker

目标架构中的 inbox、outbox 和长轮询先使用 SQLite 实现。阶段 0 需先确认现有状态文件向 SQLite 的迁移边界；迁移完成前不允许同时维护两个可写事实源。出现多节点、高吞吐或独立 worker 后，再评估 NATS JetStream。数据库迁移保持标准 SQL，避免锁死后端。

### D7. 手机端是交互控制面，Push 只是唤醒通道

Push payload 只告诉用户“有新交互请求”，不能承载完整上下文或成为主要操作界面。手机 PWA 必须能读取当前请求、提交结构化响应、观察 Session 状态，并在必要时进入 Terminal。Push 丢失、延迟或重复时，交互请求仍保留在持久化 inbox。

### D8. 实时状态与副作用操作分离

WebSocket/SSE 负责推送 `interaction.created/updated/resolved`、Session 状态和输出摘要。批准、拒绝、回答问题、暂停、停止等产生副作用的操作使用认证 HTTP API，并强制执行 CSRF、row version、idempotency、风险策略和审计记录。

### D9. PWA 优先，原生 App 按证据启动

第一版手机端交付物是可安装 PWA，不同时开发独立 iOS、Android 客户端。Interaction API、认证、深链接和响应式界面必须保持客户端无关，保证以后可以接入原生客户端，但不能为尚未验证的需求提前维护第二套业务逻辑。

如果 PWA 真机数据证明系统推送动作、生物认证、安全凭据、多服务器配对或后台恢复能力成为主要瓶颈，再开发薄型 `RoamBench Companion`。Companion 复用同一 Interaction API；服务器仍是任务、权限和审计的唯一事实源。

## 5. 目标架构

```text
Codex app-server ───────────┐
Claude Permission hook ─────┤
OpenCode HTTP/SSE ──────────┤
Kimi Wire/ACP/hooks ────────┤
Generic CLI wrapper ────────┤
tmux/OSC fallback ──────────┘
              │
              ▼
       Runtime Adapter Layer
              │ canonical interaction/result
              ▼
       Interaction/Decision Gateway
         ┌────┼──────────────┐
         │    │              │
 Checkpoint  Mailbox       Outbox
 Decision    Artifact refs Delivery jobs
                               │
                     ┌─────────┼─────────┐
                     │         │         │
                  Web Push  WebSocket  ntfy 可选
                    唤醒      实时状态
                     │         │
                     └────┬────┘
                          ▼
                   手机交互 PWA
                          │ approve / answer / control / takeover
                          ▼
              Interaction/Decision Gateway
                          │
                          ▼
                 Adapter 回答原始 CLI
```

## 6. 核心数据模型

字段名在实现前需与现有 `projectControl*` 类型对齐。以下是最小契约，不要求一次完成所有可选字段。

### 6.1 Interaction Request

```text
request_id
checkpoint_id?            普通问答或 Session 控制可为空
task_id
runtime_id
session_id
adapter_kind
vendor_request_id
request_kind
risk_class
title
summary
preview
artifact_refs[]
allowed_actions[]
response_schema
ui_hints
status                 pending | resolved | expired | cancelled
created_at
expires_at
row_version
input_hash
```

约束：

- `vendor_request_id` 在同一 adapter/session 内唯一；
- 非空 `checkpoint_id` 唯一绑定一个待处理 request；
- `status != pending` 时禁止再次响应；
- `preview` 必须经过长度限制和秘密信息清理；
- `response_schema` 定义允许的动作、选择项、文本长度和是否允许自定义输入；
- 手机端只能根据服务端 schema 渲染和提交，不得自行扩大动作集合；
- 原始长命令、Diff 或日志使用 Artifact 引用保存。

`request_kind` 第一版支持：

```text
permission              批准一次、批准本 Session、拒绝、拒绝并反馈
question_single         单选题
question_multiple       多选题
question_text           文本回答
plan_review             批准计划、要求修改、拒绝
diff_review             批准变更、要求修改、拒绝
session_control         pause | resume | stop_turn | terminate_session
manual_takeover         打开绑定 Terminal，由用户接管
```

### 6.2 Interaction Response

```text
response_id
request_id
checkpoint_id?            与 request 一致，可为空
action
selected_option_ids[]
feedback
actor
device_id
auth_evidence
expected_row_version
idempotency_key
input_hash
created_at
```

`feedback`、选择项和 action 必须通过对应 `response_schema` 校验。`approve_session` 还必须保存实际新增的规则、scope 和 expiry，不能只存一个布尔值。

### 6.3 Decision Delivery

```text
delivery_id
request_id
channel                web_push | websocket | ntfy
device_id
state                  queued | sent | acknowledged | failed | dead_letter
attempt_count
next_attempt_at
last_error
created_at
updated_at
```

### 6.4 Push Subscription

```text
subscription_id
username
device_id
endpoint
p256dh
auth
user_agent_summary
created_at
last_seen_at
revoked_at
```

Push endpoint 和密钥按凭证处理，不写普通应用日志。

### 6.5 Agent Message

```text
message_id
task_id
from_session_id
to_selector             session:<id> | role:<name> | task:<id>
kind                    task_offer | review_request | result | question | reply
reply_to
summary
artifact_refs[]
state                   queued | claimed | acknowledged | expired
deadline
idempotency_key
created_at
row_version
```

Mailbox 只传控制信息。代码、Diff、测试输出、截图和文档存为 Artifact，以 hash/URI 引用。

### 6.6 Outbox Event

Checkpoint 创建、状态更新和 outbox event 必须在同一数据库事务中提交。独立 dispatcher 根据 outbox 发送通知，成功后记录 delivery；失败不回滚已经存在的 checkpoint。

## 7. API 计划

路径可在实现时调整，但语义和认证边界必须保持。

### 7.1 Adapter/Agent API

```text
POST /api/agent/v1/interactions
GET  /api/agent/v1/interactions/:id
GET  /api/agent/v1/interactions/:id/wait?timeout=30s
POST /api/agent/v1/interactions/:id/cancel

POST /api/agent/v1/messages
GET  /api/agent/v1/inbox?sessionId=...&after=...
POST /api/agent/v1/messages/:id/claim
POST /api/agent/v1/messages/:id/ack
```

要求：

- Bearer Token 绑定 username、runtime/session scope 和能力；
- `wait` 使用有限长轮询，服务端重启后客户端可重试；
- 所有 POST 支持 `Idempotency-Key`；
- Adapter 不能调用人类决策接口伪装为用户。

### 7.2 手机/Web API

```text
POST   /api/push/subscriptions
DELETE /api/push/subscriptions/:id
POST   /api/push/test

GET  /api/mobile/interactions
GET  /api/mobile/interactions/:id
POST /api/mobile/interactions/:id/respond

POST /api/mobile/sessions/:id/pause
POST /api/mobile/sessions/:id/resume
POST /api/mobile/sessions/:id/stop-turn
POST /api/mobile/sessions/:id/terminate
```

决策请求体至少包含：

```json
{
  "action": "reject_with_feedback",
  "selectedOptionIds": [],
  "feedback": "先运行测试并提供结果",
  "expectedRowVersion": 4,
  "idempotencyKey": "device-generated-uuid"
}
```

第二个设备或页面对已解决请求作答时返回 `409 Conflict`，并返回当前最终状态。

Session 控制 API 必须复用 Runtime/Policy Gateway。`terminate` 与 `stop-turn` 是不同能力；终止整个 Session 需要更高风险确认。

### 7.3 实时事件

新增 `/api/mobile-control/ws`，推送：

```text
interaction.created
interaction.updated
interaction.resolved
session.state_changed
session.output_summary
task.updated
```

事件源改为持久化 event/outbox，而不是临时 OSC 广播。WebSocket 重连携带 cursor，补取断线期间事件。WebSocket 只传状态，不直接执行批准或 Session 控制。

## 8. CLI Adapter 计划

### 8.1 Codex Adapter

首选 `codex app-server`：

- 使用 stdio 或本机 Unix socket；
- 初始化 thread/turn 映射；
- 接收服务端发起的 command/file approval JSON-RPC；
- 转换成 Interaction Request，并为需要人类裁决的请求绑定 Checkpoint；
- 等待 RoamBench Response/Decision；
- 返回供应商支持的 `accept`、`acceptForSession`、`decline` 或 `cancel`；
- turn/session 结束时取消残留请求。

`acceptForSession` 视为更高权限动作，必须展示具体规则范围，不能由普通 `approve` 隐式产生。

### 8.2 OpenCode Adapter

首选本机 `opencode serve`：

- 仅监听 loopback 或 Unix socket 等价边界；
- 订阅 SSE event stream；
- 接收 permission/question 事件；
- 通过官方 permission response endpoint 回答；
- 不把 OpenCode Server 直接暴露到公网；
- 为 Server 配置独立随机密码，并由 Adapter 保管。

### 8.3 Claude Code Adapter

使用 `PermissionRequest` hook：

- hook 从 stdin 读取供应商 JSON；
- 调用本机 `roambench-agent request --wait`；
- 手机批准后输出 Claude Code 需要的 allow/deny JSON；
- deny 可携带用户反馈；
- hook 设置明确超时，超时默认不批准；
- CLI/session 退出时取消等待请求。

普通 `Notification(permission_prompt)` 只用于提醒，不能代替能返回决定的 Permission hook。

### 8.4 Kimi Adapter

按以下优先级实施：

1. Wire/ACP 中的 ApprovalRequest/ApprovalResponse；
2. 能返回 permission decision 的 Permission/PreToolUse hook；
3. Notification hook 只做提醒；
4. 版本锁定的交互式 TUI 兜底。

Kimi hooks 当前仍可能发生协议变更，Adapter 必须做版本探测和能力协商。

### 8.5 Generic Adapter

新增：

```text
roambench-agent request \
  --kind permission \
  --title "Deploy service" \
  --summary "Restart production worker" \
  --risk R3 \
  --preview-file ./approval.json \
  --wait \
  --timeout 30m
```

输出模式：

- 人类使用：短文本和退出码；
- 程序使用：`--json` 输出稳定 schema；
- approve 返回 0；
- reject 返回约定的非零码；
- expired/cancelled/transport error 使用不同退出码。

### 8.6 tmux/OSC Fallback

OSC 和 tmux 只承担两类工作：

- 通知任务完成、失败、卡住或需要注意；
- 为暂时没有 Adapter 的 CLI 发现可能的等待状态。

实现候选：

- 为每个 pane 配置 `tmux pipe-pane -O`，输出进入常驻 ingest；或
- 使用单个 tmux control-mode observer 订阅 pane output。

实施前必须用原型验证：

- 浏览器完全关闭时仍能收到输出；
- 不影响现有 attach、scrollback、颜色和 pane 大小；
- 不重复消费同一 OSC；
- RoamBench 重启后能重新绑定已有 tmux sessions；
- pane 创建、销毁和 session 恢复时 observer 自动同步。

`tmux send-keys` 仅在满足以下条件时允许启用：CLI 类型和版本已识别、pane ID 精确绑定、当前屏幕状态 hash 与请求一致、请求仍为 pending、按键后能验证请求已经解决。否则只通知用户手动接管。

## 9. 手机交互控制面与推送

### 9.1 移动信息架构

```text
待处理
├── 权限审批
├── 问题
├── 计划审阅
└── Diff 审阅

运行中
├── Task/Agent 状态
├── 最新输出摘要
├── pause/resume/stop-turn
└── terminate session

Terminal
└── 绑定现有 tmux pane，手动接管

设置
├── 推送设备
├── 通知级别与静默时段
└── 安全与重新认证
```

手机首页以“待处理交互”为主，不以 terminal 标签为主。每条交互卡片至少显示来源 CLI、Task、真实副作用、风险、等待时间和过期时间。

### 9.2 交互能力

手机端必须支持：

- `approve_once`：仅回答当前供应商请求；
- `approve_session`：批准服务端明确展示的规则范围和有效期；
- `reject`；
- `reject_with_feedback`：填写拒绝原因并返回 CLI；
- 单选、多选和文本回答；
- 计划/Diff 的 approve、revise、reject；
- `pause`、`resume`、`stop_turn`、`cancel_request`；
- `terminate_session`，二次确认并展示后果；
- `manual_takeover`，打开绑定的 terminal session。

长命令、Diff、日志和测试结果按需加载。Push 摘要不能代替完整审阅内容。

### 9.3 PWA 基础

- 添加 Web App Manifest、稳定 `id`、standalone display 和图标；
- 添加 Service Worker；
- 提供“安装到手机”和“启用通知”的明确引导；
- 设置设备名称，允许用户撤销单个设备；
- 显示最后推送成功时间和测试通知。

### 9.4 Web Push

- 服务端生成并管理 VAPID 密钥；
- 保存浏览器 `PushSubscription`；
- outbox dispatcher 发送标准 Web Push；
- notification `tag` 使用 request ID，防止同一请求重复堆叠；
- 通知点击进入 `/mobile/interactions/<request-id>`；
- Service Worker 只负责展示和导航，不在后台静默批准。

### 9.5 通知按钮的兼容边界

Web Notification actions 和 Service Worker `notificationclick` 在不同浏览器上的支持不一致，不能成为核心交互契约。第一版只保证点击通知能打开对应交互页面。

- “查看”可以作为通知动作；
- “拒绝”可作为后续增强，但仍要经过服务端状态和认证校验；
- “批准”不在锁屏通知中直接执行；
- R2/R3、计划审阅、Diff 审阅、文本回答必须进入 PWA；
- 若未来原生 App 提供系统通知动作，也必须调用同一 Interaction API，不能绕过 Gateway。

### 9.6 交互详情页

首屏必须展示：

- 来源 CLI、服务器/runtime、工作目录和 task；
- 请求类型、风险等级和过期时间；
- 完整命令或文件操作目标；
- 供应商提供的 reason；
- Diff、日志或测试证据入口；
- “仅本次批准”和“本 Session 批准”的明确区别；
- 服务端允许的选择项或文本输入约束；
- approve、reject、revise、反馈或手动接管操作；
- Session 的实时状态，防止对已经结束的请求作答。

高风险请求不允许只看截断的通知摘要就批准。

### 9.7 Terminal 接管

结构化交互无法覆盖复杂场景时，用户从详情页点击“手动接管”：

1. RoamBench 将 Interaction 标记为 `manual_takeover_requested`，但不自动批准或拒绝；
2. 打开绑定的 tmux session/pane；
3. 用户查看完整上下文并输入复杂回复；
4. Adapter 观察供应商请求是否已经解决；
5. 已解决后关闭 Interaction，并记录接管方式和结果。

普通权限审批不应强迫用户进入 Terminal。

### 9.8 网络部署

默认建议：

- RoamBench 保持 loopback/私网监听；
- 手机和服务器加入同一 Tailscale；
- 使用私有 HTTPS 入口；
- Push 由服务器主动连接浏览器 push endpoint，不开放新的入站 webhook；
- 不默认使用公网 Funnel。

Tailscale 断开时，系统仍可展示已送达的 Push 摘要，但交互详情页必须提示网络不可达，不能缓存可离线提交的批准动作。

### 9.9 ntfy 过渡方案

为了尽快验证通知价值，可以先增加可选 ntfy sink：

- 只发送摘要和 RoamBench 深链接；
- ntfy 不持有交互或决策状态；
- ntfy action 不直接调用 respond endpoint；
- 文档明确说明自托管 iOS 即时通知的上游依赖。

Web Push 完成后，ntfy 保留为可选渠道，不成为主路径。

### 9.10 原生手机 App 决策

#### 当前结论

当前不单独立项开发完整原生 App。阶段 3 的 PWA 目标覆盖主要闭环：接收唤醒通知、查看完整上下文、批准或拒绝、附加反馈、回答结构化问题、审阅计划或 Diff、控制 Session 和进入 Terminal。

iOS/iPadOS 16.4 及以上的主屏幕 Web App 支持标准 Web Push，且启用 Web Push 本身不要求加入 Apple Developer Program。第一版应先用真机数据验证这条路径，而不是把原生 App 当作推送功能的前置条件。

原生 App 也不能替代服务端必须完成的 Decision Gateway、Adapter、状态持久化、幂等、安全策略和审计。提前开发 App 会增加签名、商店审核、发布升级和客户端兼容成本，但不会缩短服务端交互闭环的关键路径。

#### 能力边界

| 需求 | PWA 第一版 | 原生 Companion 的额外价值 |
|---|---|---|
| Push 唤醒并打开请求详情 | 支持 | 更完整的 APNs/FCM 生命周期和诊断 |
| approve/reject/feedback/question | 支持 | 可增加系统级通知动作 |
| 计划、Diff 和日志审阅 | 支持 | 价值有限，仍依赖服务端内容 |
| Session 控制和 Terminal 接管 | 支持 | 可改善深链接和应用恢复体验 |
| 锁屏直接批准或拒绝 | 不作为可靠承诺 | 原生通知动作更可控，但仍须经过 Gateway |
| 生物认证和安全凭据 | 可使用 WebAuthn/passkey | 可集成 Face ID、指纹、Keychain/Keystore |
| 多服务器和设备配对 | 可以实现 | 二维码配对和凭据隔离体验更好 |
| 应用市场分发 | 不适用 | 适合面向非技术用户安装和升级 |

高风险请求即使使用原生通知按钮，也不能仅凭通知 payload 完成批准。App 必须打开受保护的确认界面或完成符合风险策略的近期认证，再调用同一 `respond` API。

#### 启动门槛

阶段 3 完成一轮实际使用后评审。出现以下至少两类持续问题，并确认无法通过 PWA、服务端或部署配置合理解决时，才启动 Companion App：

1. 业务要求稳定地从系统通知执行动作，而“点击后进入 PWA”无法接受；
2. R2/R3 审批要求原生生物认证或硬件保护凭据；
3. PWA 的推送到达、点击打开、后台恢复或登录保持达不到发布目标；
4. 用户需要管理多个 RoamBench 服务器，并通过二维码完成可信配对；
5. 需要相机、系统分享、快捷指令、桌面组件等明确的系统能力；
6. 产品需要通过 App Store 或 Android 应用市场面向非技术用户分发。

不能只以“原生体验可能更好”启动项目。评审材料至少包含真机问题记录、指标、目标用户场景、PWA 修复成本和 App 全生命周期成本。

#### Companion App 边界

如果达到启动门槛，App 定位为薄型 `RoamBench Companion`，而不是新的控制器或另一套后端：

```text
RoamBench Companion
├── APNs / FCM 注册与通知深链接
├── Face ID / 指纹重新认证
├── Keychain / Keystore 凭据保存
├── 二维码配对与多服务器配置
├── 复用移动交互界面
└── 调用同一 Interaction / Session API
```

- Checkpoint、Interaction、Decision 和 Session 状态只保存在服务器；
- App 不复制风险判定、授权规则、状态机或 Adapter 逻辑；
- Push 仍只负责唤醒，不能成为授权事实源；
- 优先评估 Capacitor 等 Web-first 容器，复用现有移动界面，只为原生能力增加窄插件；
- Android Trusted Web Activity 只能作为 Android 包装选项，不能解决 iOS，也不能代替需要原生访问的安全能力；
- 若计划提交 App Store，不能只把网站包进 WebView。版本必须具备生物认证、安全配对、原生通知管理或其他明确的原生价值，以满足 Apple 对最低功能完整性的要求。

Companion App 不改变本计划阶段 0–6 的优先级；其可行性评审和实施属于阶段 3 完成后的独立决策。

## 10. 安全不变量

以下条件是合并和发布门槛：

1. 只有 Interaction/Decision Gateway 能把人类响应或决定映射回 CLI；
2. Push、WebSocket、ntfy 和 Adapter 都不能直接修改 checkpoint 数据文件；
3. 同一请求最多成功决定一次；
4. 已过期、取消或 session 已结束的请求不能批准；
5. 高风险批准必须校验最新 preview/input hash；
6. 决策状态迁移与审计 event 在同一事务；
7. Push payload、日志和通知标题不得泄露 token、cookie、密钥或完整敏感参数；
8. Adapter token 按 session/capability 最小授权；
9. `approve for session` 必须记录实际新增的规则范围和到期时间；
10. Adapter 或推送失败时默认保持等待或拒绝，不能 fail-open 执行高风险动作；
11. 手机 approve 对 R2/R3 请求要求近期认证，高于边界的操作要求 passkey/重新登录；
12. R4 操作交给用户在原系统完成，R5 拒绝；
13. 手机端只渲染服务端 `response_schema` 允许的操作，不能凭前端状态生成额外授权；
14. WebSocket 事件不能触发副作用，所有交互响应和 Session 控制必须走认证 HTTP API；
15. Terminal 手动接管不能把 pending request 自动标记为 approved，必须观察并记录真实结果。

## 11. 分阶段实施

估算以一名熟悉 Go 和 Web 前端的工程师为基准，不包含供应商协议发生重大变更的缓冲。每个阶段都应独立合并，禁止一次性大改。

### 阶段 0：基线与协议探针（已完成，2026-08-04）

目标：冻结现状，验证四种 CLI 的真实版本和事件能力。

任务：

- 为当前“浏览器关闭后 OSC 不送达”增加失败测试或可复现实验；
- 记录 Codex、Claude Code、OpenCode、Kimi 的最低支持版本；
- 为每种 CLI 保存脱敏的审批请求/响应 fixture；
- 比较 `pipe-pane` 与 control mode observer；
- 确认当前 project control 持久化后端和迁移策略。

退出条件：

- [x] 四种 CLI 至少各有一个可重放协议 fixture；
- [x] tmux 常驻 observer 方案完成书面选择；
- [x] 不依赖浏览器在线的失败用例稳定复现。

阶段 0 交付记录见 [阶段 0 基线](./mobile-control-phase-0-baseline.md) 和 [ADR-0001](./adr-0001-mobile-control-persistence.md)。已选择每个 tmux session 一个 server-owned control-mode observer，`pipe-pane -O` 作为降级；SQLite 唯一拥有 Interaction/Checkpoint/Decision/Outbox 等控制面事实。Kimi CLI 本机尚未安装，当前 fixture 来自官方 Wire 文档；这不阻止阶段 1，但在阶段 4 发布 Kimi Adapter 前必须补真实运行 fixture。

### 阶段 1：持久化 Interaction/Decision Gateway（已完成，2026-08-05）

目标：先建立可靠交互闭环，不接手机推送。

任务：

- 增加 Interaction Request/Response、Delivery、Outbox schema；
- 定义 permission、question、plan/diff review、session control 的 response schema；
- 将 agent checkpoint 请求扩展为结构化 interaction；
- checkpoint 创建与 outbox 写入保持原子性；
- 决策 API 增加 row version、idempotency 和 409；
- 增加 wait/cancel API；
- 服务重启后恢复 pending requests；
- 保留现有 approvals inbox 行为。

退出条件：

- Adapter 创建请求后能在另一个浏览器批准并收到结果；
- question request 能按 schema 接收选择或文本，并原样映射回 Adapter；
- 服务在等待期间重启，客户端重试后仍得到同一个最终决定；
- 两个客户端同时决定时只有一个成功。

阶段 1 交付与退出条件证据见 [阶段 1 进展](./mobile-control-phase-1-progress.md)。SQLite Gateway、结构化 Agent/Web API、完整 Response 回传、创建/响应/cancel 的持久化幂等、expiry/session 自动终止、旧审批迁移、可恢复 Task projector，以及运行时旧 Project Control Checkpoint 的 SQLite 单一事实源已经交付。空状态 fixture 和创建、响应、expiry、session cancel、迁移、projector 故障恢复矩阵通过，阶段 2 可以开始。

### 阶段 2：Generic Adapter 与 tmux 常驻监听（2–4 天）

目标：任何脚本都能请求手机前置的人类决定；浏览器关闭后仍能采集通知。

任务：

- 实现 `roambench-agent request --wait --json`；
- 定义退出码和超时行为；
- 将 OSC ingest 从 terminal WebSocket 生命周期中拆出；
- 为已有和新建 tmux pane 自动维护 observer；
- OSC event 进入持久化 event/outbox；
- 防止 observer 与浏览器 attach 重复通知。

退出条件：

- 页面关闭、手机锁屏、RoamBench 前端无连接时，OSC 通知仍写入 inbox；
- generic request 能 approve、reject、expire、cancel；
- tmux session 重连和 RoamBench 重启不会丢失 observer。

### 阶段 3：移动交互 PWA 与 Web Push（4–7 天）

目标：手机在网页关闭时收到通知，并完成审批、问答、审阅、Session 控制和 Terminal 接管。

任务：

- Manifest、Service Worker、订阅管理；
- VAPID 配置与密钥加载；
- outbox dispatcher、重试和 dead-letter；
- 推送测试页和设备撤销；
- 待处理 inbox、交互详情、结构化选择和文本反馈；
- 计划/Diff 审阅页面；
- 运行中 Session 控制页和输出摘要；
- `/api/mobile-control/ws` 与 cursor 补取；
- Terminal 手动接管入口；
- notification deep link、tag 和过期处理；
- 高风险请求重新认证；
- 记录推送发送、客户端接收确认、通知点击、详情打开、重新登录和响应完成漏斗；
- 建立原生 Companion 启动门槛评审所需的真机问题记录。

退出条件：

- Android 和 iOS 主屏幕 Web App 均完成真机测试；
- 页面和浏览器关闭时能收到通知；
- 手机能完成 approve once、reject with feedback、单选、文本回答和 plan revise；
- pause/resume/stop-turn 能映射到正确 Session，terminate 有二次确认；
- Terminal 接管不会隐式批准 pending request；
- 点击过期通知不会产生决定；
- 通知内容不包含敏感 payload；
- 能区分“Push 未发送、已发送但客户端未确认、已打开未响应”和“认证失败”等问题。

### 阶段 4：供应商 Adapter（5–8 天）

优先顺序：Codex → OpenCode → Claude Code → Kimi。

每个 Adapter 单独交付，必须完成：

- capability/version 探测；
- permission/question/review 的 request/response 映射；
- session 生命周期和取消；
- approve/reject/question/feedback/timeout fixture 测试；
- 手机端来源和 preview 展示；
- 最小权限配置文档；
- 升级兼容失败时退回“通知 + 手动接管”，不能静默批准。

退出条件：

- 四种 CLI 均完成一次真实的手机 approve 和 reject，并验证各自支持的结构化问答能力；
- 至少 Codex/OpenCode 能通过原生 RPC 完成，不使用 `send-keys`；
- hook 类 Adapter 超时不会自动执行请求。

### 阶段 5：CLI Mailbox（3–5 天）

目标：不同 CLI 围绕 Task 交换结构化工作，不共享隐式对话状态。

任务：

- Agent Message schema 与 API；
- claim、ack、reply、expire；
- Artifact 引用和 hash 校验；
- 为支持入站控制的 Adapter 注入下一轮任务；
- 为纯 TUI 提供 pull 命令，不自动输入 prompt；
- approvals inbox 展示消息来源链路。

退出条件：

- Codex 能向 reviewer role 发 review request；
- 另一个 CLI claim 后返回带 artifact 的 result；
- 原 CLI 重启后仍能恢复未确认消息；
- 同一消息不会被两个独占消费者同时 claim。

### 阶段 6：加固、观测与发布（3–5 天）

任务：

- 故障注入、并发和安全测试；
- 推送速率限制、去重、静默时段和聚合；
- Adapter 健康状态、积压、投递延迟和失败率指标；
- 升级/回滚文档；
- 数据迁移和备份恢复演练；
- 更新 README、配置示例、部署加固和 Agent Integration 文档；
- Alpha feature flag 和逐步启用。

退出条件：满足第 14 节发布门槛。

### 可选阶段 7：RoamBench Companion

本阶段默认不排期。只有第 9.10 节启动门槛通过书面评审后，才拆分独立计划、工期和发布门槛。第一版仅实现已经由证据确认的原生能力，不重写 PWA 已稳定运行的任务、审批或 Terminal 业务逻辑。

## 12. 预计代码影响

具体文件在实现时可拆分，优先新增窄模块，避免继续扩大 `project_control.go` 和 `app.js`。

```text
cmd/roambench-agent/
  main.go                       # request/wait/inbox 命令

internal/
  decision/
    model.go
    store.go
    service.go
    outbox.go
  interaction/
    schema.go
    validator.go
    service.go
  adapters/
    adapter.go
    codex/
    claude/
    opencode/
    kimi/
    generic/
  messaging/
    model.go
    store.go
    service.go
  push/
    service.go
    webpush.go
    ntfy.go
  terminal/
    observer.go
    osc.go
  server/
    decision_handlers.go
    interaction_handlers.go
    mobile_control_ws.go
    messaging_handlers.go
    push_handlers.go

web/
  manifest.webmanifest
  sw.js
  js/approvals.js
  js/mobile-control.js
  js/interaction-renderer.js
  js/push.js
```

需要修改的现有接缝：

- `internal/server/server.go`：路由、生命周期和旧 notification hub 迁移；
- `internal/server/project_control.go`：Checkpoint/Decision 适配调用；
- `internal/terminal/manager.go`：tmux observer 生命周期；
- `cmd/roambench-agent/main.go`：新命令；
- `web/index.html`、`web/js/app.js`：PWA 注册和 approvals 导航；
- `internal/config/config.go`：push、adapter、feature flag 配置。

## 13. 测试矩阵

### 13.1 状态与并发

- pending → approved/rejected/rerouted/expired/cancelled 合法迁移；
- 非 pending 请求拒绝再次决定；
- 手机和桌面同时 approve/reject；
- 相同 idempotency key 重试；
- 不同 idempotency key 重复决定；
- preview/input hash 改变后旧批准失效；
- session 结束自动取消未决请求；
- `response_schema` 拒绝未声明 action、非法选项和超长文本；
- approve once 与 approve session 的权限效果严格区分；
- Terminal 接管后由供应商真实状态关闭请求。

### 13.2 故障恢复

- RoamBench 在请求等待时重启；
- Adapter 在请求等待时重启；
- tmux session 保持运行但浏览器全部关闭；
- Push provider 超时、429、5xx；
- 手机离线后重新上线；
- outbox job 重复执行；
- SQLite busy/transaction rollback；
- 旧版本 CLI 协议不兼容。

### 13.3 安全

- 未认证订阅、决策和设备撤销；
- CSRF、跨 Origin WebSocket 和重放；
- Adapter token 越权访问其他 session；
- Push payload 与日志秘密扫描；
- notification deep link 篡改；
- `approve for session` 规则扩大；
- ANSI/OSC 注入、超长 preview、恶意标题；
- tmux pane/session ID 混淆；
- 已变化屏幕上的 `send-keys` 被拒绝。

### 13.4 真机与兼容性

- Android Chrome PWA；
- iOS Safari 主屏幕 Web App；
- 手机处于锁屏、低电量和 Focus 模式；
- Tailscale 在线/离线切换；
- 手机和桌面同时保持 RoamBench 会话；
- 权限、单选、多选、文本、计划/Diff 审阅和 Session 控制；
- notification action 不可用时仍能通过点击正文进入交互页；
- Codex、Claude Code、OpenCode、Kimi 最低支持版本及当前版本。

## 14. 发布门槛

满足以下条件才能从 feature flag 后转为默认可用：

- 100% 的 checkpoint 决策都有 actor、时间、输入 hash、最终结果和 audit ref；
- 并发决定测试证明最多一个成功；
- 页面关闭和服务重启场景没有审批请求丢失；
- Push 投递失败不影响 inbox 可见性；
- 四个正式 Adapter 都有协议 fixture 和失败路径测试；
- 高风险请求没有通知内直接批准路径；
- 秘密扫描未发现 push/log 泄露；
- Android/iOS 各完成一轮 approve、reject、expire 真机演练；
- Android/iOS 各完成 question、reject with feedback、plan revise 和 Session pause/resume；
- 数据迁移可回滚或有恢复备份；
- 停止按钮能取消 Adapter 等待并阻止排队副作用；
- 部署文档明确 HTTPS、Tailscale、VAPID 和设备撤销步骤。

## 15. 观测指标

第一版至少记录：

- `interaction_requests_pending`；
- `decision_time_to_human_seconds`；
- `decision_expired_total`；
- `interaction_pending{kind}`；
- `interaction_response_latency_seconds{kind,action}`；
- `delivery_attempts_total{channel,result}`；
- `delivery_latency_seconds{channel}`；
- `mobile_interaction_open_total{source}`；
- `mobile_push_to_open_seconds{platform}`；
- `mobile_auth_failures_total{platform,reason}`；
- `mobile_response_completion_total{platform,kind,result}`；
- `adapter_requests_total{kind,result}`；
- `adapter_protocol_errors_total{adapter}`；
- `mailbox_depth{selector}`；
- `outbox_oldest_age_seconds`；
- `tmux_observer_active_panes`。

日志使用 request/checkpoint/session ID 关联，禁止记录 PushSubscription 密钥、Cookie、Bearer Token 或未清理的完整命令参数。

## 16. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| CLI 协议频繁变化 | Adapter 中断 | 版本探测、fixture、feature flag、降级为手动接管 |
| tmux 输出重复或丢失 | 重复通知/漏提醒 | 常驻单 observer、event ID 去重、重启恢复测试 |
| 手机通知误触 | 错误批准 | 通知只打开详情页，高风险重新认证 |
| 浏览器不支持自定义通知按钮 | 无法在锁屏直接操作 | 正文点击进入 PWA；按钮不进入核心验收标准 |
| PWA 推送或后台恢复在目标设备不可靠 | 人工响应延迟或遗漏 | 记录平台漏斗与真机问题；先修复部署/认证，达到启动门槛后再评估 Companion |
| 过早开发原生 App | 延长关键闭环、增加双端维护 | PWA 优先；原生项目必须通过第 9.10 节证据门槛 |
| 纯 WebView App 不满足应用商店要求 | 审核失败或重复返工 | 上架版本必须提供明确的原生安全、配对或通知能力 |
| Session 控制对象选错 | 中断错误任务 | request/session 强绑定、展示 runtime/cwd、row version 和二次确认 |
| 推送平台不可用 | 无即时提醒 | Inbox 持久存在、WebSocket/ntfy 可选、重试与 dead-letter |
| 多端竞态 | 相反决定同时提交 | row version、事务、409、idempotency |
| hook 长时间阻塞 | CLI 卡住 | 明确超时、取消、健康检查、可见等待状态 |
| 敏感内容进入第三方 Push | 数据泄露 | payload 最小化、服务器端详情、日志清理 |
| Mailbox 演变成第二个 Agent | 控制权分裂 | Mailbox 只存消息，Task/Decision 仍由 RoamBench 拥有 |

## 17. 待确认决策

以下问题在阶段 0 结束前确认：

1. ~~当前持久化层继续演进还是先迁移到 SQLite migrations；~~ 已由 ADR-0001 决定：审批控制面迁移到 SQLite；
2. ~~tmux 采用 `pipe-pane` 还是单 control-mode observer；~~ 已决定：每个 tmux session 一个 control-mode observer，`pipe-pane -O` 降级；
3. Web Push VAPID 私钥的部署存储方式；
4. R2/R3 重新认证使用密码、WebAuthn/passkey，还是两者并存；
5. 第一批正式支持的 CLI 最低版本；
6. ntfy 是否作为 Alpha 默认可选 sink；
7. Mailbox 的第一版是否只支持单消费者 claim；
8. `approve for session` 是否进入第一版，或先只支持 approve once；
9. 第一版允许的文本反馈长度、附件类型和敏感信息清理规则；
10. Session `pause` 是协议级暂停还是进程级信号，各 Adapter 分别支持到什么程度。

以下问题不在阶段 0 决定。阶段 3 真机使用后，根据第 9.10 节评审：

11. 是否达到开发 `RoamBench Companion` 的启动门槛；
12. 如果达到门槛，优先支持 iOS、Android 还是同时支持；
13. 是否采用 Capacitor，或只在一个平台实现最小原生客户端。

## 18. 推荐最小交付路径

如果资源有限，按以下顺序交付可形成最短闭环：

1. 阶段 1：持久化 Interaction/Decision Gateway；
2. 阶段 2：`roambench-agent request --wait` 和浏览器关闭后的 tmux 监听；
3. 阶段 3：手机交互 PWA + Web Push + Session 控制；
4. 阶段 4：先交付 Codex 和 OpenCode Adapter；
5. Claude/Kimi Adapter 和 CLI Mailbox 后续增量加入。

前三步完成后，通用脚本和手工 wrapper 已能可靠使用手机完成批准、拒绝、反馈和结构化问答；不必等待所有供应商 Adapter 同时完成。

## 19. 研究依据

- tmux `pipe-pane` 与 control mode：<https://man.openbsd.org/OpenBSD-current/man1/tmux.1>
- Codex App Server：<https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md>
- Claude Code Hooks：<https://code.claude.com/docs/en/hooks>
- OpenCode Server：<https://dev.opencode.ai/docs/server/>
- Kimi Hooks：<https://moonshotai.github.io/kimi-cli/en/customization/hooks.html>
- Web Push on iOS/iPadOS：<https://webkit.org/blog/13878/web-push-for-web-apps-on-ios-and-ipados/>
- Apple Web Push：<https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers>
- Apple App Review Guidelines：<https://developer.apple.com/app-store/review/guidelines/>
- Capacitor：<https://capacitorjs.com/docs>
- Android Trusted Web Activity：<https://developer.android.com/develop/ui/views/layout/webapps/trusted-web-activities>
- Notification click/actions compatibility：<https://developer.mozilla.org/en-US/docs/Web/API/ServiceWorkerGlobalScope/notificationclick_event>
- ntfy：<https://docs.ntfy.sh/>
- MCP Architecture：<https://modelcontextprotocol.io/docs/learn/architecture>
- Agent2Agent Specification：<https://google-a2a.github.io/A2A/specification/>
