# cmux vs RoamBench 对比分析

分析日期：2026-04-19

## cmux 概览

[cmux](https://github.com/manaflow-ai/cmux) 是 manaflow-ai 开发的原生 macOS 终端应用，基于 Ghostty 的 libghostty 渲染引擎，用 Swift/AppKit 构建。YC 孵化项目，GPL-3.0 许可。

核心定位：为并行运行多个 AI coding agent 设计的本地终端。

## 核心特性

- 垂直标签栏（显示 git branch、PR 状态、工作目录、监听端口、最新通知）
- 通知环（pane 蓝色光环 + tab 高亮，提示 agent 需要注意）
- 通知面板（集中展示所有 pending 通知，点击跳转）
- 内嵌浏览器（可脚本化 API，基于 agent-browser）
- Unix socket JSON-RPC API + CLI，可编程控制 workspace/split/通知
- Claude Code Teams 集成（原生 split，不需要 tmux）
- Sidebar 元数据（status pills、progress bars、log entries）
- SSH workspace（远程机器的浏览器 pane 通过远程网络路由）
- 自定义命令（项目级 `cmux.json` 定义命令面板操作）
- OSC 9/99/777 终端转义序列检测

## 定位对比

| 维度 | cmux | RoamBench |
|---|---|---|
| 平台 | macOS only（原生 Swift） | 跨平台（Go + Web，自托管） |
| 访问方式 | 本地桌面应用 | 浏览器访问，支持手机/远程 |
| 终端后端 | libghostty（GPU 加速） | tmux + PTY |
| 会话持久化 | 仅恢复布局，不恢复进程状态 | tmux 支持完整进程持久化 |
| 多设备 | 不支持 | 核心卖点 |
| 用户模型 | 单用户本地 | 单用户远程 |
| 通知系统 | 完整（OSC 序列 + CLI + 面板） | 尚未实现 |
| 可编程 API | Unix socket + CLI | HTTP API（可扩展） |
| 浏览器集成 | 内嵌可脚本化浏览器 | 不需要（本身就在浏览器中） |
| 文件编辑 | 无 | 内置文件浏览器和编辑器 |

两者解决同一个问题域（管理多个并行 AI agent），但切入角度完全不同：cmux 是"更好的本地终端"，RoamBench 是"可远程访问的持久化工作台"。

## 值得借鉴的设计

### 1. 通知系统（高优先级）

cmux 最受欢迎的功能。RoamBench 目前没有通知机制。

可借鉴点：

- OSC 转义序列检测（OSC 9/99/777）：零配置，任何支持这些序列的 agent 自动生效
- 通知面板：集中展示所有 pending 通知，点击跳转到对应 workspace
- 视觉提示：workspace tab 上的未读 badge / 高亮
- 浏览器通知 API：RoamBench 作为 web 应用，可以用 Web Notification API 推送桌面通知

实现路径：在 `internal/terminal` 层解析终端输出中的 OSC 序列 → WebSocket 推送到前端 → 前端显示 badge + 浏览器通知。这在远程场景下比 cmux 的原生通知更有价值（手机上收到 agent 完成通知然后点进去处理）。

### 2. Sidebar 元数据（中优先级）

cmux 的 sidebar 显示每个 workspace 的 git branch、工作目录、监听端口、最新通知文本。

可借鉴点：

- 在 workspace tab 旁显示当前目录和 git branch
- 显示 agent 的运行状态（running / waiting / idle）
- 这些信息可以通过解析终端 prompt 或 OSC 序列获取

### 3. 可编程 API（中优先级）

cmux 提供完整的 Unix socket JSON-RPC API，允许外部脚本创建 workspace、split pane、向特定 surface 发送文本/按键、发送通知、设置 sidebar 状态。

可借鉴点：

- 暴露更多编程接口（如向特定终端发送命令）
- 提供 CLI 工具让用户从本地脚本控制远程 RoamBench
- 支持 agent hook 集成（类似 cmux 的 Claude Code hooks）

### 4. Agent 集成模板（低成本高价值）

cmux 提供现成的 Claude Code hooks、Copilot CLI hooks、OpenCode 集成配置。

可借鉴点：

- 提供类似的 hook 脚本模板
- 文档中加入各 agent 的集成指南

### 5. 自定义命令（低优先级）

cmux 支持在项目中放 `cmux.json` 定义命令面板中的快捷操作。

可借鉴点：

- command palette 功能，让用户快速启动常用 agent 工作流

## 不需要借鉴的部分

- 内嵌浏览器：RoamBench 本身就在浏览器里
- GPU 加速渲染：RoamBench 用 xterm.js，已经足够
- 原生桌面体验：RoamBench 的价值在于不依赖特定桌面环境

## RoamBench 的差异化优势

- 完整进程持久化（tmux 支持，cmux 重启后进程丢失）
- 跨设备访问（手机、平板、任何浏览器）
- 内置文件浏览器和编辑器
- 跨平台（Linux 服务器，不限 macOS）
- 项目控制层方向（Task / Workstream / Policy，cmux 不涉及）

## 建议优先级

1. 终端通知系统（OSC 序列检测 + WebSocket 推送 + 浏览器通知）
2. Workspace 状态元数据（git branch、cwd、agent 状态）
3. Agent 集成文档和 hook 模板
4. 可编程 API 增强
