# 实施任务卡（看板 v3.6 落地 + Hermes 全程接管）

设计定稿：design/board-v3-prototype.html（v3.6，已验收）
执行者：Astra（codex gpt-6-astra，workspace-write）
纪律：每卡独立可测；共享文件（server.go 路由注册）只加注释桩；`go test ./...` + `node --test web/*.test.js` 全绿才提交；提交用 `-c user.email=hermes@local -c user.name=hermes`。

## I1 数据与命令层（Go）
- 会话语义命名：`internal/terminal` 会话元数据加 task/project 关联字段（name 已有），创建时可带 slug
- `roambench ls` 子命令（cmd/roambench 或新 cmd）：列出会话 名称/项目/状态/空闲时长；`--stale 2h` 过滤；`--json`；`attach <模糊名>`
- 空闲检测：会话最后活动时间（tmux display-seconds 或 proc 探测），喂给状态墙（运行中/空闲>2h/已退出/已分离>24h）
- 目录列表 API：GET /api/fs/list?path=（限 home 内，供文件夹选择器；返回子目录列表）
- 共享桩：server.go 路由注册处留 `// I1 routes` 注释

## I2 项目生命周期与接管状态机（Go）
- 项目契约数据：Project 加 brief（目标理解/交付物/不做/假设）、milestones[]（名称/验收证据/状态 todo|current|done）、lifecycle 状态（已委派→侦察中→规划中→待确认→执行中·MN→待里程碑验收→已完成 | 阻塞）
- 「下一步」推导：从里程碑+任务+claim 状态算 nextAction（谁：Hermes|你，做什么）
- 接管状态机（server 侧控制权，Hermes 只能请求）：
  - 自动继续：心跳正常+验收通过+无越界 → 领取已批准下一步（无验收规则不算绿灯）
  - 暂停：验收连败 2 次/心跳失联/越界/高危请求 → 关执行入口+通知；普通失败停单任务，共享冲突/越界停项目
  - 恢复：持久化检查点续跑；无检查点由用户选重跑，禁止盲重试
  - 停止接管：停派单+撤销旧令牌+停受管进程→人工模式
- 接口：项目详情聚合接口（生命周期+里程碑+下一步+活动构成分类事件数+代码变更统计）
- 共享桩：server.go 同上

## I3 看板前端上半（web/js）
- 状态墙（28 格会话网格：状态点+会话名+相对时间+H 徽标）+ 项目分组标题点击→项目详情卡
- 项目详情卡：生命周期状态链/里程碑线/下一步行/活动构成堆叠条（事件占比·非工时）/代码变更/需处理
- 时间轴（甘特移植：分段任务条+时长标注+今日线）+ 日历范围选择（预设+月历，多日日粒度）
- 悬停轻量详情卡（200ms 防抖，证据/自述分离，触屏点击）
- i18n 沿用 PROJECT_PANEL_TRANSLATIONS（en/zh-CN/ja）

## I4 看板前端下半 + 新建通道（web/js）
- 新建：文件夹图标按钮+选择器（接 I1 目录 API）/ Git URL 入口 → 提交 → Hermes 草案卡（验收命令/out-of-scope 可编辑）→ 单勾选自动接管 → 批准并创建
- 任务抽屉四组（待批准/待领取/执行中/最近结束）+ 摘要条数字
- 字号调节 A−/A＋/重置（独立 localStorage，--board-fs）
- 全界面去黑话（领取/释放/领取有效期/检查点/产物）

## I5 集成审查
- 独立 agent（未参与 I1-I4）审 diff：契约对齐/状态机边界/安全（fs API 越界）/测试质量
