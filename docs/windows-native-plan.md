# Windows Native App 规划文档

## 目标

将 RoamBench/LiteTerm 打包为 Windows 原生桌面应用，用户下载即用，无需 WSL。同时集成 opencode 自动安装功能。

---

## 一、当前架构

```
浏览器 ──WebSocket──> Go HTTP Server ──> creack/pty (Unix PTY) ──> Shell
```

- **后端**: Go，使用 `creack/pty`（仅支持 Unix）提供伪终端
- **前端**: 原生 JS + xterm.js，通过 WebSocket 双向通信
- **配置**: TOML 文件，支持 shell/会话/认证等设置

### 1.1 需要 Windows 适配的代码路径

以下现有代码在 Windows 上**无法编译或运行**，必须处理：

| 代码位置 | 问题 | 处理方式 |
|----------|------|---------|
| `manager.go` — `import "github.com/creack/pty"` | 不支持 Windows 编译 | Build tag 隔离 |
| `manager.go:38` — `/tmp/.roambench-bashrc` | 硬编码 Unix 路径 | 平台条件判断，Windows 用 `%TEMP%` |
| `manager.go:43-55` — bash RC 内容 | bash 专属语法 | Windows 跳过，使用 PowerShell profile |
| `manager.go:88-95` — `Session.ptyFile *os.File` | ConPTY 不是 fd，是两个 pipe handle | PTY 接口封装读写 |
| `manager.go:249` — `AttachSessionForUser` 返回 `*os.File` | server.go 直接用 `*os.File` 读写 | 改为返回 PTY 接口 |
| `manager.go:301` — `pty.Setsize()` 直接调用 | 需走接口 | 通过 PTY.Resize() |
| `manager.go:786` — `loadPersistedSessions` 无 tmux 时直接 return | 会话持久化在无 tmux 时**不工作** | 实现独立持久化逻辑 |
| `main.go:151` — `syscall.SIGTERM` | Windows 无此信号 | Build tag 或 `os.Interrupt` 替代 |
| `main.go:176-181` — `user.Current()` 安全检查 | Windows 用户名处理不同 | 适配 Windows 用户模型 |
| `server.go:56-69` — WebSocket origin 检查 | 拒绝 WebView2 的 `wails.localhost` origin | 白名单 Wails origin |
| `server.go:136` — `__ROAMBENCH_BASE_PATH__` 模板注入 | Wails 静态资源服务方式不同 | 构建时替换或 Wails handler |

---

## 二、技术方案：Wails + ConPTY

### 2.1 桌面框架：Wails v2

- 使用系统 WebView2（Windows Edge 内核），无需捆绑 Chromium
- **体积说明**：若系统已有 WebView2 则约 10-20MB；若需嵌入 WebView2 运行时则 80-150MB
- Go 后端 + 前端资源打包为单个 .exe
- 推荐使用 Wails bindings 替代 WebSocket，避免 TCP 监听触发 Windows 防火墙弹窗

### 2.2 终端层：ConPTY 替换 creack/pty

**核心差异**：
- Unix PTY = 单个双向 `*os.File`（fd）
- Windows ConPTY = `HPCON` handle + 两个单向 pipe（input/output）

**ConPTY 库选择**：使用 `UserExec/conpty` 或直接封装 `golang.org/x/sys/windows`。
> 注意：`ActiveState/termtest/conpty` 是测试工具，不适合生产使用。

**接口设计**（修正版）：
```go
// internal/terminal/pty.go
type PTY interface {
    // StartProcess 启动进程，ConPTY 需要自己创建进程，不能接受 exec.Cmd
    StartProcess(name string, argv []string, env []string, dir string) error
    Read(p []byte) (n int, err error)   // 从 output pipe 读取
    Write(p []byte) (n int, err error)  // 向 input pipe 写入
    Resize(rows, cols uint16) error
    Kill() error
    Wait() error
    Close() error
}
```

> **为什么不用 `Start(cmd *exec.Cmd)`**：ConPTY 的 `CreatePseudoConsole` 需要在进程创建时
> 就绑定控制台，无法接受已构建的 `exec.Cmd`。必须由 PTY 实现自己创建进程。

**文件结构**（均需 build tag）：
- `internal/terminal/pty.go` — 接口定义（所有平台）
- `internal/terminal/pty_unix.go` — `//go:build !windows`，封装 `creack/pty`
- `internal/terminal/pty_windows.go` — `//go:build windows`，封装 ConPTY

**重构范围**：
- `Session` 结构体：`ptyFile *os.File` → `pty PTY`
- `AttachSessionForUser`：返回值从 `(*os.File, *exec.Cmd, error)` → `(PTY, error)`
- `server.go`：所有直接读写 `*os.File` 的地方改为调用 PTY 接口
- `ResizeSessionForUser`：`pty.Setsize()` → `session.pty.Resize()`

### 2.3 认证处理

当前认证系统（密码登录、cookie session、rate limiting）是为远程浏览器访问设计的。
桌面应用方案：
- 新增 "local mode"：检测到 Wails 环境时自动跳过认证
- 保留认证代码，不删除（未来可能支持远程访问模式）

