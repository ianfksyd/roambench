## R1 裁决（Go 后端）：FAIL

1 个 blocker，5 个 major。说明：本轮 shell 被禁用，我没能跑 `git diff` 和 `go test`，结论全部来自静态阅读源码，没有测试运行结果佐证。

### 四项结论

**1. 安全红线：FAIL**（问题在 d，a/b/c 通过）

- a) PASS。`recordCheckpointDecision` 只有 `server.go:196` 这一个入口，走的是 session 鉴权（`authRequired`），唯一调用点在 `project_control.go:6521`。Agent 侧的 `handleAgentInteraction`（`interactions.go:110-170`）只开放 GET、wait、cancel，没有 respond。dispatch 的批准只能走 `task_projector.go:175-189` 或人工 checkpoint。
- b) PASS。`fs_list.go:12-36` 先做词法检查，再对 home 和目标都做 `EvalSymlinks`，并重新校验在 home 内；子项逐个复检（81-84）。repoPath 的三个执行入口都是先 `EvalSymlinks` 再 `validate`：`project_takeover.go:64-70`、`project_takeover.go:306-312`、`project_detail.go:132-138`，board 也一样。
- c) 基本 PASS。人工路由用 `agent=false`（`project_lifecycle.go:640`）；`/api/agent/v1/pause` 用 `agent=true`，而 `controlTakeover` 在 438-440 限定 agent 只能 pause。但 checkpoint 路径能绕过这道守卫，见 M1。
- d) FAIL，见 B1 和 M2。

**2. 接口字段与数据诚实：FAIL（major）**

- codeChanges 的数据是真的。`gitstatus/changes.go` 用真实的 `git diff --numstat HEAD` 加 `ls-files --others`，环境做了净化，fsmonitor 和 ext-diff 都关掉了。全文没有虚构的百分比。
- `checkpointsByKind` 名不副实，见 M3。
- `ready` 在后端仍是 `"ready"`。对外文案「领取已批准任务」（`project_lifecycle.go:617`）语义正确，但 `project_detail.go:93` 把一个不可领取的 ready 任务称作「待领取」，见 N1。

**3. 测试真实性：PASS，但缺口多**

- 没发现 mock 被测函数，也没发现为了过测试改 stub。测试走的都是真东西：HTTP mux、SQLite、git、带软链的真实文件系统。
- `controlplane/claims_test.go:50-75` 用 start barrier 制造了真竞态。
- server 层的 `agent_claims_test.go:209-235` 没有 barrier，而且会被 `s.mu` 串行化，实际测的是串行正确性，不是竞态。
- 弱断言和缺失的用例见 M5。

**4. checkpoint 对外部运行时的放行语义：FAIL（major）**

- 任务未登记时，两条路径的行为一致：跳过暂停、照常建 checkpoint（`project_control.go:6285-6296`；`interactions.go:93-101`）。`runtime_checkpoint_persistence_test.go:16` 覆盖了这个路径。
- 但任务已登记、项目处于 manual 模式时，两条路径分叉，见 M1。

### Blocker

**B1** `project_lifecycle.go:170`、`agent_claims.go:76`、`project_takeover.go:206`

- 问题：三处高风险判断都只认 `RiskLevel == "high"`。但 `normalizeProjectControlRisk`（`project_control.go:5248-5254`）允许 `"critical"`，创建（2852）和更新（3094）都能写入这个值。结果是 critical 任务在接管下会被自动领取、自动执行，风险越高反而越不受门控。人工 dispatch 审批只能部分缓解。
- 修法：抽一个 `isHighRisk(level)` 函数，覆盖 high 和 critical，三处统一调用；补 critical 用例。

### Major

**M1** `project_control.go:6288-6293`，对照 `project_lifecycle.go:470`、`project_takeover.go:267`、`interactions.go:94-100`

- 问题：`managedProject` 把 manual 也算作受管。
  - `agentRequestCheckpoint` 直接调用 `pauseControlState`，绕过了 agent 守卫：在 manual 项目里，agent 能给任务打上 PauseReason，把人工正在跑的 ToolRun/PhaseAttempt 标成 failed。
  - 传 `reasonCode=path_violation` 时，还能把 manual 改成 paused。之后人工一点 resume，状态直接变成 active，跳过了 start 前置条件（lifecycle 必须是 executing）。
  - 同样场景下，`handleAgentInteractions` 走 `controlTakeover(agent=true)` 会被拒绝，并且持续返回 500，而 interaction 已经创建了。两条路径语义不一致。
