HEAD 即 752938f，shell 不可用，我将直接读取工作树源码核对。## R2 复核裁决：PASS（有前提）

前提：本轮 shell 仍被禁用（R1 时也一样）。我跑不了 `git show 752938f`，也跑不了 `go test`。当前 HEAD 就是 752938f（我核对过 `.git/refs/heads/main`），所以我直接读了工作树源码，和 R1 报告逐条对照。以下结论全部来自静态核实，没有测试运行结果佐证。合入前必须由 Hermes 或 CI 跑一遍 `go test ./internal/...`，确认全绿。如果出现红灯，这个 PASS 自动作废。

### 逐条核实

| 编号 | 结论 | 核实依据 |
|---|---|---|
| B1 | 已修 | 新增 `isHighRisk()`（`project_lifecycle.go:162`），覆盖 high 和 critical。R1 点名的三处都改成调用它：`taskClaimAllowed:179`、`agent_claims.go:75`、`project_takeover.go:212`。全仓 grep 不到残留的 `=="high"` 判断。`TestR1CriticalTaskGates` 覆盖了 high 和 critical 两种情况，断言领取谓词为假、claim 返回 204、项目被暂停且 reason 为 high_risk。另有一个用例把运行中的任务改成 critical，断言项目被暂停、租约变成 released。断言验证的是真实状态，不是返回码。 |
| M1 | 已修 | `agentRequestCheckpoint` 现在只在 `takeoverRunning`（mode 为 active 或 paused）时才调用 `pauseControlState`，和 `controlTakeover(agent=true)` 的守卫等价，而且在同一把锁内执行。manual 模式下只建 checkpoint，不暂停。`pauseExecutionViolation` 在未接管时返回 nil，interactions 侧不再报 500。`TestR1ManualCheckpointDoesNotPauseHumanWork` 用 agent_request 和 path_violation 两种 reason 测试，断言 mode 仍是 manual、lifecycle 仍是 planning、PauseReason 为空、人工的 PhaseAttempt 和 ToolRun 仍是 running、checkpoint 已持久化，并且 manual 下 resume 返回 400，无法绕过 start。这是真行为断言。 |
| M2 | 已修 | 新增 `controlplane.NormalizeRiskClass`：R0→low，R1→medium，R2/high_risk→high，R3→critical。`CreateInteraction` 在入库时归一化，高风险请求剔除 `approve_session`。`validateResponse` 再兜底拒绝高风险请求的 `approve_session`，属于双层防护。测试覆盖 R2、R3、带空格的 HIGH_RISK、critical，并在 active 和 manual 两种模式下分别验证。 |
| M3 | 已修 | 统计拆成两个字段：`checkpointsByKind` 只统计 `checkpoint_resolved`，`artifactsByKind` 统计 `artifact_recorded`。我追查了 `checkpoint_resolved` 的来源，只有人工 decision 路径（`project_control.go:3307`）和 respond 产生的 projection（`task_projector.go:96`）。agent 的 cancel 不会写入 `task_projections`，所以 agent 刷不了这个数。旧测试已改成精确的 DeepEqual 断言。 |
| M4 | 已修 | 环境变量改成白名单（`acceptanceEnvironment` 只传 PATH、HOME、LANG 等），测试断言密钥变量和 BASH_ENV 不会被继承。人工批准时记录验收输入的指纹基线，release 时比对；发生修改、新增或删除都会暂停，必须人工 restart 才能继续。`deployment-hardening.md §13` 已写明同 uid 的信任边界。 |
| M5 | 已修 | HTTP 层的 codeChanges 断言了全部字段。以下用例都已补上：暂停时取消验收命令（含进程组 kill 和注册表不泄漏）、critical 任务、R2/R3 interaction、manual 模式 checkpoint、outcome 大小写。HTTP 层的 claim 竞态测试加了 ready+start barrier。 |
| N1 | 已修 | 文案改为「已批准但缺验收命令，无法领取」（`project_detail.go`）。 |
| N2 | 已修 | 先执行 `req.Outcome = outcome` 再调用 `releaseManagedTask`。测试覆盖了 `" Done "`、`FAILED` 和 `" Released "`。 |
| N3 | 已修 | 函数内部对 home 和目标都做 `EvalSymlinks`。测试覆盖了 home 本身是软链的情况，以及经软链逃逸的情况。 |
| N4 | 已修 | 加了 `MaxBytesReader`（1 MiB），TaskID 限制在 128 字节以内并校验格式。测试断言非法请求不会被持久化。 |
| N5 | 基本修复 | 加了上限 4096，去重扫描因此有界。 |
| N6 | 已修 | 兄弟目录改成独立的 `home-other`，新增 traversal 用例，过时的注释已删除。 |

没有发现假修复。所有改动都改变了实际语义，测试断言检查的是落盘状态、进程副作用或租约状态，不只是 HTTP 码。

### 遗留风险（都不阻断，建议排进后续轮次）

1. **风险词表不一致，而且未知值放行。**
   - 服务端的 `isHighRisk` 不做归一化。`normalizeProjectControlRisk` 不认识 R 级，会把任务的 `riskLevel:"R3"` 静默降成 medium。
   - interaction 侧遇到未知值（比如 `R4`、`severe`、空串）都按非高风险处理，属于失败开放。
   - riskClass 本身由 agent 自报。
   - 建议：统一用 `controlplane.IsHighRisk`，未知值按高风险处理。
2. **验收指纹有 TOCTOU 窗口。** 从指纹比对通过到 `cmd.Run` 之间有时间差，而这个时机由 agent 控制。指纹也不覆盖 `vendor`、`node_modules`、`.venv` 目录，以及仓库外用绝对路径引用的文件。代码注释已承认它只是绊线，不是沙箱，验收是否可信最终还是靠里程碑人工审 diff。
3. **高风险任务在接管下会反复暂停。** 每次 claim 都会因 ready 的 high/critical 任务暂停整个项目，restart 之后又会被暂停。这些任务实际上只能退出接管、由人工执行。这是失败关闭的方向，安全上没问题，但需要在 UX 或文档里说明。
4. **interaction 先落库、后暂停，两步不是原子的。** 暂停时如果遇到 IO 错误，接口返回 500，但 interaction 已经创建了。
5. **Activity 上限是全局共享的。** 被淘汰的 source 如果被重放会重复计数，老项目的统计也会慢慢衰减。
6. **checkpoint 刷量没有限流。** 未登记的任务 ID 按设计是放行的，agent 仍然可以用格式合法的随机 ID 刷 pending checkpoint。
7. **前端没有展示 `artifactsByKind`。** 堆叠条现在只反映人工裁决，相关文案需要和这个语义对齐（R1 只审了后端，这条不在范围内）。
8. **一处测试断言偏弱。** `TestR1PauseCancelsAcceptanceCommand` 检查 `acceptance-escaped` 不存在，但被测进程要 `sleep 30` 才会写这个文件，所以这条断言本身证明不了什么。真正能证明进程被取消的，是 release 在 3 秒内返回 409。
