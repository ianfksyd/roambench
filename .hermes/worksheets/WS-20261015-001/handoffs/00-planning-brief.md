# 规划输入包（Planner 用）
base_sha: 6ea5585ffdac21881739fd223eb488cb5fdc38cb

## 关键代码事实（Hermes 侦察，供规划时直接引用，不必重新发现）
1. 数据模型：internal/server/project_control.go（6182 行）—— projectControlSnapshot 含 Projects/Workstreams/Tasks/Sessions/Runtimes/Dashboard；Task 有 State/AcceptanceStatus/CurrentPhase/FilesChanged/DiffSummary/Timeline/Evidence
2. API 路由注册：internal/server/server.go L186-196（/api/project-control*）；Agent API v1 已有 bearer token 鉴权
3. 前端：web/js/project-panel.js（4738 行，vanilla JS IIFE，无框架）；视图=dashboard/workstream/task 详情；TASK_DETAIL_TABS=[overview,timeline,evidence,files_diff,sessions,audit]
4. 终端层：internal/terminal/manager.go + tmux；OSC 通知解析器已有
5. 持久层：internal/controlplane/ SQLite（control-plane.sqlite 在 persist 目录）
6. 产品边界（docs/project-control-discussions/hermes-agent-comparison-v0.1.md）：RoamBench=agent-neutral 控制面；Hermes=可插拔 worker runtime。不要做 Hermes 替代品
7. 本机 coding agent：kiro-cli 2.29.1 / claude 2.1.300 / codex 0.162.1 / opencode 2.0.27（均有非交互模式，见多Agent流水线方案 README）
8. 构建前置：本机无 Go 工具链，go.mod 要求见文件头；Makefile 在仓库根

## 两个功能的用户验收口径
F1 看板：用户打开浏览器就能看到——每个项目一行/一卡：当前任务与阶段、最近活动、git 分支状态（当前分支、领先/落后、未提交变更、活跃 worktree）。数据来源可以是 server 侧执行 git 命令读真实仓库状态，不只是 Project Control 里手工录入的状态。
F2 Hermes 接手：RoamBench 的 Agent API v1 已预留（task/artifact/checkpoint/interactions）。需要：a) 一个 Hermes runtime 适配（server 侧能启动/监督 hermes 会话或识别其回调）；b) agent 能力探测——探测本机安装的 coding agent（版本、可用模型），据此在 UI 推荐给新任务（如"检测到 codex 0.162.1 + kiro 2.29.1，建议 T2 任务用 glm-5 worker + sol 审查"或简化为按 agentLabel 推荐）；c) 用户在 checkpoint 处批准后任务可被 Hermes 领走执行。

## 约束
- 遵循项目显式非目标：不做多用户、不做通用 IDE、agent-neutral（Hermes 只是 runtime 之一）
- 前端无框架（vanilla JS），新增 UI 遵循 project-panel.js 既有模式
- SQLite 迁移遵循 internal/controlplane/migration.go 既有模式
- 单用户产品；全部走现有 authRequired 中间件
