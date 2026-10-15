# WS-20261015-001 规划（F1 项目看板 / F2 Hermes 接手）

Planner: claude-opus-5.5 (kiro-cli, effort=high) — 2026-10-15
base_sha: 6ea5585ffdac21881739fd223eb488cb5fdc38cb

探查说明：shell 在非交互模式下被拒，没能跑 `git log` 和 `which`，所以 Go 缺失、四家 agent 版本两条直接沿用输入包。下面的代码事实都是我读文件确认过的：
- `projectControlProject` 只有 id/key/name/status/currentGoal 这几个字段，没有仓库路径。
- 全局只有一个 `runtimeRootDir`，取自 server 启动时 cwd 的 git root。
- Project 状态存在 `.project-control/users` 下的 JSON 里，不在 SQLite；SQLite 的表结构定义在 `repository.go` 的 `schema` 常量和 `schema_migrations`。
- `agentGetTask` 只返回第一个处于 running 状态的任务，没有领取或租约机制。
- `go.mod` 写的是 go 1.18，README 写的是 1.22+，两处不一致。

## 设计契约（所有卡必须遵守）

K1 Project 新增字段 `repoPath`（string，可为空，为空时回落到 runtimeRootDir）。存储仍走 JSON，写入时校验：必须是绝对路径、目录存在。

K2 `GET /api/project-control/board`，走 authRequired。返回：
```
{generatedAt, projects:[{projectId,name,status,
  currentTask:{taskId,title,state,currentPhase}|null,
  lastActivityAt,lastActivitySummary,
  git:{repoPath,available,error,branch,headSha,upstream,ahead,behind,
       dirty,changedCount,untrackedCount,
       worktrees:[{path,branch,headSha,isCurrent}]}}]}
```
git 只能执行只读命令，单条超时 3s，按 repoPath 缓存 10s。git 出错时 `available=false` 并填 `error`，接口不返回 500。

K3 `GET /api/project-control/agents`，加 `?refresh=1` 时强制重新探测。返回：
```
{detectedAt, agents:[{agentLabel,binary,path,installed,version,nonInteractive,models[]}],
 recommendations:[{tier:"T1".."T4",workerLabel,reviewerLabel,reason}]}
```
agentLabel 的取值固定为 `kiro-cli|claude|codex|opencode|hermes`。探测只允许执行 `--version` 这类无副作用命令，超时 5s。

K4 Task 新增两个字段：`dispatchState`（`none|awaiting_approval|ready|claimed|released|done`）和 `claimedByRuntimeId`。新增 checkpoint 类型 `kind="dispatch"`，批准后 dispatchState 变为 `ready`。

K5 Agent API v1 新增三个接口，都用 bearer 鉴权：
- `POST /api/agent/v1/claim`：请求 `{runtimeId,agentLabel}`，返回 `{taskId,leaseId,leaseExpiresAt,...原 task 字段}`，没有可领任务时返回 204。
- `POST /api/agent/v1/heartbeat`：请求 `{leaseId}`。
- `POST /api/agent/v1/release`：请求 `{leaseId,outcome}`。

租约 TTL 为 120s，过期后任务回到 `ready`。

K6 SQLite 新增表 `task_claims`，`schema_migrations` 版本号 +1：
```
claim_id PK, username, task_id, runtime_id, agent_label,
status CHECK(active|released|expired),
lease_expires_at, created_at, updated_at
UNIQUE INDEX(task_id) WHERE status='active'
```
同一时刻一个任务最多只有一个 active 领取。

## 任务卡

| 卡 | 级别 | 推荐 agent | 内容 |
|---|---|---|---|
| C0 | T1 | kiro-cli | 工具链：按 GATE-1 结论装好 Go，并跑通基线 |
| C1 | T1 | kiro-cli | 契约桩：先把 K1–K6 的骨架落进共享文件 |
| C2 | T3 | codex | F1 后端：看板接口 |
| C3 | T2 | claude | F1 前端：看板视图 |
| C4 | T2 | opencode | F2 agent 探测 |
| C5 | T3 | codex | F2 领取与审批 |
| C6 | T2/T3 | claude | Hermes 适配（级别看 GATE-1 的 B 项） |
| C7 | T2 | claude | F2 前端 |
| C8 | T2 | kiro-cli | 集成审查（用没写过 C2/C5 的 agent） |

C0：Go 工具链与基线
- 验收：`make build` 成功，在 base_sha 上 `go test ./...` 全绿，记下 Go 版本。
- owned_files：无，只改环境。

C1：契约桩
- 验收：
  - Project、Task 新增的字段完成序列化，旧 JSON 读入后默认值正确。
  - K2、K3、K5 的路由注册完毕，暂时返回 501。
  - K6 的迁移可以幂等执行。
- 测试：旧 JSON 兼容测试、迁移重复执行测试、路由鉴权测试（不带凭据返回 401）。
- owned_files：`project_control.go`（只动结构体字段和 repoPath 校验）、`server.go`（只动路由行）、`internal/controlplane/repository.go`（只动 schema）。
- 约束：C1 之后其他卡不得再改这三个文件的上述区域。

