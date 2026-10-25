# 任务工作台：审查结论与实施方案

日期：2026-10-25  
状态：P0a、P0b 核心、P1 进展卡、P3 分支/综合接口已实施（P4 流程视图未做）——`internal/localruntime`（Claude `-p --output-format stream-json` 适配器：真实退出码完成判定、事件解析、进程组 Stop、观察窗转写）替代 send-keys/pane-guess 路径；worker 心跳丢失即停执行器；`permission_mode` 可配（默认 acceptEdits，绕过参数须部署者显式选择）。P0b 其余（execution 表、重启对账、暂停协议）与 P1–P4 未实施。

## 1. 产品目标与审查结论

用户希望在 Project Panel 完成日常工作管理：看到最新进展、回答问题、开始/暂停/取消任务、派生审查与并行路线，并把多个成果交给一个新任务综合处理。Terminal 保留完整交互和排障能力。

已确认的展示要求：

- Task info 的 State / Current phase / Next step / Last update 用“最新进展”取代。
- 默认显示最近一条有效消息的 3–5 行，支持展开；默认不调用模型生成摘要。
- 不再重复显示 Goal、Self-report、Recent activity、机器接口地址和无内容的占位卡。
- 用户直接发起工作不重复要求派发审批；AI 发起的授权请求保留明确的人类决策。
- 面板内支持 Yes / No / 单选 / 多选 / 文字回答，但每次回答必须绑定一个有效问题。
- RoamBench 启动的 agent 工作自动登记任务；普通 shell 不自动刷成任务。
- 支持执行中增加分支、对已完成成果创建综合任务，以及可拖动的流程视图。
- 所有面板隐藏可见滚动条，保留滚轮、触摸、键盘滚动；流程图支持平移、缩放。
- Agent Setup 选择执行器，默认 Claude；不得把当前会话的选择静默改成全局默认。

审查后的实施顺序：执行可靠性 → 最新进展 → 结构化问答 → 任务登记及分支关系 → 流程视图及受控并发。不要先增加并发再补执行事实，也不要把依赖图和低代码编辑器一次性交付。

## 2. 当前基础与必须纠正的判断

下列基础可复用，但尚不组成可靠闭环：

| 现有入口 | 可复用部分 | 当前缺口 |
|---|---|---|
| `cmd/roambench-worker/main.go` | 领取、心跳、tmux 启动 | 用 shell 名称判断结束，未可靠取得退出码；停止和恢复未闭环 |
| `internal/server/agent_terminal.go` | Manager 正式终端及任务绑定 | 应要求有效租约并验证任务归属；绑定失败不能吞错 |
| `internal/controlplane/claims.go` | SQLite 独占租约、过期、释放 | 需要执行实例 fencing，拒绝旧实例覆盖新执行 |
| `internal/server/project_lifecycle.go` | 任务依赖、风险、暂停和接管策略 | 人发起任务可在 manual 项目领取，但撤销策略仍按 manual 全量撤销 |
| `internal/server/project_takeover.go` | 验收与释放 | 当前无验收命令路径放宽了已关闭租约结果接收，需要恢复严格校验 |
| `internal/controlplane/repository.go`、`internal/server/interactions.go` | 问题、版本、选项、幂等回答及审计 | 未等价于捕获并回答 Claude 当前问题；缺执行器适配闭环 |
| 任务 `DependsOn` | 全部依赖满足后可运行 | 缺成果引用、版本固定、分支入口及图编辑 |
| `web/js/project-panel.js`、`web/js/app.js` | 任务卡与终端桥接 | 新建终端发现、详情弹窗绑定和跳转需浏览器验证 |

### 2.1 租约问题的具体证据与待验证点

`revokeClosedClaims` 当前条件为：任务无 PauseReason 且项目既非 manual 也非 paused 才跳过撤销。因此 manual 项目的 active lease 会被撤销。这与 `taskClaimAllowed` 对人发起任务放宽接管条件的规则直接冲突。

必须新增“manual 项目、人发起、刚领取”回归，覆盖领取后所有会触发保存、撤销和监控的路径，定位实际触发者。此前把 409 归因于未证实的“GetClaim 竞态”，随后放宽租约检查，不能作为已验证结论延续。

修复应统一授权判定，保留读取租约、有效期、执行归属检查。不得通过接受任意 released / expired lease 的完成报告掩盖问题。

### 2.2 成本与状态口径