- 修法：
  - agent 侧统一走 `controlTakeover(..., true)`，或者在调用 `pauseControlState` 前检查 `Mode ∈ {active, paused}`。
  - manual 模式下只建 checkpoint，不暂停。
  - interactions 侧对「未接管」的情况视同放行，不返回 500。
  - 补 manual 模式的用例。

**M2** `interactions.go:93`

- 问题：暂停钩子只匹配 `high`、`critical`、`high_risk`，而设计文档和测试 fixture 用的是 R0–R3 分级（`docs/internal/mobile-approval-...md:419,523`，fixture 里是 `"R1"`）。R2/R3 级别的破坏性操作或凭据变更永远不会触发接管暂停。另外，`allowedActions` 由 agent 自报，服务端没有禁止对高风险请求提供 `approve_session`（`task_projector.go:121`）。这一点可能是存量代码，不一定在本次 diff 里。
- 修法：风险分级在服务端归一化（把 R2/R3 映射为高风险）。高风险请求剔除 `approve_session`，强制逐次人批。

**M3** `project_detail.go:42,72-83`，`project_control.go:1384-1394`

- 问题：`checkpointsByKind` 同时统计 `checkpoint_resolved` 和 `artifact_recorded`。agent 每提交一个 artifact 计数就加一，agent 可以自行刷高「检查点」数字。`project_lifecycle_test.go:TestI2ActivityCounts...` 把这个错误口径固化成了断言（plan/test_result/diff_summary 都被计入）。
- 修法：拆成 `checkpointsByKind`（只统计人工裁决）和 `artifactsByKind` 两个字段，测试跟着改。

**M4** `project_takeover.go:104-112`

- 问题：验收命令在 agent 可写的 repo 里运行（比如 `./run_tests.sh`、Makefile），agent 改掉脚本就能自证通过。命令还继承了服务端的完整环境变量（`cmd.Env` 为 nil，包括 `ROAMBENCH_PASSWORD_HASH`）。另外，同 uid 的 agent 本来就能直接改 JSON 状态文件或 SQLite，这是架构层面的残余风险，需要写进文档。
- 修法：
  - 净化环境变量。
  - 验收前对比 acceptance 相关文件的 hash，或者在 milestone 人工验收时展示测试文件的 diff。
  - 在文档中声明同 uid 的信任边界。

**M5** 测试缺口

- `TestI2ContractLifecycleAndDetailHTTP` 里的 `len(CheckpointsByKind)!=4` 恒为真；codeChanges 在 HTTP 层完全没有断言（测试用的 home 不是 git 仓库）。
- 以下场景没有测试：
  - 暂停时取消正在运行的验收命令（`acceptanceRuns`）
  - critical 级别的任务
  - R2/R3 级别的 interaction
  - manual 模式下的 checkpoint
  - outcome 大小写
- 修法：逐项补上；HTTP 层的 claim 竞态测试加 start barrier。

### Nit

- **N1** `project_detail.go:93`：文案改成「已批准但缺验收命令，无法领取」。
- **N2** `agent_claims.go` 的 `agentReleaseTask`：只把归一化后的 outcome 赋给局部变量，传给 `releaseManagedTask` 的还是原始 `req`。agent 发 `"Done"` 时会跳过验收并以 `task_failed` 暂停。结果是失败关闭（不会误放行），但属于误伤。修法：先 `req.Outcome = outcome`，再调用。
- **N3** `validateProjectControlRepoPath`（`project_control.go:381-406`）只做词法检查，依赖每个调用方自己先做 `EvalSymlinks`，比较脆弱；另外 `$HOME` 本身是软链时会误拒。建议把 `EvalSymlinks` 收进函数内部（对 home 和目标都做）。
- **N4** `handleAgentRequestCheckpoint`：没有 `MaxBytesReader`，也没有校验 TaskID 的格式和长度，agent 可以给任意 ID 刷 pending checkpoint。
- **N5** `state.Activity` 无上限增长，而且每追加一次都做 O(n) 去重。
- **N6** `fs_list_test.go`：「outside」和「prefix sibling」是同一个路径，后者没测到同前缀的兄弟目录；`agent_claims_test.go:27` 的注释「not yet wired」已经过时。

### 已确认通过

- Agent 拿不到 respond、approve、resume、stop、start 中的任何一个。
- fs list 和 repoPath 的软链逃逸防护有效，测试覆盖了逃逸、断链、`..`、NUL 字节。
- pause 会在同一把锁内同时完成三件事：撤销租约、让 PhaseAttempt/ToolRun 失败、取消验收命令。
- resume 没有持久化 checkpoint 时必须显式 `restart=true`，并且必须重新 claim，旧租约作废。
- 验收连续失败两次后，必须人工重启。
- 代码变化统计来自真实 git，没有虚构数据。
