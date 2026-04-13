# RoamBench

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

> 让 AI coding session 一直跑着，你随时都能从别的设备接回。

- 用 `tmux` 保住 terminal 会话
- 支持 `2 / 4` 分屏 workspace，适合同屏并行跑 Codex、Claude Code、Kimi-CLI、OpenCode 等工具
- 不用拖进一个完整浏览器 IDE，也能从别的设备接回任务、看输出、顺手做小修改

RoamBench 是一个面向单人自托管场景的轻量远程工作台。

`RoamBench` 是对外产品名。配置和启动命令请按你的本地二进制与配置路径替换为实际名称。

你可以把它理解成 SSH 和重型浏览器 IDE 之间“刚刚好”的那一层：无论在电脑还是手机上，都能进入自己的机器，接回长时间运行的 terminal，会看文件、复制输出、做小修改，而不必背上一个很重的远程开发栈。

## 为什么大家会打开它

- 在别的设备上接回 Codex / Claude Code / Kimi-CLI / OpenCode 的会话，手机也能用
- 用 `2 / 4` 分屏同时盯多个 terminal-first agents
- 远程看脚本、数据任务和长时间 CLI 工作的进度，而不是一直守着 SSH
- 离开工位后看文件、复制结果、补一刀轻量修改
- 通过 terminal 直接指挥 `openclaw` 这类工具，而不是先打开一个重型 IDE

## 为什么不是 SSH，也不是浏览器 IDE

RoamBench 的边界是刻意收窄的：它只保留远程工作里最值钱的那一层，不试图变成一个完整浏览器 IDE。

| 需求 | SSH | VS Code Remote / 浏览器 IDE | RoamBench |
| --- | --- | --- | --- |
| 用手机接回同一个长时间运行的会话 | 别扭 | 能做，但太重 | 就是为这个场景做的 |
| 让 `tmux` 会话恢复这件事更省心 | 手工拼 | 不是重点 | 内建 |
| 跑 `2 / 4` 分屏的 agent / CLI 工作流 | 手工管理 | 更偏 IDE | 内建 |
| 在 terminal 旁边看文件并做小修改 | 要额外工具 | 能做，但开销更大 | 内建 |
| 单用户自托管且保持低开销 | 可以 | 通常更重 | 可以 |

它不是多用户平台，也不打算替代你本地的完整编辑器去做重度开发。它刻意保持有取舍的产品边界，这样 UI 才能持续保持轻、快、并且在手机上也能用。

## 截图

桌面端 `4` 个 terminal 的长任务 / agent 工作区：

![RoamBench screenshot](docs/screenshot-main.png)

移动端重连到同一个 workspace：

![RoamBench mobile screenshot](docs/screenshot-mobile.jpg)

## 核心能力

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
- 登录用户名必须等于运行 RoamBench 进程的 Unix 账户
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
cp configs/roambench.quickstart.toml roambench.toml
APP_BIN=<path-to-binary>         # e.g. ./roambench
APP_CONFIG=<path-to-config-file>  # e.g. ./roambench.toml
"$APP_BIN" --password-hash
export ROAMBENCH_USER="$(whoami)"
export ROAMBENCH_PASSWORD_HASH='<填入刚生成的 hash>'
APP_CONFIG=${APP_CONFIG:-roambench.toml}
"$APP_BIN" --config "$APP_CONFIG"
```

说明：

- 这条路径优先追求“几分钟跑起来”，不是安全加固版
- `configs/roambench.quickstart.toml` 会开启不安全 HTTP，并关闭 IP 白名单
- 只适合可信的本地或局域网环境
- 如果你要正式部署，按下面的完整配置流程来

## 完整配置

1. 构建二进制：

   ```bash
   make build
   ```

2. 复制示例配置：

   ```bash
   cp configs/roambench.example.toml roambench.toml
   ```

3. 导出你本地启动时使用的二进制和配置：

   ```bash
   APP_BIN=<path-to-binary>         # e.g. ./roambench
   APP_CONFIG=${APP_CONFIG:-roambench.toml}
   ```

4. 编辑 `roambench.toml`：

   - 把 `[auth].single_user` 改成当前通过 `$APP_BIN` 启动服务的 Unix 用户
   - 设置 `[server].allowed_ips`，或者只在可信测试环境下启用 `allow_all_ips = true`
   - 检查 terminal 持久化相关配置

5. 生成密码哈希：

   ```bash
   "$APP_BIN" --password-hash
   ```

6. 把生成出来的哈希填入 `roambench.toml` 的 `password_hash`

7. 启动 RoamBench：

   ```bash
   "$APP_BIN" --config "$APP_CONFIG"
   ```

8. 在浏览器中打开服务地址

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

1. `./roambench.toml`
2. `~/.config/roambench/roambench.toml`
3. `/etc/roambench/roambench.toml`

也可以显式指定：

```bash
./roambench --config /path/to/roambench.toml
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
- [GitHub Release Copy](docs/github-release-v0.3.0.md)
- [GitHub Publishing Notes](docs/github-publishing.md)
- [Changelog](CHANGELOG.md)
- [License](LICENSE)

## Terminal 持久化

当系统中存在 `tmux` 时，RoamBench 会优先使用它作为 terminal 后端。

- terminal 元数据会写入磁盘
- 空闲 session 会按 `terminal.idle_timeout` 自动清理
- 元数据总量会受 `terminal.persist_max_bytes` 限制
- 默认存储目录是 `~/.local/state/roambench/terminals`

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

- [cmd/roambench](cmd/roambench) - CLI 入口
- [internal/auth](internal/auth) - 认证与会话
- [internal/server](internal/server) - HTTP 服务与 API
- [internal/terminal](internal/terminal) - terminal 会话管理
- [internal/filebrowser](internal/filebrowser) - 文件浏览器后端
- [web](web) - 内嵌前端资源

## 当前状态

这个项目已经适合单用户自托管场景使用，尤其适合 terminal 优先的编码、agent 协作和轻量远程接管，但整体仍然保持小而明确、偏工程化的取舍。