- 共享记忆减少重复探索和整理，不会让不同模型免读上下文，不保证跨供应商提示缓存共享。
- “终端片段”零额外模型 token，但仍有采集、清洗、存储和网络开销；它不一定是有效进展。
- `running` 字段、存活进程或存在 tmux 会话，都不能单独证明任务正在工作。
- 执行器正常退出表示一次执行结束，不自动等于验收通过。
- 不从 Kiro 缺少某个子命令推导整个产品永不支持自定义端点；适配能力按版本和官方契约记录。

## 3. 统一模型与状态契约

### 3.1 四种对象分别管理

| 对象 | 含义 | 生命周期 |
|---|---|---|
| Task | 一个工作目标 | 可以重试、派生、引用多个成果 |
| Execution | 一次具体执行 | 独立 executionId、generation、租约、进程和结果 |
| Terminal | 用户可查看/交互的终端 | 可以跨执行恢复；不得单独用来推导任务完成 |
| Artifact | 一份版本化成果 | 内容摘要、文件引用、哈希、来源执行和验证状态 |

Task 与 Execution 一对多；Execution 可关联一个或多个 Terminal。Terminal 可以先后承载不同任务，每次绑定必须保存时间范围与执行 ID。

### 3.2 拟议 SQLite 表

新增版本化迁移，不重写现有 JSON 项目状态存储：

- `task_executions`：execution_id、username、task_id、generation、lease_id、executor、adapter_version、terminal_id、workspace、state、desired_state、row_version、started_at、heartbeat_at、ended_at、exit_code、result_kind、error_code。
- `execution_events`：execution_id、sequence、event_id、event_type、source、payload、created_at。唯一键防重复；消息与控制确认复用同一有序事件流。
- `task_artifact_refs`：target_task_id、source_task_id、artifact_id、artifact_revision、content_hash、purpose。
- `task_graph_layouts`：username、project_id、node_id、x、y、layout_version。布局不改变执行语义。

`UNIQUE(username, task_id, generation)`；每个任务最多一个未终结 execution。事件 sequence 由服务端分配，客户端游标增量读取。

现有 Interaction 增加 executionId / generation 关联（通过正式迁移或经校验的关联表）；不要只把关键关联塞进未经校验的 UIHints。

### 3.3 Execution 状态

`queued → starting → running ↔ waiting_input → succeeded | failed | cancelled | interrupted`

另有控制过程 `pausing → paused → starting/running` 与 `cancelling → cancelled`。只在操作系统确认对应动作后显示 paused/cancelled。能力不足时显示明确失败或“不支持”，不可仅改 UI 状态。

- Task 卡显示状态由最新 execution、依赖和有效问题投影；取消任务单独显示“已取消”，不再映射为暂停。
- 成功退出且无验收命令：`execution_complete`，验证状态为 `unverified`，不能写 AcceptancePassed=true。
- 有验收命令：保留原验证链；失败或输入变化按既有规则处理。
- 多次执行中，只有当前 generation 可以发布最终结果。旧执行的事件可留审计，不改变当前任务。

## 4. P0：执行可靠性（首先实施）

### 4.1 修复领取和租约策略

1. 把领取、心跳、终端创建、artifact、回答转发、释放、撤销统一到一个授权判定函数：用户名、taskId、executionId、generation、leaseId、有效期、暂停取消状态。
2. 人发起任务的自动派发按服务端认证来源判定；客户端传入 origin 只能作为描述，不能单独授予权限。旧任务从 task_created 审计识别人类来源；来源不明不自动扩大授权。
3. 修复 manual 项目下撤销租约与允许领取的矛盾。显式暂停/取消/停止仍立即关闭旧执行的写入口。
4. 移除“租约已关闭仍算验收通过”的宽容分支。相同执行的重复结果返回原结果；过期、新 generation、错误任务一律拒绝。
5. 保持序列：关闭执行入口 → 停止/暂停进程 → 确认结果 → 终结租约。JSON 与 SQLite 更新失败时提供可重试投影，不把 SQLite 副作用埋在会失败的 JSON 保存回调里。

### 4.2 进程与 tmux 启动

引入 `internal/localruntime` 管理执行生命周期，CLI worker 只处理配置、信号与轮询：

- 为每次执行创建 0700 私有目录，提示词、状态文件权限 0600，不在命令行暴露 token 或完整敏感提示词。
- 以参数数组启动执行器，shell 包装仅负责进入指定 cwd、建立进程组、捕获退出码并原子写入结果文件。对路径/参数统一编码，tmux 使用 literal 输入或固定脚本路径，避免任务文本被解释为 shell 或 tmux 按键。
- 用显式启动确认和结果标记取代 `pane_current_command == bash` 的完成判定。
- tmux 使用独立 socket/可识别归属，记录精确 session/pane ID；清理仅针对本次执行，不按名称排除列表批量杀会话。
- Claude 首先实现会话级的结构化消息与回合完成判定。具体 CLI/SDK 协议先按安装版本查官方资料并做最小验证；交互 CLI 保留 TTY 兼容路径，不假定聊天完成后进程会退出。
- 若采用子进程结构化协议，仍将可读事件写入可查看的 Terminal；Panel 与 Terminal 展示同一执行，不出现两份互不关联的日志。

