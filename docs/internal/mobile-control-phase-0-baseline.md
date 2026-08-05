# 手机交互控制与多 CLI 协同：阶段 0 基线

日期：2026-08-04
状态：完成
代码基线：`4e76a34`（工作区另有未提交的用户改动）

## 结论

阶段 0 的四项任务和三项退出条件已形成可重复证据：

- 浏览器未连接时 OSC 不会被采集，带 build tag 的真实 tmux reproducer 可以稳定复现；
- Codex、Claude Code、OpenCode、Kimi 各有一个统一格式、可解析和可关联重放的审批 fixture；
- tmux 选择每个 session 一个 server-owned control-mode observer，`pipe-pane -O` 作为兼容降级；
- ADR-0001 决定由 SQLite 接管手机控制面的审批事实和 outbox，JSON/WAL 保留非审批 Project Control 实体。

阶段 1 可以开始。不得先实现 PWA 或 Push，也不得在 JSON 和 SQLite 中双写 Checkpoint/Decision。

## 环境与支持下限

阶段 0 执行环境：Linux arm64、Go 1.22.2、tmux 3.4。

| CLI | 本机版本 | 第一版支持下限 | Fixture 依据 | 状态 |
|---|---|---|---|---|
| Codex | `codex-cli 0.146.0` | `0.146.0` | 本机 `app-server generate-json-schema` + 官方 App Server 手册 | 已本机生成 schema |
| Claude Code | `2.1.222` | `2.1.222` | 本机版本 + 官方 `PermissionRequest` hook 文档 | 已核对输入/输出 |
| OpenCode | `1.18.10` | `1.18.10` | 本机 `/doc` OpenAPI 3.1 | 已本机生成 OpenAPI |
| Kimi Code CLI | 未安装 | `1.25.0 / Wire 1.6` | 官方 Wire 文档；1.25.0 引入 rejection feedback | 可重放，未本机执行 |

支持下限是本项目的固定验证基线。更旧版本统一视为未验证，Adapter 必须 capability/version 探测后拒绝结构化自动操作，降级到通知和手动接管。

Kimi 本机缺失不阻止阶段 1 的通用 Gateway 实施，但会阻止阶段 4 的 Kimi Adapter 发布。进入 Kimi Adapter 开发前必须安装目标版本，并用真实 Wire 会话替换或补充当前官方文档 fixture。

Fixture 位置：

```text
internal/protocolfixture/testdata/
├── codex/approval.json
├── claude/approval.json
├── opencode/approval.json
└── kimi/approval.json
```

## OSC 生命周期失败证据

当前 `OSCScanner` 只在 terminal WebSocket attach handler 内创建并消费 PTY 输出。没有浏览器 terminal WebSocket 时，tmux session 仍运行，但不存在把 pane 输出送入 scanner 和 notification hub 的读者。

可重复 reproducer：

```bash
go test -tags phase0_repro ./internal/server \
  -run '^TestOSCNotificationIngestContinuesWithoutTerminalWebSocket$' \
  -count=1
```

当前预期结果是失败，稳定错误为：

```text
OSC notification was not ingested without a terminal WebSocket within 750ms;
scanner lifecycle is still attach-scoped
```

该测试使用独立 `TMUX_TMPDIR`、真实 tmux session 和 OSC 9 输出，不依赖源码文本匹配。它被 `phase0_repro` build tag 隔离，默认测试套件保持绿色。阶段 2 实现常驻 observer 后，应移除 build tag 或把它转成默认回归测试，并要求其通过。

## tmux observer 对比与选择

探针命令：

```bash
go test -tags phase0_probe ./internal/terminal \
  -run '^TestPhase0(PipePane|ControlMode)' \
  -count=1 -v
```

实测结果：

| 项目 | `pipe-pane -O` | control mode |
|---|---|---|
| 无 Web terminal attach 时取得输出 | 通过 | 通过 |
| 影响 pane 尺寸 | 未观察到 | `123×37` attach 前后不变 |
| 输出形式 | 原始字节流 | `%output` control record，需反转义 |
| 生命周期事件 | 需要另行轮询/订阅 | 同一协议包含 session/window/pane 事件 |
| 资源模型 | 每个 pane 一个 pipe handler | 每个 tmux session 一个 control client |
| 与用户配置冲突 | 占用 pane 唯一 pipe 槽位 | 不占用 `pipe-pane` |
| 实现复杂度 | 低 | 中，需要 control protocol parser 和 backpressure |

选择 control mode，具体约束：

1. RoamBench 为每个受管 tmux session 启动一个 `tmux -C` observer，并保持 stdin 打开；
2. observer 只订阅和读取，不发送 resize、select、switch-client 或用户输入命令；
3. 解析 `%output` 的 pane ID 和 tmux 转义，再送入每个 pane 独立的 `OSCScanner`；
4. 处理 pane/window 创建、销毁、session 重连、`%exit` 和 control client 重启；
5. 为重复 attach/restart 生成稳定 event ID，防止与浏览器 attach 路径重复通知；
6. 明确处理 control-mode 输出暂停/backpressure，不能阻塞 tmux server；
7. capability probe 失败或目标 tmux 版本不兼容时，才启用 `pipe-pane -O` 降级。

阶段 2 仍需验证颜色、scrollback、多 pane、已有 session 恢复和长时间高输出；阶段 0 的选型不替代这些验收测试。

## 持久化策略

已接受 [ADR-0001](./adr-0001-mobile-control-persistence.md)：

- SQLite 唯一拥有 Interaction、Checkpoint、Response、Decision、Audit、Outbox、Delivery 和 idempotency；
- JSON/WAL 继续拥有 Project、Task、Artifact 等现有状态；
- 现有 Checkpoint/Decision 一次性迁移并从 JSON 可写模型中移除；
- Task 状态变化通过 SQLite durable event 投影到 JSON，允许短暂滞后但可重放；
- 禁止 JSON/SQLite 双写同一个审批实体。

阶段 1 的首个代码变更必须是 migration 和 repository 测试，随后才能接入 Interaction API。

## 阶段 0 证据命令

```bash
go test ./internal/protocolfixture -count=1
go test -tags phase0_probe ./internal/terminal \
  -run '^TestPhase0(PipePane|ControlMode)' -count=1 -v
go test -tags phase0_repro ./internal/server \
  -run '^TestOSCNotificationIngestContinuesWithoutTerminalWebSocket$' -count=1
go test ./...
```

前三条分别证明 fixture 可重放、两种 tmux 方案的实际能力和当前 OSC 生命周期缺口。第三条在阶段 2 修复前必须失败；最后一条必须通过。

## 官方来源

- Codex App Server：<https://developers.openai.com/codex/app-server/>
- Claude Code Hooks：<https://code.claude.com/docs/en/hooks#permissionrequest>
- OpenCode Server/OpenAPI：<https://opencode.ai/docs/server/>
- Kimi Wire mode：<https://moonshotai.github.io/kimi-cli/en/customization/wire-mode.html>
- Kimi changelog：<https://moonshotai.github.io/kimi-cli/en/release-notes/changelog.html>
- tmux manual：<https://man.openbsd.org/tmux.1>
