# GitHub Publishing Notes

This file collects the GitHub-facing metadata for the repository.

## Repository Name

Suggested:

- `roambench`

Current public repo:

- `ianfksyd/roambench`

## Repository Description

Recommended short description:

`Self-hosted tmux workbench for long-running terminal and AI coding sessions.`

Shorter alternative:

`Lightweight self-hosted remote workbench for terminal-first coding from anywhere.`

More explicit alternative:

`RoamBench is a self-hosted web workbench for persistent tmux sessions, split-view CLI workflows, lightweight file edits, and AI coding agents.`

## Repository Tagline

Recommended slogan for README top, release subtitle, or social preview:

`Keep long-running terminal and AI coding sessions alive. Reconnect from anywhere.`

Recommended subtitle:

`A compact self-hosted workbench for persistent tmux sessions, 2 / 4-pane CLI workflows, lightweight remote edits, and browser-based supervision from desktop or phone.`

Chinese version:

`一个能让你在手机上接回 Codex、Claude Code 等 terminal-first coding 工作流的轻量远程工作台。`

推荐副标题：

`一个面向单人自托管场景的轻量工作台，适合用 2 / 4 分屏跑 CLI 工作流、保住 tmux 会话，并在电脑或手机上做轻量远程修改。`

Japanese short version:

`スマホから Codex や Claude Code などの terminal-first coding ワークフローに復帰できる、軽量リモート作業台。`

推奨サブタイトル:

`2 / 4 ペインの CLI ワークフロー、永続 tmux セッション、デスクトップやスマホからの軽いリモート編集に向いた、単一ユーザー向けの軽量セルフホスト作業台。`

## Elevator Pitch

Recommended one-paragraph pitch:

`RoamBench sits between SSH and a full browser IDE. It keeps the high-value parts of remote terminal work close at hand: persistent tmux-backed sessions, 2 / 4-pane workspaces, file access, lightweight editing, and browser-based supervision for terminal-first tools such as Codex, Claude Code, Kimi-CLI, OpenCode, and ordinary long-running scripts.`

## Suggested Topics

Recommended GitHub topics:

- `terminal`
- `web-terminal`
- `self-hosted`
- `tmux`
- `golang`
- `xtermjs`
- `pty`
- `remote-access`
- `remote-workspace`
- `file-browser`
- `browser-terminal`
- `vibe-coding`
- `ai-agents`

If you want a smaller set, keep these first:

- `web-terminal`
- `self-hosted`
- `tmux`
- `golang`
- `browser-terminal`

## Release Tag Strategy

Recommended format:

- `v0.2.0`
- `v0.2.1`
- `v1.0.0`

Guideline:

- patch release: fixes, polish, no major workflow change
- minor release: new user-facing features or behavior changes
- major release: compatibility or deployment model changes

## First Release

Suggested first tag:

- `v0.2.0`

Suggested release title:

- `RoamBench v0.2.0`

Suggested release subtitle:

- `RoamBench, reconnect to terminal-first coding from your phone`

Suggested release note source:

- [GitHub Release Copy](../releases/v0.2.0.md)

## Future Release Title Pattern

Recommended pattern:

- `RoamBench v0.2.0`
- `RoamBench v0.2.1`
- `RoamBench v1.0.0`

Optional marketing-style pattern:

- `RoamBench v0.2.0: Workspaces and Viewer`
- `RoamBench v0.3.0: Session Recovery Improvements`

The simpler numeric pattern is better for the first few releases.

## Recommended GitHub Repo Setup

Suggested checklist:

- set the repo description
- add the recommended topics
- pin the screenshot-rich `README.md`
- create the first release from tag `v0.2.0`
- copy release notes from [GitHub Release Copy](../releases/v0.2.0.md)
- follow the launch checklist in [Launch Playbook](launch-playbook.md)
- keep `CHANGELOG.md` updated for future releases

## Notes

- `README.md` stays English-first for GitHub defaults
- `README.zh-CN.md` is the Simplified Chinese version
- `README.ja.md` is intentionally a short Japanese version, not a full mirrored manual
- release notes can stay bilingual until the project stabilizes
