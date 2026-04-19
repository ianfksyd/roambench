# RoamBench v0.3.0 发布说明（中文）

## 变更概述

`v0.3.0` 主要做了两件事：

- 把运行时命名彻底统一到 `roambench`
- 把 viewer 草稿与关闭编辑的交互整理得更顺

## 主要更新

- viewer 空状态下可以直接通过粘贴文字或图片进入新草稿
- 从 viewer 进入编辑后关闭，不再强制切回上下分屏，而是回到 viewer 空状态
- 当 viewer 草稿已有内容时，再去打开其他预览会走统一的放弃确认流程

## 运行时命名清理

本次版本移除了剩余的 `liteterm` 兼容入口，运行时标识统一为 `roambench`：

- 不再读取 `LITETERM_PASSWORD_HASH` / `LITETERM_USER`
- 不再回退读取 `liteterm.toml`、`~/.config/liteterm/`、`/etc/liteterm/`
- 不再读取旧的 `liteterm_session` cookie
- terminal 持久化目录统一为 `~/.local/state/roambench/terminals`

## 升级提示

- 如果你还在使用 `liteterm` 前缀的环境变量、配置文件名、cookie 约定或旧状态目录，请在升级前改到 `roambench`

## 建议附带的发布资产

- Linux `amd64` 压缩包
- Linux `arm64` 压缩包
- 两个压缩包对应的 `.sha256` 校验文件

## 短版发布文案

`RoamBench v0.3.0 完成了运行时命名清理，并把 viewer 草稿流转整理成更直接、更偏剪贴板优先、也更不打断操作的体验。`