### 2.4 tmux 与会话持久化

- Windows 上 tmux 不可用，`hasTmux` 为 false
- **问题**：当前 `loadPersistedSessions` 在 `hasTmux == false` 时直接 return，无持久化
- **方案**：实现独立于 tmux 的会话状态保存/恢复逻辑
- Windows 持久化路径：`%APPDATA%\RoamBench\sessions\`

### 2.5 Shell 配置

| 平台 | 默认 Shell | RC 文件 |
|------|-----------|---------|
| Unix | `/bin/bash` 或 `$SHELL` | `/tmp/.roambench-bashrc` |
| Windows | `powershell.exe` | 跳过自定义 RC，使用系统 PowerShell profile |

`shellUsesRcFile`（manager.go:634）仅在 bash 时生效，Windows 上跳过。

---

## 三、opencode 自动安装

### 3.1 检测流程

```
启动应用
  └─ 检测 opencode 是否在 PATH 中（exec.LookPath("opencode")）
      ├─ 找到 → 检查版本，提示更新（可选）
      └─ 未找到 → 弹窗询问用户是否安装
          ├─ 同意 → 自动下载安装
          └─ 拒绝 → 跳过，正常使用终端
```

### 3.2 安装方式

1. 从 GitHub Releases（`sst/opencode`）下载 Windows amd64 预编译二进制
2. 放置到 `%APPDATA%\RoamBench\bin\`
3. 将 `bin/` 加入应用内进程 PATH（不污染系统 PATH）
4. 安装完成后在终端中可直接使用 `opencode` 命令

### 3.3 更新机制

- 启动时后台检查最新版本（GitHub API: `repos/sst/opencode/releases/latest`）
- 有新版本时通知用户，用户确认后更新
- 不强制更新，不阻塞启动

---

## 四、实现阶段

### 阶段 1：PTY 抽象层（不影响现有功能）

- [ ] 定义 `PTY` 接口（`StartProcess` 签名）
- [ ] 重构 `Session` 结构体，用 `pty PTY` 替代 `ptyFile *os.File`
- [ ] 重构 `AttachSessionForUser` 返回值
- [ ] 将 `creack/pty` 调用提取到 `pty_unix.go`，加 build tag
- [ ] 重构 `server.go` 中所有直接读写 `*os.File` 的代码
- [ ] 处理 `manager.go` 中硬编码 Unix 路径（条件编译或平台抽象）
- [ ] Linux 上回归测试，确保功能不变

### 阶段 2：Windows 平台适配

- [ ] 实现 `pty_windows.go`（ConPTY，使用 `UserExec/conpty` 或直接 syscall）
- [ ] 处理默认 shell 选择（PowerShell/cmd）
- [ ] 替换 `syscall.SIGTERM` 为跨平台信号处理
- [ ] 适配 `user.Current()` 安全检查
- [ ] 实现无 tmux 的会话持久化
- [ ] Windows 路径适配（持久化目录、临时文件等）
- [ ] Windows 上编译测试、基础终端功能测试
- [ ] **重点测试 ConPTY VT 输出**：`\r\n` 换行、TUI 程序渲染（vim、opencode）

### 阶段 3：Wails 桌面壳

- [ ] 初始化 Wails v2 项目结构
- [ ] 将 `web/` 前端迁移为 Wails 前端资源
- [ ] 用 Wails bindings 替代 WebSocket 通信（避免防火墙弹窗）
- [ ] 处理 `__ROAMBENCH_BASE_PATH__` 模板注入（构建时替换）
- [ ] 添加 Wails origin 到 WebSocket origin 白名单（如保留 WS 作为备选）
- [ ] 实现 "local mode" 跳过认证
- [ ] 窗口管理基础功能

### 阶段 4：opencode 集成

- [ ] 实现 opencode 检测与自动下载
- [ ] 实现版本检查与更新提示
- [ ] 集成到应用首次启动流程

### 阶段 5：打包与分发

- [ ] 配置 Wails 构建脚本，输出单个 .exe
- [ ] 决定 WebView2 策略：依赖系统安装 vs 嵌入（体积 vs 兼容性权衡）
- [ ] 可选：NSIS 安装包或 MSIX 打包
- [ ] 代码签名（EV/OV 证书，$200-500/年，审批需数天到数周）
- [ ] CI/CD：需要 Windows runner 或 mingw-w64 交叉编译环境
- [ ] GitHub Releases 自动构建

---

## 五、文件变更预估

| 文件/目录 | 变更类型 | 说明 |
|-----------|---------|------|
| `internal/terminal/pty.go` | 新建 | PTY 接口定义 |
| `internal/terminal/pty_unix.go` | 新建 | Unix PTY 实现（从 manager.go 提取），`//go:build !windows` |
| `internal/terminal/pty_windows.go` | 新建 | ConPTY 实现，`//go:build windows` |
| `internal/terminal/manager.go` | **大改** | Session 结构体、AttachSession 签名、所有 pty 调用点、路径适配 |
| `internal/terminal/manager_unix.go` | 新建 | Unix 特有逻辑（bashrc、信号等） |
| `internal/terminal/manager_windows.go` | 新建 | Windows 特有逻辑（PowerShell、路径等） |
| `internal/terminal/persist.go` | 新建 | 独立于 tmux 的会话持久化 |
| `internal/server/server.go` | **中改** | PTY 读写改用接口、origin 白名单、local mode |
| `cmd/roambench/main.go` | 修改 | 信号处理跨平台、用户检查适配 |
| `internal/opencode/installer.go` | 新建 | opencode 检测、下载、更新 |
| `wails.json` | 新建 | Wails 项目配置 |
| `frontend/` | 新建 | Wails 前端（迁移自 web/） |
| `cmd/roambench-desktop/main.go` | 新建 | Wails 桌面应用入口（与 CLI 入口分离） |

