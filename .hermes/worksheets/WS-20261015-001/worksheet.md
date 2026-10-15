# WS-20261015-001: RoamBench 项目看板 + Hermes 自动接手

## 需求（用户原话）
1. 项目看板：了解每个项目的进展、分支情况
2. Hermes 自动接手能工作，且能根据机器上现有的 coding agent 推荐

## 现状盘点（Hermes 实地侦察）
- Go 单二进制 web 应用；tmux 终端后端；vanilla JS 前端（project-panel.js 4738 行）
- 已有 Project Control 面：Project/Workstream/Task/Session/Runtime/Skill/Runbook/Tool/Artifact/Checkpoint/Decision/Memory，SQLite 持久化（control-plane.sqlite）
- 已有 Agent API v1（bearer token）：/api/agent/v1/task、artifact、checkpoint、interactions —— Hermes 可经此接入
- 已有工具注册表（repo_status/diff_capture/go_test + 自定义工具）与 runbook 自动推进引擎
- docs/project-control-discussions/hermes-agent-comparison-v0.1.md 已明确边界：Hermes=可插拔 worker runtime，RoamBench=agent-neutral 控制面
- 已有 Telegram 通知、OSC 终端通知、agent activity 视图
- 尚无：git 分支/worktree 状态展示、Hermes runtime 适配、agent 能力探测与推荐
- 构建前提：本机当前无 Go 工具链（需装 Go ≥1.27 或按 go.mod 要求）

## 状态
- phase: PLANNING
