# RoamBench

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

> 一个能让你在手机上接回 Codex、Claude Code 等 terminal-first coding 工作流的轻量远程工作台。

- 用 `tmux` 保住 terminal 会话
- 支持 `2 / 4` 分屏 workspace，适合同屏并行跑 Codex、Claude Code、Kimi-CLI、OpenCode 等工具
- 在任何地点以较低开销发起、盯住、接回长时间运行的任务，并顺手做轻量文件修改

RoamBench 是一个面向单人自托管场景的轻量远程工作台。

`RoamBench` 是对外产品名。当前仓库名、二进制名、配置名和环境变量名暂时仍然使用 `liteterm`，直到后续完成代码层改名。

你可以把它理解成 SSH 和完整浏览器 IDE 之间“刚刚好”的那一层：无论在电脑还是手机上，都能进入自己的机器，接回 terminal，会看文件、复制文件、做小修改，并发起或盯住长时间运行的任务，而不必背上一个很重的浏览器 IDE。

它特别适合这些场景：

- `vibe coding` 和 agent 驱动开发
- 同屏挂起 Codex、Claude Code、Kimi-CLI、OpenCode 等长期 CLI 工作流
- 远程脚本、数据任务和长时间运行的 CLI 工作
- 离开工位后查看进度、补一刀小修改
- 通过 terminal 直接指挥 `openclaw` 这类命令行工具，而不是依赖一个很重的桌面 IDE

它把这些能力放在一个很轻的应用里：

- 基于 `tmux` 的持久化终端会话
- 支持 `1 / 2 / 4` 布局、可跨设备同步的 workspace 视图标签，其中 `2 / 4` 分屏尤其适合同屏并行任务
- 内置文件浏览器、文本编辑器和图片查看器
- 支持复制、重命名 / 移动、上传、下载等轻量文件操作
- 更完整的编辑器保护能力和大目录导航辅助
- 保存在浏览器本地的语言、字体、颜色和布局设置
- 整体轻量、低开销，接回任务和恢复视图都很快

## 为什么做这个项目

RoamBench 参考了 `rstudio-server` 这类远程工作环境的思路，但做了非常激进的精简。

目标不是把桌面 IDE 完整搬进浏览器，而是只保留真正高频、真正值钱的能力：

- terminal
- 文件访问
- 快速编辑
- 会话恢复
- 能跟着你跨设备走的 workspace 视图

这个取舍在手机和小屏设备上尤其重要。完整浏览器 IDE 往往会变得又重又别扭，而 RoamBench 更适合“快速进入、发起任务、观察进度、做一点修改、再退出”的工作节奏。

## 它是什么，不是什么

- 它是一个小而快、单用户、自托管的远程工作台。
- 它适合在任何地点发起任务、盯任务、必要时接管一下。
- 当 terminal 是 coding agent 或自动化任务的主控制面板时，它尤其顺手。
- 它很适合同屏挂多个 terminal-first 工具，用 `2 / 4` 分屏并行工作。
- 它不是多用户平台。
- 它不打算替代本地完整编辑器去承担重度编码工作。
- 它刻意保持有取舍的产品边界，这样界面才能持续保持轻和快。

## 截图

![RoamBench screenshot](docs/screenshot-main.png)

## 功能

- 支持 `password` 或 `pam` 的单用户认证
- 支持 IP 白名单
- 在启用 `tmux` 时，终端会话可在刷新页面、重新连接和服务重启后恢复
- 终端元数据落盘，并支持总存储上限
- 支持 `1 / 2 / 4` 布局的多窗格 workspace 标签
- terminal 在不同 workspace 中不会重复分配
- 同一 RoamBench 用户的 workspace 状态可跨浏览器同步
- 适合长期挂起多个 terminal-first 工具，例如 Codex、Claude Code、Kimi-CLI、OpenCode 等 CLI 工作流
- 支持新建文件 / 文件夹、另存为、重命名 / 移动、复制、上传、下载、删除和图片预览
- 编辑器支持草稿恢复、未保存离开提醒、查找替换、跳转到行、可选行号和更明确的保存状态
- 文件浏览器支持面包屑导航和当前目录筛选
- 每个 terminal 窗格右侧都有可见滚动滑块
- 整体轻量、低开销，在普通自托管机器上也能保持较快响应
- 顶部实时显示应用内存 / 系统已用 / 系统总内存
- 界面支持英文、简体中文、日文

## 当前设计模型

RoamBench 是一个刻意保持简单的项目：

- 一个服务进程只服务一个 Unix 用户
- 登录用户名必须等于运行 `liteterm` 的 Unix 账户
- terminal 会话状态保存在服务端
- workspace 标签状态保存在服务端，并在浏览器里做本地缓存
- UI 偏好仍保存在浏览器本地

这意味着：

- 在启用 `tmux` 时，terminal 会话可以跨刷新、断线重连和服务重启继续存在
- workspace 名称、`1 / 2 / 4` 布局和 terminal 摆放方式可以跟随同一个 RoamBench 用户跨浏览器 / 设备恢复
- 语言、字体、主题和编辑器视图偏好仍然按浏览器分别保存

## 环境要求

- Go `1.22+`
- 推荐安装 `tmux`，这样 terminal 会话可以持久化
- Linux 或其他支持 PTY 的类 Unix 系统

没有 `tmux` 也能运行，但 terminal 会话持久化能力会弱很多。

