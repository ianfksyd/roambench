# REVIEW C8 Round 1 (candidate_sha=0170a34)

VERDICT: FAIL

FINDINGS:

- [blocker] internal/server/project_control.go:3154 + web/（缺文件）| K4 断裂：全仓库没有任何代码创建 `Kind="dispatch"` 的 checkpoint，也没有任何路径把 dispatchState 置为 `awaiting_approval`（grep 全部非测试 .go/.js 仅命中审批钩子两处和注释）；前端（project-panel.js/app.js）完全没有 dispatchState/claimedBy/dispatch 审批 UI（`grep -rn 'claimedByRuntimeId|dispatchState' web/` = 0 命中，approvalKindLabel 只有 final_acceptance/archive_override 两分支）。task card C7 的三项验收（新建任务展示推荐 agentLabel、页面批准 dispatch checkpoint、任务详情显示 claimedBy 与租约剩余时间）全部缺失，无对应提交（log 只有 C0/C1/C2C4C5/C3C6）。后果：3154 行的批准钩子永远不可达，用户无法在产品里把任务变成 ready → Hermes 永远 claim 到 204，北极星"批准后自动领走"端到端不通。agent_claims_test.go:8-10 注释自认测试是直接写 DispatchState 绕过钩子。| 建议修法：补创建 dispatch checkpoint 的入口（任务创建/agentRequestCheckpoint 扩展 kind=dispatch），并补 C7 前端（checkpoint kind=dispatch 审批按钮 + claimedBy/lease 剩余时间 + 推荐 agentLabel）。| evidence: `grep -rn '"dispatch"' internal/ web/ --include=*.go --include=*.js`（仅 project_control.go:3154,3173 与注释）；`grep -rn 'awaiting_approval' internal/ web/ --include=*.go --include=*.js | grep -v _test`（仅常量定义）；`git log --oneline 6ea5585..HEAD`（无 C7 提交）

- [major] internal/server/project_control.go:5922 | agent token 比较非恒时：`state.AgentToken == token`；新的 claim/heartbeat/release 三端点经 tokenRequired（contract_stubs.go:16）复用此比较，扩大了时序侧信面。基线已存在但本次 diff 新增 3 个消费端点。| 建议 `subtle.ConstantTimeCompare`。| evidence: `sed -n 5918,5924p internal/server/project_control.go`；base 版本同函数已存在（git show 6ea5585 行 5858）

- [major, suspected] internal/server/agent_claims.go:72-80 | claim TOCTOU：readyTaskIDsForUser 取的是 loadOrSeed 状态快照，之后 ClaimReadyTask 直接 INSERT 并由 applyClaimToTask 无条件覆写 dispatchState=claimed。若快照与覆写之间任务被并发 reject（dispatchState→none）或归档，claim 仍成功并覆盖审批结果。单用户+互斥锁下窗口极窄，未能稳定复现。| 建议 applyClaimToTask 前在 withStateLocked 内复查 dispatchState==ready。| evidence: 逻辑推演（agent_claims.go:72 快照 → claims.go:158 INSERT → agent_claims.go:224 覆写，中间无状态复查）

- [nit] internal/agentprobe/agentprobe.go:86-93 | Probe 持 mutex 执行整个 probeAll（5 agent × 5s 超时，最坏 25s），refresh 期间阻塞所有 /agents 请求包括缓存命中。| 建议 copy-on-read 或探测放到锁外。| evidence: 代码 87-92 行锁内调 probeAll

- [nit] internal/agentprobe/agentprobe.go:47 | K3 字段 nonInteractive 恒为 false，无任何检测逻辑（死字段）。契约 JSON 一致但语义缺失。| 建议实现或注明待补。| evidence: `grep -rn 'NonInteractive' --include=*.go .`（仅结构体定义）

- [nit] internal/server/agent_claims_test.go:268 | heartbeat 过期测试用伪造 leaseId "lease-from-expired"，只验证未知租约 404，未验证真实过期租约被拒（404 恒真面）；真实覆盖在 controlplane/claims_test.go:103 有，但 server 层断言偏弱。另外 gitstatus Cache（gitstatus.go:303-318）无容量上限/淘汰，repoPath 由用户增删，长期运行 entries 只增不减。| 补真实过期租约 heartbeat 测试；cache 加上限。| evidence: 读源码

- [nit, suspected] internal/server/project_board.go:109-121 | repoPath TOCTOU：EvalSymlinks→validate→git exec 之间符号链接可被替换指向 home 外；以及 repo 本地 .git/config 的 core.fsmonitor 不受 env 清洗（GIT_CONFIG_GLOBAL/SYSTEM=/dev/null 只压全局/系统层），`git status` 仍可能拉起 fsmonitor daemon。GATE-1 #4 home 限制+env 清洗已显著缓解，单用户前提下风险低。| 建议 git 命令追加 `-c core.fsmonitor=` 覆写。| evidence: gitstatus.go:177-192 env 清洗清单无 fsmonitor 项

## 审查员确认通过项

`go test ./... -race -count=1` 全绿；并发 claim 唯一索引竞争测试真实有效；租约过期注入时钟、重启存活测试到位；git 超时进程组清理实测无残留；board 轮询定时器视图切换/登出均清理；K2/K5/K6 契约字段名与 01-plan.md 逐项一致；agentGetTask 410、repoPath home 限制、models 空数组均符合 GATE-1 五项裁决；diff 无无关顺手改动。

## 裁决（Hermes）

- blocker = **C7 未做**（我此前说"C7 等回来后串行上"，C8 审查在我派 C7 之前——审查员 correctly 发现计划中的缺口）。处置：立即派 C7 + checkpoint 创建入口（K4 断裂修复）。
- major#1（恒时比较）：accepted，修。
- major#2（claim TOCTOU，suspected）：accepted as defensive fix，修（withStateLocked 内复查）。
- nits：fsmonitor 覆写与 cache 上限采纳（小改动）；Probe 锁与 nonInteractive 死字段、测试加强记录为已知项，不在本 worksheet 修。