### 4.3 暂停、取消、恢复

- 暂停优先用执行器支持的停止/挂起协议；本机进程组 SIGSTOP/SIGCONT 作为明确标注的能力（不保证远端模型请求也暂停）。暂停期间租约由 worker 续约，避免另一个 worker 重领。
- 取消发送中断，给短暂退出窗口，再按归属终止进程组；确认进程结束后标记 cancelled。保留日志与成果，不删除工作目录。
- worker SIGTERM：停止领取，持久化当前执行，并按配置交接或中断；不留下无人负责的活进程。
- 重启对账：核验进程身份、tmux pane、执行目录及 generation；可恢复则续接，否则标 interrupted。只允许用户明确重试或可靠的重试策略启动下一代。
- heartbeat 失败必须处理：失去租约时关闭本执行的写入口并停止/隔离执行，不继续无限运行。

### 4.4 配置闭环

Agent Setup 增加“执行器”与“启用本地执行”设置，默认 Claude，并显示已安装/可运行/协议能力。设置持久化到用户 runtime 配置，worker 每次领取前读取生效；当前执行固定配置快照。

统一 CLI、服务和 UI 的配置来源。目前 worker 并未实际读取 `[agent.worker]`，不能继续在文档承诺它已生效。移除硬编码用户名和安装目录，使用独立 0600 EnvironmentFile 存凭据，单元文件不内嵌 token。权限策略由用户明确选择，不预设绕过参数。

### 4.5 P0 验收

- 网页建任务后先显示排队/启动中，得到执行确认才显示运行中。
- manual 项目的人类任务领取后，保存/监控/创建终端不撤销其合法租约。
- `/bin/false`、不存在的执行器、启动失败、丢失 pane 均不会显示成功。
- 执行器一次回答结束、等待用户、整个会话退出能区分。
- 暂停/继续/取消用真实有子进程的夹具验证，进程与界面状态一致。
- 重启 worker、重启服务、网络断开、租约过期、重复结果和陈旧结果均覆盖。
- 无验收命令不自动标记验证通过；release 409 等失败不再打印“released … done”。

## 5. P1：最新进展卡（默认零额外模型 token）

### 5.1 消息来源和采集

新增消息适配层：优先采集执行器结构化 assistant message、progress、question、turn-complete、error 事件；保留 source 与 adapterVersion。CLI 不支持时读取指定 pane 的有限尾部作为 `terminal_fragment`。

兼容采集只清洗 ANSI/OSC、控制字符和重复重绘；不推断思维过程、不把工具原始日志冒充最终结论。限制单次行数/字节数、频率和每执行保留量，不采集其他 pane。敏感字段在持久化前脱敏；完整终端只通过既有登录会话访问。

执行事件推动 SSE 更新，断线以 sequence 重连；无事件接口时退化为有上限的轮询。前端只更新当前卡片，避免重建 Terminal 或整个看板。

### 5.2 拟议接口与界面

- `GET /api/project-control/tasks/{id}/progress?after={sequence}`：最新消息、来源、截断标记、执行状态、有效问题和终端引用。
- `GET /api/project-control/executions/{id}/events?after={sequence}`：SSE 或分页事件；沿用登录鉴权和 BasePath。

卡片仅显示状态点、最近消息（3–5 行）、必要时“展开”和“终端”。无真实进展时显示“等待执行器启动/等待回答”，不展示内部 API 地址。用户滚动查看旧内容时不强制跳到最底部；所有滚动条隐藏但键盘可达。

摘要默认关闭。若后续启用，缓存键包含 executionId、消息范围及哈希，只在用户请求或配置触发点生成；返回文本必须标为 AI 摘要。

### 5.3 P1 验收

长中文、长 URL、代码、多行错误在窄屏不横向撑大；展开不改变对话框宽度。正常进展不增加模型调用，消息重复推送不重复显示。浏览器测试验证滚动条隐藏且内容仍可访问，以及新终端发现和点击跳转。

## 6. P2：Panel 内回答（先 Claude）

### 6.1 结构化问题契约

每个问题包含：questionId、taskId、executionId、generation、vendorRequestId、questionType、原文、options、状态、expiresAt、rowVersion、inputHash。

