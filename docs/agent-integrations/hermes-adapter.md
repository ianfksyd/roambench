# Hermes Adapter (RoamBench Agent API v1)

Hermes 领取并执行 RoamBench 项目看板上的任务：按 **claim → 执行 → heartbeat 保活 → artifact 提交 → release** 的循环工作。RoamBench 服务端不会拉起 hermes 进程（GATE-1 #2 方案 A），Hermes 完全主动拉取。

## 0. 前置

- RoamBench server 已运行（下方以 `http://127.0.0.1:38303` 为例；生产一般是 `http://localhost:3000`）。
- `roambench-agent` 已构建：`make build`，产物在仓库根目录。下文用 `$ROAMBENCH_REPO/roambench-agent` 指代。
- 任务的 dispatch 已经过 checkpoint 批准（`dispatchState=ready`），否则 claim 永远 204。

## 1. 拿到 agent token（一次性，人肉操作）

Agent API 用独立于网页会话的 bearer token。获取方式需要**已登录的 session cookie**，因此由人（或一次浏览器自动化）完成：

1. 浏览器登录 RoamBench；
2. `POST /api/project-control/agent-token`（带 cookie，无 body），返回 `{"token":"rbag_..."}`；
3. 放进 Hermes 侧的环境变量：

```bash
# ~/.hermes 环境或 systemd user 环境均可
export ROAMBENCH_AGENT_TOKEN=rbag_xxxxxxxxxxxxxxxx
```

命令行里 `--token` 显式传入 > `ROAMBENCH_TOKEN` > `ROAMBENCH_AGENT_TOKEN`，三者任选。

> token 生成后持久保存在 server 端项目状态里，重启不变。若怀疑泄漏，重新调 `POST /api/project-control/agent-token` 不会轮换（返回同一个），需要人工清掉 server 状态里的 `agentToken` 字段后重取。

## 2. 任务循环（Hermes 侧伪代码）

```
loop every POLL_INTERVAL (建议 30-60s):
  out = roambench-agent claim --server $URL --runtime-id hermes-$HOST --agent-label hermes --json
  if out == no-task: continue

  # 拿到 taskId / leaseId / leaseTtlSeconds(=120)
  spawn heartbeat 循环: roambench-agent heartbeat --lease-id $LEASE --interval 60s
                       (TTL 120s，interval 必须 < 60% TTL)

  执行任务 (title/goal/nextStep/currentPhase 来自 claim 响应):
    - 过程中按 phase 要求提交证据:
      roambench-agent artifact --kind test_result --outcome pass --file ./out.txt

  kill heartbeat 循环
  roambench-agent release --lease-id $LEASE --outcome done    # 或 failed
```

### 子命令一览

```bash
URL=http://127.0.0.1:38303

# 领取。无可领任务时打印 no-task、退出码 0；成功打印任务 JSON
roambench-agent claim --server $URL --runtime-id hermes-$(hostname) \
  --agent-label hermes [--json]

# 保活。单次；加 --interval 进入循环模式（前台阻塞，Ctrl+C 停止）
roambench-agent heartbeat --server $URL --lease-id <leaseId> [--interval 60s] [--json]

# 释放。outcome ∈ done|failed|released
#   done     任务完结，dispatchState -> done
#   failed   失败交还，任务回到 ready 可被重新领取
#   released 放弃领取权（比如要换 runtime 重做），任务回到 ready
roambench-agent release --server $URL --lease-id <leaseId> --outcome done [--json]
```

人类可读输出（无 `--json`）示例：

```
$ roambench-agent claim --server $URL --runtime-id hermes-ian --agent-label hermes
Task:    C6: Hermes 适配
ID:      task-c6-hermes
Lease:   claim-xxxx (expires 2026-10-15T12:34:56Z, ttl 120s)
State:   running
Phase:   plan
Next:    Implement the adapter
```

### 退出码约定（脚本分支用）

| 码 | 含义 |
|---|---|
| 0 | 成功 / 204 无任务（输出 `no-task`） |
| 1 | 参数错误或 HTTP 5xx/其他 |
| 3 | 401：token 无效（提示重新获取并更新环境变量） |
| 4 | 404：lease 已失效（heartbeat/release），需要重新 claim |

网络错误自动重试 3 次（指数退避 1s/2s），仍失败按退出码 1 报 `network error after 3 attempts`。HTTP 状态错误（401/404 等）不重试，立即按语义处理。

## 3. 一条真实可复制的 curl 链路

以下四步可在拿到 agent token 后完整走通（`leaseId` 从 claim 响应取）：

