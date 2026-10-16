# I1：会话数据与本地命令

```sh
roambench ls
roambench ls --stale 2h
roambench ls --json
roambench ls --stale 2h --json --config ./roambench.toml
roambench attach rb-schema
roambench attach --config ./roambench.toml rb-schema
```

命令使用当前 Unix 用户、服务端相同的配置查找规则和 tmux socket 环境。
`ls` 只读持久化会话元数据（包括尚未合并的 WAL）并探测 tmux；不会创建
Manager、恢复会话、触发清理或修改 tmux 配置。项目列显示 `projectId`。
`attach` 对名称作不区分大小写的子串匹配，只有唯一命中且有存活 tmux 会话时，
才打印可复制的 `tmux attach-session -t '=lt-…'` 命令；不直接执行。
多重命中、无命中、已退出或纯 PTY 会话返回非零退出码。

JSON 输出是数组，保留 `id/name/createdAt`，增加 `taskId/projectId`（关联时）、
`lastActivity/idleSeconds/status/backend` 和可用时的 `detachedAt`。
空结果是 `[]`。`--stale` 要求正时长，严格筛选空闲时间大于给定时长的会话。
状态依优先级判定：

- `exited`：tmux 会话不存在、所有 pane 已退出，或纯 PTY 进程已结束。
- `detached`：最后一个客户端断开超过 24 小时，且尚未重新连接。
- `idle`：最后活动时间距今超过 2 小时。
- `active`：其他存活会话。

活动时间合并服务端已有终端 I/O 时间与 tmux session/window activity，包含
未连接客户端时的后台输出。纯 PTY 使用持久化 PID 探测存活，I/O 时间由服务端
记录，按现有周期刷新。运行中不代表一定有 CPU 工作，只表示未达到以上阈值。

新建、恢复的受管 tmux 会话通过独立索引的 client-attached/client-detached
钩子记录实际分离时间；旧会话没有记录时从安装钩子开始计时，不将最后连接时间
冒充断开时间。`ls` 也列出 `lt-` 前缀的孤立 tmux 会话，名称回退到 ID。
已退出会话只在元数据仍被保留时可见，仍遵循已有服务端清理策略。

## 创建会话

已鉴权的 `POST /api/terminals` 可接收以下 JSON，空请求体仍兼容：

```json
{"name":"rb-schema-review","taskId":"task-1","projectId":"project-1"}
```

名称复用 `normalizeSessionName`，支持传入 slug；关联字段随元数据持久化并恢复。
项目阶段会话创建时自动携带既有 task/project ID。

## 目录选择 API

已鉴权的 `GET /api/fs/list?path=/absolute/path/inside/home` 返回：

```json
{"path":"/home/ian","directories":["Documents","Projects"]}
```

`path` 省略或为空时使用服务端 home。只返回按名称排序的直接子目录名（含指向
home 内目录的软链），不返回文件和越界软链；空目录返回空数组。
相对路径、不存在的路径、文件、home 之外路径及软链逃逸均返回 403。
检查同时覆盖词法边界和 `EvalSymlinks` 后的真实边界。