---

## 六、构建环境与 CI/CD

### 6.1 各阶段构建环境

| 阶段 | 构建环境 | 说明 |
|------|---------|------|
| 阶段 1（PTY 抽象） | 本台 Linux 服务器 | 纯 Go 重构，`go build` 即可，无需 Windows |
| 阶段 2（ConPTY） | Windows 机器 | 需要实际运行 ConPTY 测试终端功能 |
| 阶段 3（Wails） | Windows 机器 | Wails 涉及 CGO + WebView2 绑定，必须在 Windows 上构建 |
| 阶段 4-5（集成/发布） | GitHub Actions + Windows 机器 | 开发在 Windows，正式发布走 CI |

### 6.2 为什么不能全在 Linux 上交叉编译

- **纯 Go 部分**（`GOOS=windows go build`）可以在 Linux 上交叉编译
- **Wails 不行**：Wails 依赖 CGO 绑定系统 WebView2 库，交叉编译 CGO 需要 `mingw-w64` 工具链，配置复杂且问题多（链接器错误、缺少 Windows SDK 头文件等）
- **无法测试**：即使编译成功，ConPTY 和 Wails 窗口也无法在 Linux 上运行和调试

### 6.3 Windows 开发环境方案（推荐按成本选择）

| 方案 | 成本 | 适用场景 |
|------|------|---------|
| **VirtualBox/VMware 本地虚拟机** | 免费（Windows 评估版 90 天） | 日常开发调试，推荐首选 |
| **云实例按需开** | AWS/Azure ~$0.05-0.15/小时 | 不想本地跑 VM，按需开关 |
| **便宜二手 Windows 迷你主机** | 一次性 $50-100 | 长期开发，最省心 |

**推荐**：先用 VirtualBox 虚拟机开发调试（零成本），稳定后再考虑其他方案。

### 6.4 GitHub Actions CI/CD 配置

```yaml
# .github/workflows/release-windows.yml
name: Build Windows Release

on:
  push:
    tags: ['v*']

jobs:
  build-windows:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Install Wails CLI
        run: go install github.com/wailsapp/wails/v2/cmd/wails@latest

      - name: Build
        run: wails build -platform windows/amd64

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: roambench-windows-amd64
          path: build/bin/*.exe

      - name: Create Release
        uses: softprops/action-gh-release@v2
        if: startsWith(github.ref, 'refs/tags/')
        with:
          files: build/bin/*.exe
```

**说明**：
- 使用 `windows-latest` runner（公开仓库免费，私有仓库有免费额度）
- 打 tag 时自动构建并发布到 GitHub Releases
- 后续可扩展为多平台矩阵构建（Windows + Linux + macOS）

### 6.5 本地构建命令

```bash
# Linux 上构建 Linux 版（现有流程不变）
go build -o roambench ./cmd/roambench/

# Windows 上构建桌面版
wails build -platform windows/amd64

# Windows 上开发模式（热重载）
wails dev
```

---

## 七、风险与注意事项

### 高风险
1. **ConPTY VT 输出差异** — 不仅仅是"细微差异"，ConPTY 在某些路径下发送 `\r\n` 换行，TUI 程序（vim、htop、opencode）可能渲染异常。需要针对性测试和可能的输出过滤
2. **重构范围** — PTY 抽象涉及 manager.go、server.go、Session 结构体，是跨文件的侵入式重构
3. **WebView2 可用性** — 企业锁定环境可能无法安装 WebView2。需决定是否嵌入运行时（体积代价大）

### 中风险
4. **Windows 防火墙** — 如保留 TCP WebSocket 会触发弹窗，建议优先 Wails bindings
5. **CI/CD 复杂度** — 从纯 `go build` 变为需要 Windows 构建环境 + Wails 工具链
6. **最低系统要求** — ConPTY 需要 Windows 10 1809+（Build 17763），对应 Server 2019

### 低风险（后期处理）
7. **代码签名** — 未签名 exe 被 SmartScreen 拦截，证书需要时间和费用
8. **杀毒软件误报** — 新/未知发布者的 exe 可能被标记