问题类型：yes_no、single_choice、multi_choice、text、permission。目录信任、工具权限和一般业务选择分别处理，不笼统转成 Approve/Reject。

复用现有 Interaction / RespondInput / 幂等键及审计。实际 CLI 命令与 hook/SDK 能力必须先验证；此前计划中举例的 `checkpoint --options` 不视为已存在接口。

### 6.2 回答闭环

1. 执行器适配器收到一个真实问题，登记对应 Interaction，任务投影为等待回答。
2. Panel 展示原问题与有效选项；权限问题同时显示操作对象和范围。
3. 用户点击，POST 携带 executionId、expectedRowVersion、inputHash、idempotencyKey。
4. 服务校验当前 generation、问题仍 pending、选项有效、未过期，持久化回答并产生待交付事件。
5. worker 仅向相同 vendorRequestId 的等待请求交付一次，收到执行器确认后标 answered；传输重试不会再次批准。
6. 提问已变化、执行已停止、重复或过期回答返回冲突并刷新。取消问题不等于批准。

优先通过执行器正式结构化通道回答。无法核实问题身份时显示“到终端处理”，不靠解析终端字符串猜测问题，不盲发方向键/Enter。用户主动输入的终端文本仍沿用既有交互入口。

### 6.3 P2 验收

Yes/No、单选、多选、文字、工具权限各有夹具；双击、重连重发、陈旧问题、换执行、选项被修改等负例全部覆盖。浏览器实测用户只在 Panel 回答即可继续对应执行，审计保留原问题与回答，权限拒绝不会被转成批准。

## 7. P3：终端登记、分支和成果汇合

### 7.1 登记规则

- 受管 runtime 启动新目标：先建 Task/Execution 再建 Terminal，自动成卡。
- 普通 shell：提供“作为任务管理”入口，用户给目标和项目；不把每个 tab 自动变成任务。
- 外部 agent：通过 adapter/hook 显式 register 上报，认证并验证目录及会话归属；无法确认时只提示登记。
- 恢复原目标：绑定新的 execution，不创建重复任务。
- 复制某终端成果进行 Review：创建新 task，记录 source execution/artifact 引用；不得复制整段聊天或凭终端名称猜来源。

### 7.2 两类边

- `depends_on`：前置完成及所需验证条件满足后，下游才可调度，沿用现有 DependsOn 并补循环检查。
- `uses_artifact`：引用固定版本成果，不自动改变任务启动条件。记录 purpose，例如 review / compare / synthesize。

成果至少保存来源 task/execution、artifact ID/revision、哈希、可访问路径与验证状态。无自动验收的成果可引用，但不能被误认为已验证；需要验证的依赖边仍等待验证条件。

### 7.3 拟议事务接口

- `POST /api/project-control/tasks/{id}/branches`：一次创建 N 条子目标、依赖和来源引用。
- `POST /api/project-control/tasks/compose`：选择两个或更多成果，创建新的综合任务，保留来源原样。
- `PATCH /api/project-control/tasks/{id}/dependencies`：带版本的依赖修改与循环校验。
- `POST /api/project-control/terminals/{id}/register-task`：现有终端登记；不重复启动执行器。

创建关系与任务记录必须原子或可恢复，不出现半条分支。对 running 节点改变依赖不追溯修改当前 execution 的输入，只影响显式下一次执行。

### 7.4 工作目录与并行写入

写分支默认独立 git worktree，记录 base commit、branch、worktreePath、写入权限和互斥键。同一路径写入只允许一个执行；只读审查可并行。不在用户脏工作树上自动 stash、reset、清理或合并。

非 git 项目使用显式串行写入或已验证的独立副本策略；界面不能显示具备未实现的 worktree 能力。合并节点首先综合成果，代码合并另做显式操作并处理冲突。

### 7.5 P3 验收

A 完成后 B/C 可就绪；D 依赖 B/C，任一失败不会误运行。选择两条已完成线索产生新综合任务，旧任务结果不变。上游重跑产生新版本不静默替换下游已固定输入。循环、跨用户引用、未知成果、重复提交、运行中改依赖有明确拒绝或版本冲突。

## 8. P4：流程视图及受控并发

先自动布局 DAG，再加轻操作，最后评估拖线编辑：

- 节点来自真实任务，连线分别表示执行依赖与成果引用；不解析聊天文字凭空生成执行关系。
- 节点显示状态与一小段最新进展；问题高亮，点开直接回答；能打开关联 Terminal。
- 操作：派生审查、派生路线、多选成果综合、开始/暂停/取消；拖动仅保存布局。
- 画布支持缩放/平移、适配窗口、键盘焦点移动及等价的列表视图；不展示滚动条。
- 后续拖线必须调用依赖事务接口，预览受影响节点，禁止循环和静默改运行中输入。