```bash
URL=http://127.0.0.1:38303
TOKEN=$ROAMBENCH_AGENT_TOKEN

# 1) claim：领取一个 dispatch-ready 任务（无可领返回 204 空 body）
curl -sS -X POST "$URL/api/agent/v1/claim" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"runtimeId":"hermes-ian","agentLabel":"hermes"}'
# => {"taskId":"task-c6-hermes","leaseId":"claim-01H...","leaseExpiresAt":"...","leaseTtlSeconds":120,
#     "title":"...","goal":"...","state":"running","currentPhase":"plan",
#     "requiredArtifacts":[...],"missingEvidence":[...],"nextStep":"..."}

# 2) heartbeat：续期到 now+120s
curl -sS -X POST "$URL/api/agent/v1/heartbeat" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"leaseId":"claim-01H..."}'
# => {"leaseId":"claim-01H...","taskId":"task-c6-hermes","leaseExpiresAt":"..."}

# 3) artifact：提交阶段证据（触发 runbook 自动推进）
curl -sS -X POST "$URL/api/agent/v1/artifact" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"taskId":"task-c6-hermes","artifactKind":"test_result","outcome":"pass","label":"Tests","value":"go test ./... green"}'
# => {"ok":true,"currentPhase":"..."}

# 4) release：完结任务
curl -sS -X POST "$URL/api/agent/v1/release" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"leaseId":"claim-01H...","outcome":"done"}'
# => {"leaseId":"...","taskId":"task-c6-hermes","outcome":"done","dispatchState":"done","leaseExpiresAt":"..."}
```

## 4. 故障恢复

**lease 过期（最常见）。** TTL 120s，超时后 server 把任务拨回 `ready`、lease 行置 `expired`。表现：

- 循环 heartbeat 收到 HTTP 404 并退出（退出码 4），消息为 `lease expired or no longer active (HTTP 404)... re-claim`
- 继续对旧 leaseId heartbeat/release 同样 404

恢复流程：**直接重新 claim**。任务回到 `ready` 后会被（可能不同的）runtime 重新领走：

```bash
roambench-agent claim --server $URL --runtime-id hermes-$(hostname) --agent-label hermes --json
# 拿到新的 leaseId 后重续 heartbeat、继续任务；已提交的 artifact 不受影响
```

预防：heartbeat 循环间隔 ≤ 60s（TTL 的 50%），对网络抖动留足重试余量；Hermes 长思考步骤前先手动 heartbeat 一次。

**进程崩溃 / Hermes 重启。** lease 留在 server 侧直到 TTL 过期（server 启动时也会清扫一次），期间其他 runtime 抢不走（部分唯一索引保证一任务一个 active claim）。重启后不要复用内存里的旧 leaseId，先 heartbeat 验证——404 就重新 claim。

**token 失效（401）。** 说明 server 状态里的 token 被人重置过。重新走第 1 节拿 token，更新 `ROAMBENCH_AGENT_TOKEN`，重试即可；任务与 lease 不受 token 更换影响。

**网络中断。** CLI 已内置 3 次指数退避重试；仍失败时任务不会丢——只要 lease 未过期，恢复网络后继续 heartbeat 即可。中断超过 TTL 就按"lease 过期"处理。

**误 release 成 failed。** 无需补救：任务回 `ready`，重新 claim 继续做，Timeline 里会保留完整记录供审计。

## 5. API 参考

| Endpoint | Method | Auth | 说明 |
|---|---|---|---|
| `/api/project-control/agent-token` | POST | Session cookie | 获取/查看 agent token |
| `/api/agent/v1/claim` | POST | Bearer | 领取下一个 dispatch-ready 任务；204 = 无任务 |
| `/api/agent/v1/heartbeat` | POST | Bearer | 续约；404 = lease 失效 |
| `/api/agent/v1/release` | POST | Bearer | 释放，`outcome` 必须 done/failed/released |
| `/api/agent/v1/artifact` | POST | Bearer | 提交阶段证据，满足要求时 runbook 自动推进 |
| `/api/agent/v1/checkpoint` | POST | Bearer | 请求人工介入 |
| `/api/agent/v1/task` | GET | Bearer | **已废弃（410 Gone）**，改用 claim |

claim 请求体 `{runtimeId, agentLabel}`；`agentLabel` 取值固定 `kiro-cli|claude|codex|opencode|hermes`，Hermes 固定传 `hermes`。`runtimeId` 建议形如 `hermes-<hostname>`，用于在看板/Runtimes 视图区分来源。
