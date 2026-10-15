# REVIEW C8 Round 2 复审（Hermes 独立验证）

candidate_sha: f2f43d9
日期: 2026-10-15

## 逐项复核 R1 findings

- **blocker（K4 断裂）→ RESOLVED (confirmed)**：
  - `syncProjectControlDispatchCheckpoint` 已挂 4 个同步点（createTask/updateTask/tool 完成/recordCheckpointDecision）；新建任务即产生 pending dispatch checkpoint，dispatchState=awaiting_approval。
  - FIXER 发现并修复了隐藏断裂点：approve 走 control-plane 交互路径时，`resolveControlPlaneCheckpointDecision` 会短路 legacy 钩子——`task_projector.go:174` 已加 `case "dispatch"`。这正是"修了表面钩子仍不通"的那类 bug，回归测试 `TestDispatchCheckpointCreationAndApprovalGatesClaim` 覆盖创建→审批→claim→release→reject 全链路（实测 PASS）。
  - C7 前端三项验收：审批 UI 覆盖 dispatch（approvalKindLabel/Summary/状态卡/徽章）；向导第 3 步 T1-T4 推荐展示（riskLevel→tier 高亮，失败静默降级）；任务详情 Agent dispatch 卡显示 dispatchState/claimedBy/租约剩余秒数（从 timeline 事件时间戳推算）。
- **major#1（token 恒时比较）→ RESOLVED**：`project_control.go:6000` 已用 `subtle.ConstantTimeCompare`。
- **major#2（claim TOCTOU）→ RESOLVED (defensive)**：`applyClaimToTaskRechecked` 在 withStateLocked 内复查 dispatchState==ready，竞争失败回滚 lease 返回 409；原并发测试不受影响。
- **nit（fsmonitor）→ RESOLVED**：git 命令已追加 `-c core.fsmonitor=false -c core.fsmonitorDaemon=false -c fsmaintenance.enabled=false`。
- **nit（cache 上限）→ RESOLVED**：gitstatus Cache 64 entries，append-order 淘汰。
- **nit（probe 锁阻塞 / nonInteractive 死字段 / server 层 heartbeat 过期断言弱）→ DEFERRED**：按 R1 裁决记为已知项，不阻塞本 worksheet。

## Hermes 独立验证（不信任代理自报）

- `go build ./...` 通过；`go test ./... -count=1` 11 包全绿；`node --test web/*.test.js` 10/10。
- `-race` 复跑 server+gitstatus：干净（61.9s / 1.8s）。
- 代码位抽查：task_projector.go:174 dispatch 分支、project_control.go:6000 ConstantTimeCompare、agent_claims.go:243 rechecked 复查、gitstatus fsmonitor 覆写——四处全部在位。
- 工作树干净（fixer 提交 f2f43d9）。

## 已知限制（记录，不阻塞）

1. FIXER 的浏览器 E2E 截图未产出（本地 Chromium 拉起超时）——server 日志链路证据完整（201/200/204 状态码逐条），但 UI 渲染层建议人工开浏览器确认一次。
2. R1 三条 deferred nit。

## VERDICT: PASS

新问题仅 suspected 及以下、无 confirmed blocker、R1 blocker/major 全部消化——按防漂移规则（R2 起新问题仅 blocker 才判 FAIL）判定通过。
