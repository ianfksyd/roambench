# GitHub Release Copy

Suggested tag:

- `v0.2.0`

Suggested title:

- `RoamBench v0.2.0`

Suggested subtitle:

- `Reconnect to Codex, Claude Code, and other terminal-first coding workflows from your phone`

## English

RoamBench is a compact self-hosted remote workbench for one person.

Public product name: `RoamBench`
Current technical identifiers: `roambench`

It is built for the space between SSH and a full browser IDE: keep terminal sessions alive, run `2 / 4`-pane split views, resume work from anywhere, inspect files, copy and edit files, and direct terminal-first tools such as `openclaw`, Codex, Claude Code, Kimi-CLI, and OpenCode from a laptop or phone.

This public release includes:

- persistent terminal sessions backed by `tmux`
- server-synced workspaces with `1 / 2 / 4` terminal layouts
- practical `2 / 4`-pane split views for running multiple terminal-first tools side by side
- unique terminal assignment across workspaces
- renameable workspace tabs
- built-in file browser, text editor, and image viewer
- `New File`, `New Folder`, `Save As`, rename / move, copy, upload, download, delete, and inline image viewing
- well suited for long-running Codex, Claude Code, Kimi-CLI, OpenCode, and similar CLI workflows
- draft restore, unsaved-change warnings, find / replace, go-to-line, and optional line numbers in the editor
- breadcrumb navigation and current-directory filtering in the file browser
- visible right-side terminal scrollbar in each pane
- lightweight, low-overhead behavior that stays fast to reconnect and resume
- password or PAM authentication
- IP allowlist support
- disk-backed terminal metadata persistence with idle cleanup and storage limits
- browser-local UI settings for language, fonts, and terminal theme
- live memory indicator in the header

Design notes:

- terminal session state lives on the server
- workspace state lives on the server and is cached in the browser
- RoamBench is intentionally single-user and opinionated
- it is optimized for terminal-first workflows, lightweight remote intervention, and mobile access rather than full IDE parity
- it favors practical split-view concurrency over a heavy all-in-one IDE model

Short release blurb:

`RoamBench is a terminal-first self-hosted remote workbench for vibe coding, split-view CLI workflows, long-running tasks, and lightweight remote edits from desktop or phone.`

Docs:

- README: `README.md`
- Chinese README: `README.zh-CN.md`
- Japanese README: `README.ja.md`
- Configuration: `docs/configuration.md`
- Authentication: `docs/authentication.md`

## 简体中文

RoamBench 是一个面向单人自托管场景的轻量远程工作台。

对外产品名：`RoamBench`
当前技术标识：`roambench`

它处在 SSH 和完整浏览器 IDE 之间：保住 terminal 会话、用 `2 / 4` 分屏同时跑多个工具、从任何地点接回工作、看文件、复制和修改文件，也可以在电脑或手机上直接指挥 `openclaw`、Codex、Claude Code、Kimi-CLI、OpenCode 这类 terminal-first 工具。

这个首个公开版本包含：

- 基于 `tmux` 的持久化 terminal 会话
- 支持 `1 / 2 / 4` 布局、可跨设备同步的 workspaces
- 实用的 `2 / 4` 分屏视图，适合同屏并行跑多个 terminal-first 工具
- terminal 在不同 workspace 中唯一分配
- 可重命名的 workspace 标签
- 内置文件浏览器、文本编辑器和图片查看器
- 支持新建文件 / 文件夹、另存为、重命名 / 移动、复制、上传、下载、删除和图片预览
- 适合长期挂起 Codex、Claude Code、Kimi-CLI、OpenCode 等 CLI 工作流
- 编辑器支持草稿恢复、未保存离开提醒、查找替换、跳转到行和可选行号
- 文件浏览器支持面包屑导航和当前目录筛选
- 每个 terminal 窗格右侧都有可见滚动滑块
- 整体轻量、低开销，接回任务和恢复视图都很快
- `password` 或 `pam` 认证
- IP 白名单支持
- 磁盘持久化 terminal 元数据，带空闲清理和存储上限
- 保存在浏览器本地的语言、字体和终端主题设置
- 顶部实时内存显示

设计说明：

- terminal 会话状态保存在服务端
- workspace 状态保存在服务端，并在浏览器里做缓存
- RoamBench 刻意保持单用户、轻量和明确取舍
- 它优先服务 terminal-first 工作流、轻量远程接管和移动端访问，而不是追求完整 IDE 等价物
- 它更强调实用的分屏并行能力，而不是一个很重的 all-in-one IDE

短版发布文案：

`RoamBench 是一个 terminal-first 的轻量自托管远程工作台，适合 vibe coding、分屏 CLI 工作流、长任务盯盘，以及在电脑或手机上做轻量远程修改。`

文档入口：

- README：`README.md`
- 中文 README：`README.zh-CN.md`
- 日文短版：`README.ja.md`
- 配置说明：`docs/configuration.md`
- 认证说明：`docs/authentication.md`