图表达依赖，不承诺同时运行：容量不足时分支显示“就绪/排队”。worker 的 `max_concurrent` 初始为 1，P0/P3 验收后再开放 2+；增加按供应商/模型/项目限额及共享目录互斥。停止服务和租约丢失时必须同时对账所有执行。

P4 验收：串行/并行两种容量下图与真实执行一致；拖动不改变依赖，连线修改有版本控制；20–100 个节点可操作，窄屏有列表替代，分叉汇合及失败阻塞可读。

## 9. 实施批次、迁移与交付纪律

| 批次 | 主要代码范围 | 交付门槛 |
|---|---|---|
| P0a | claims、lifecycle、takeover、claim handler | manual 人类任务租约不被误撤；旧执行结果不能覆盖；不虚报验收 |
| P0b | 新 localruntime、worker、terminal、Agent Setup 配置 | 明确完成信号/退出码；真实暂停取消；重启恢复；UI 配置真正生效 |
| P1 | execution_events、progress API、Panel | 默认 0 额外模型调用的最新进展卡 |
| P2 | interactions、executor adapter、Panel 回答控件 | Claude 一次提问—回答—确认闭环及负例 |
| P3 | dependencies、artifact refs、登记/分支/综合 API | 版本化输入、独立目录与依赖调度验证 |
| P4 | 流程视图、布局 API、并发调度 | 图与执行一致；容量与共享目录互斥生效 |

迁移安排：

1. 开始 P0 前备份当前用户 JSON 与 SQLite（使用 SQLite backup API，不复制运行中的裸 db 文件）；记录服务/worker PID、已有 tmux 与任务关系。
2. SQLite 按 schema migration 新增 execution 表；保留已有 task/interaction ID，不重命名旧记录。路由兼容历史长 ID。
3. 旧 running 但无 execution 的任务显示“执行状态待核验”，不可自动标成功。已有会话只有在可核实进程身份和租约时接管，否则提供明确恢复操作。
4. 旧放宽 release 逻辑停止接受陈旧执行结果；需要重试的任务显式创建新 generation。保护现有成果和真实运行会话。
5. 按功能开关部署，先临时实例与临时 workspace，再本机单任务，再有隔离目录的双分支；部署时不能为了测试杀用户其他会话。
6. 每批独立评审与提交，执行适配器版本写入结果。公开仓库推送前运行既有 secret scan，测试夹具无真实凭据。

验证输出保留真实退出码，禁止 `go test | tail/grep` 掩盖失败。建议运行 `go test ./internal/... ./cmd/...`、相关 runtime `go test -race`、`node --test` 与浏览器脚本，记录失败与跳过。测试不读写真实 HOME、不创建真实工程任务、不自动回答用户权限请求。

## 10. 第一批可直接开工的任务单（P0a）

> 状态：**已实施**。租约策略矛盾已修复（`revokeClosedClaims` 按"执行入口关闭"判定，
> manual 项目的人发起任务租约不再被误撤）；宽容 release 分支已删除，恢复严格
> task/execution/lease 校验（重复/未知/过期租约 → 404）；无验收命令完成拆分为
> "执行成功 + 验证未完成"（`AcceptancePassed` 保持 false、`not_ready`，依赖边按
> 命令有无区分放行）；claimed-但无租约的存量任务快照投影为 `unverified`（不落盘）；
> JSON 保存失败注入测试证明领取干净失败且可恢复重试。回归见
> `internal/server/p0a_claim_strategy_test.go`。

1. 新增回归：manual 项目人类任务领取后调用终端绑定/状态保存/健康监控，断言 lease 仍 active；加显式 pause/stop 对照组。
2. 追踪 `revokeClosedClaims` 调用链，统一它与领取策略；删除宽容 release 分支，恢复严格 task/execution/lease 校验。
3. 把“无验收命令”完成分为执行成功与验证未完成，新增失败退出、重复 release、过期 release、旧 generation release 测试。
4. 为旧 `running` 状态提供“未核验”投影，避免界面继续宣称运行；校准取消状态独立标签。
5. 完成真实 SQLite + JSON 保存失败注入测试，证明租约与任务投影可恢复，再进入 P0b 的执行器重构。

完成定义：这些行为有回归和失败注入证据，旧工程与全局配置无测试副作用；不能仅以“服务 active / tmux 存活 / 看板 Running”验收。