## 快速试用

这是最快的本地 / 局域网试用路径：

```bash
make build
cp configs/liteterm.quickstart.toml liteterm.toml
./liteterm --password-hash
export LITETERM_USER="$(whoami)"
export LITETERM_PASSWORD_HASH='<填入刚生成的 hash>'
./liteterm --config liteterm.toml
```

说明：

- 这条路径优先追求“几分钟跑起来”，不是安全加固版
- `configs/liteterm.quickstart.toml` 会开启不安全 HTTP，并关闭 IP 白名单
- 只适合可信的本地或局域网环境
- 如果你要正式部署，按下面的完整配置流程来

## 完整配置

1. 构建二进制：

   ```bash
   make build
   ```

2. 复制示例配置：

   ```bash
   cp configs/liteterm.example.toml liteterm.toml
   ```

3. 编辑 `liteterm.toml`：

   - 把 `[auth].single_user` 改成当前通过 `./liteterm` 启动服务的 Unix 用户
   - 设置 `[server].allowed_ips`，或者只在可信测试环境下启用 `allow_all_ips = true`
   - 检查 terminal 持久化相关配置

4. 生成密码哈希：

   ```bash
   ./liteterm --password-hash
   ```

5. 把生成出来的哈希填入 `liteterm.toml` 的 `password_hash`

6. 启动 RoamBench：

   ```bash
   ./liteterm --config liteterm.toml
   ```

7. 在浏览器中打开服务地址

## 构建与运行

```bash
make build
make run
go test ./...
```

PAM 构建：

```bash
make build-pam
```

## 升级说明

- 前端资源是内嵌进 Go 二进制里的
- 修改 [web](web) 下的前端文件后，需要重新构建并重启 RoamBench 服务，浏览器强刷本身不够
- 如果当前不是 `tmux` 模式，重启 RoamBench 服务可能会中断正在运行的 shell 和任务

## 配置

RoamBench 当前默认按这个顺序查找配置文件：

1. `./liteterm.toml`
2. `~/.config/liteterm/liteterm.toml`
3. `/etc/liteterm/liteterm.toml`

也可以显式指定：

```bash
./liteterm --config /path/to/liteterm.toml
```

常用 CLI 参数：

- `--config`：指定配置文件路径
- `--host`：覆盖配置文件中的 host
- `--port`：覆盖配置文件中的 port
- `--password-hash`：从标准输入生成 `bcrypt` 哈希并退出

更多说明：

- [Roadmap](docs/roadmap.md)
- [Launch Playbook](docs/launch-playbook.md)
- [GitHub Release Checklist](docs/github-release-checklist.md)
- [Lightweight Evidence](docs/lightweight-evidence.md)
- [Rebrand Checklist](docs/rebrand-checklist.md)
- [Deployment Hardening](docs/deployment-hardening.md)
- [Configuration Guide](docs/configuration.md)
- [Authentication Guide](docs/authentication.md)
- [Contributing](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)
- [GitHub Release Copy](docs/github-release-v0.2.0.md)
- [GitHub Publishing Notes](docs/github-publishing.md)
- [Changelog](CHANGELOG.md)
- [License](LICENSE)

## Terminal 持久化

当系统中存在 `tmux` 时，RoamBench 会优先使用它作为 terminal 后端。

- terminal 元数据会写入磁盘
- 空闲 session 会按 `terminal.idle_timeout` 自动清理
- 元数据总量会受 `terminal.persist_max_bytes` 限制
- 默认存储目录是 `~/.local/state/liteterm/terminals`

这种方式可以在节省内存的同时，保留服务重启后的 session 恢复能力。

没有 `tmux` 时，RoamBench 仍可运行，但服务重启时对正在运行任务的保护会明显变弱。

## Workspaces

前端支持带 `1 / 2 / 4` 布局的 workspace 标签。

- 每个 workspace 都可以重命名
- 每个 workspace 都可以有不同布局
- 每个 terminal 在所有 workspace 中只会出现一次
- workspace 状态会按当前 RoamBench 用户持久化到服务端
- 浏览器里仍保留本地缓存副本，作为回退

## 文件工具

RoamBench 自带一个轻量的文件工作区：

- 浏览目录，支持排序、隐藏文件切换、面包屑导航和当前目录筛选
- 编辑文本文件，支持草稿恢复、查找替换、跳转到行和可选行号
- 新建文件 / 文件夹、另存为、重命名 / 移动、复制
- 上传和下载文件
- 在内置 viewer 中查看图片

文件浏览器的根目录是当前登录用户的 home 目录。

## 安全说明

- RoamBench 只支持单用户模式
- 默认部署建议保持 IP 白名单开启
- 非 loopback 地址下，除非显式设置 `allow_insecure_http = true`，否则 RoamBench 会要求 TLS

## 项目结构

- [cmd/liteterm](cmd/liteterm) - CLI 入口
- [internal/auth](internal/auth) - 认证与会话
- [internal/server](internal/server) - HTTP 服务与 API
- [internal/terminal](internal/terminal) - terminal 会话管理
- [internal/filebrowser](internal/filebrowser) - 文件浏览器后端
- [web](web) - 内嵌前端资源

## 当前状态

这个项目已经适合单用户自托管场景使用，尤其适合 terminal 优先的编码、agent 协作和轻量远程接管，但整体仍然保持小而明确、偏工程化的取舍。