C2：F1 后端
- 验收：
  - 用临时 git 仓库造出 detached、ahead=2/behind=1、dirty、两个 worktree、无 upstream、非仓库六种情况，每种都返回正确字段。
  - git 超时不会阻塞整个响应。
- 测试：表驱动测试，在 `t.TempDir()` 里真实执行 `git init`、`git worktree add`。
- 约束：不调用会写入的 git 命令；repoPath 不能经由软链逃逸。
- owned_files：新建 `internal/gitstatus/*`、`internal/server/project_board*.go`。

C3：F1 前端
- 验收：
  - 每个项目显示一张卡，包含任务/阶段、最近活动、分支、↑↓、dirty 标记、worktree 数量。
  - git 不可用时显示原因。
  - 375px 宽的手机屏幕可用。
  - 中英日三语都有文案。
- 测试：手工验收清单加截图；如果把纯函数抽出来，用 `node` 跑单测（参照 `memory-status.test.js`）。
- owned_files：`project-panel.js`、`style.css`、`index.html`（i18n 部分）。

C4：F2 agent 探测
- 验收：
  - 本机四家 agent 都能识别出版本。
  - PATH 里缺失的 agent 标记为 `installed=false`。
  - 推荐结果确定、可复现：输入相同则输出相同。
- 测试：用假 PATH 目录放桩脚本测试，覆盖超时、版本输出乱码两种情况。
- owned_files：新建 `internal/agentprobe/*`、`internal/server/agent_probe*.go`。

C5：F2 领取与审批
- 验收：
  - 只有审批通过的任务能被 claim。
  - 并发 claim 只有一个成功。
  - 租约过期后任务回到 `ready`。
  - release 和 heartbeat 写入 Timeline 和 Audit。
  - 拒绝审批后任务不可领取。
- 测试：并发 claim 竞争测试、租约过期测试（注入时钟）、checkpoint 批准/拒绝联动测试、服务重启后租约仍在的测试。
- owned_files：新建 `internal/controlplane/claims*.go`、`internal/server/agent_claims*.go`。审批钩子只能以新函数形式挂接。

C6：Hermes 适配
- 验收：hermes 进程以 `runtimeId` 注册为 `kind=hermes`，按 claim → heartbeat → artifact → release 走完一整轮；Runtimes 视图里能看到它的健康状态。
- 测试：用模拟 Hermes 的脚本跑端到端测试。
- owned_files：`cmd/roambench-agent/main.go`（新增 claim/heartbeat/release 子命令）和新的适配文档。

C7：F2 前端
- 验收：
  - 新建任务时展示推荐的 agentLabel。
  - dispatch checkpoint 可以在页面上批准。
  - 任务详情显示 claimedBy 和租约剩余时间。
- owned_files：与 C3 相同。

C8：集成审查
- 验收：`go test ./...` 全绿，F1、F2 的验收口径逐条打勾，前端没有引入任何框架。

## 依赖与并行

依赖顺序：
```
C0 → C1 → {C2 ∥ C4 ∥ C5}
C2 → C3
C4 + C5 → C6
C3 + C5 → C7
全部 → C8
```

C2、C4、C5 拥有的文件互不交叉，可以并行。C3 和 C7 都要改 `project-panel.js`，必须串行，所以全局关键路径是 C0 → C1 → C5 → C7 → C8。

## 北极星

浏览器打开看板，能看到每个项目真实的 git 分支状态和当前进展；在 checkpoint 批准一个任务后，Hermes 会自动领走、执行、回传证据，整个过程都能在 Timeline 里回放。

## 风险与开放问题（需要 GATE-1 裁决）

1. **Go 工具链。** 我推荐 C0 直接下载官方 Go 1.22+ 的 tarball，装到 `~/.local/go`：不用 sudo，删掉就能撤销。备选方案是 docker `golang` 镜像，或者只在 CI 上验证；但只靠 CI 意味着每张卡都没法在本地自验，我不推荐。另外请一并决定是否把 `go.mod` 里的 1.18 改成和 README 一致的 1.22。
2. **Hermes 适配做到多深。**
   - A：只做 API 对接，Hermes 自己来拉任务。
   - B：server 直接拉起 hermes 进程。
   - 我推荐本 worksheet 只做 A：边界干净，也符合 agent-neutral，server 不用托管第三方进程的生命周期。如果要做 B，我建议通过现有 tmux 终端管理器启动 hermes，不要裸 `exec`，这样会话可以 attach 观察、重启后也能恢复。B 会让 C6 升到 T3，并增加 cgroup 和凭据注入方面的风险。
3. **模型列表。** 各 CLI 不一定提供可以稳定解析的模型查询命令。我建议 `models` 允许为空，推荐只按 agentLabel 和级别给出。级别到 worker/reviewer 的映射表要不要放进配置文件，需要你拍板。
4. **repoPath 的风险。** repoPath 由用户填写，server 会在这个目录里执行 git，属于命令执行面。仍然是单用户加鉴权的前提，但需要确认是否把 repoPath 限制在某个根目录之下。
5. **现有的单任务假设。** `agentGetTask` 目前默认只有一个活跃任务。claim 机制上线后，这个旧接口是保留还是标为废弃，需要你决定。
