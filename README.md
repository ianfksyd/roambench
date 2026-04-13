# RoamBench

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/branding/roambench-lockup-dark.svg">
    <img src="docs/branding/roambench-lockup.svg" alt="RoamBench lockup" width="720">
  </picture>
</p>

> Start, supervise, and resume long-running AI agent work from anywhere.

RoamBench is a self-hosted workbench for developers who run multiple AI coding agents across long tasks. It keeps your terminal sessions alive, lets you reconnect from any device, and gives you just enough file tooling to stay productive without dragging in a full browser IDE.

## The Problem It Solves

When you run Codex, Claude Code, OpenCode, Kimi-CLI, or similar terminal-first tools on a remote machine, you hit a recurring set of problems:

- sessions die when you close the laptop or switch devices
- multiple agents running in parallel are hard to track
- long tasks that span hours or days need reconnection without losing state
- you need to check output, inspect files, or make small fixes from a phone

RoamBench solves these by providing a lightweight web layer over `tmux` that keeps everything alive and accessible.

## Why Not SSH Or A Browser IDE

RoamBench is deliberately narrow: it keeps the highest-value pieces of remote work without trying to become a full browser IDE.

| Need | SSH | VS Code Remote / Browser IDE | RoamBench |
| --- | --- | --- | --- |
| Reconnect to a long-running agent session from a phone | awkward | possible, but heavy | built for it |
| Keep `tmux`-backed session recovery easy | manual | not the focus | built-in |
| Run `2 / 4`-pane CLI workflows for coding agents | manual setup | IDE-first | built-in |
| Inspect files and make small edits next to terminals | extra tools | yes, with more overhead | built-in |
| Stay single-user and lightweight on a self-hosted box | yes | usually heavier | yes |

It is not a multi-user platform. It is not trying to replace your full local editor for large edits. It stays opinionated so the UI can remain fast, small, and usable on mobile.

## Screenshots

Desktop workspace with `4` terminals for long-running agent and CLI tasks:

![RoamBench workspace screenshot](docs/screenshot-main.png)

Mobile reconnect to the same workspace:

![RoamBench mobile screenshot](docs/screenshot-mobile.jpg)

## Current Capabilities

### Terminal & Session Management

- single-user authentication with `password` or `pam`, plus IP allowlist
- terminal session persistence across page reloads, reconnects, and server restarts when `tmux` is available
- disk-backed terminal metadata with a configurable storage cap
- multi-pane workspace tabs with `1 / 2 / 4` layouts and cross-browser sync
- unique terminal assignment across workspaces
- visible right-side scrollbar in each terminal pane

### File Workspace

- directory listing with sorting, hidden-file toggle, breadcrumb navigation, and current-directory filtering
- text file editing with draft recovery, find / replace, go-to-line, and optional line numbers
- `New File`, `New Folder`, `Save As`, rename / move, copy, upload, download, delete
- image preview in the built-in viewer

### Agent Workflows

- well suited for keeping multiple terminal-first tools running side by side, including Codex, Claude Code, Kimi-CLI, OpenCode, and similar CLI workflows
- steer terminal-first tools such as `openclaw` without loading a heavy desktop IDE
- reconnect to long-running agent sessions from another device, including your phone
- watch scripts, data jobs, and long-running CLI tasks without babysitting SSH

### General

- lightweight, low-overhead behavior intended to stay responsive on modest self-hosted machines
- live memory indicator in the header
- interface language switching for English, Simplified Chinese, and Japanese

## Roadmap: Toward A Project Control Layer

RoamBench today provides the execution layer: persistent terminals, multi-pane workspaces, and file tools. The next major evolution is adding a **project control layer** on top of this foundation, so that managing complex multi-agent work becomes structured rather than ad-hoc.

Planned direction:

- **Task-first model** — organize work around tasks with goals, status, and evidence, not just terminal tabs
- **Timeline and evidence** — see what happened, what changed, and what the agent claims, without reading long CLI output
- **Human checkpoints** — get notified only when a decision actually requires human judgment
- **Shared project history** — track decisions, failures, and recoveries across agents and sessions
- **Local + remote runtime** — manage agents on your local machine and remote servers from the same interface
- **Agent-neutral** — not locked to one AI provider; works with any terminal-first agent

The terminal layer stays. It becomes one view inside a task, not the entire product surface.

For the full design discussion, see [`docs/project-control-discussions/`](docs/project-control-discussions/).

## Requirements

- Go `1.22+`
- `tmux` recommended for persistent terminal sessions
- Linux or another Unix-like environment with PTY support

Without `tmux`, RoamBench still runs, but terminal session persistence is reduced.

## Fast Path

Fastest setup for trusted local or LAN testing:

```bash
make build
cp configs/roambench.quickstart.toml roambench.toml
APP_BIN=<path-to-binary>         # e.g. ./roambench
APP_CONFIG=<path-to-config-file>  # e.g. ./roambench.toml
"$APP_BIN" --password-hash
export ROAMBENCH_USER="$(whoami)"
export ROAMBENCH_PASSWORD_HASH='<paste the generated hash>'
APP_CONFIG=${APP_CONFIG:-roambench.toml}
"$APP_BIN" --config "$APP_CONFIG"
```

Notes:

- this path is intentionally optimized for setup speed, not hardening
- `configs/roambench.quickstart.toml` enables insecure HTTP and disables IP filtering
- use it only on trusted local or LAN environments
- for a safer deployment, follow the full setup below

## Full Setup

1. Build the binary:

   ```bash
   make build
   ```

2. Copy the example config:

   ```bash
   cp configs/roambench.example.toml roambench.toml
   ```

3. Export the binary and config you are using for this setup:

   ```bash
   APP_BIN=<path-to-binary>         # e.g. ./roambench
   APP_CONFIG=${APP_CONFIG:-roambench.toml}
   ```

4. Edit `roambench.toml`:

   - set `[auth].single_user` to the Unix account that runs the service today via `"$APP_BIN"`
   - set `[server].allowed_ips` or enable `allow_all_ips = true` for trusted testing
   - review the terminal persistence settings

5. Generate a password hash:

   ```bash
   "$APP_BIN" --password-hash
   ```

6. Put the generated hash into `password_hash` in `roambench.toml`.

7. Start RoamBench:

   ```bash
   "$APP_BIN" --config "$APP_CONFIG"
   ```

8. Open the server in your browser.

## Build And Run

```bash
make build
make run
go test ./...
```

Release bundles for GitHub Releases:

```bash
make release-packages TAG=v0.3.2
```

PAM build:

```bash
make build-pam
```

## Upgrade Notes

- front-end assets are embedded into the Go binary
- after changing files under [web](web), rebuild and restart the RoamBench service to serve the updated UI
- without `tmux`, restarting the RoamBench service can interrupt active shells and running tasks

## Configuration

RoamBench currently looks for config files in this order:

1. `./roambench.toml`
2. `~/.config/roambench/roambench.toml`
3. `/etc/roambench/roambench.toml`

You can also pass an explicit config path:

```bash
APP_BIN=<path-to-binary> # e.g. ./roambench
"$APP_BIN" --config /path/to/roambench.toml
```

Useful CLI flags:

- `--config`: explicit config file path
- `--host`: override config host
- `--port`: override config port
- `--password-hash`: generate a `bcrypt` hash from stdin and exit

## Terminal Persistence

When `tmux` is available, RoamBench uses it as the terminal backend.

- terminal metadata is persisted to disk
- idle sessions are removed after `terminal.idle_timeout`
- persisted metadata is capped by `terminal.persist_max_bytes`
- by default the metadata directory is `~/.local/state/roambench/terminals`

This keeps memory usage low while still allowing session recovery after restart.

Without `tmux`, RoamBench still works, but server restart safety is reduced and active tasks may be interrupted.

## Workspaces

The browser UI supports workspace tabs with `1 / 2 / 4` terminal panes.

- each workspace can be renamed
- each workspace can use a different layout
- each terminal can appear only once across workspaces
- workspace state is persisted on the server for the current RoamBench user
- the browser keeps a local cached copy as a fallback

## Security Notes

- RoamBench is single-user only
- default deployments should keep IP filtering enabled
- for non-loopback hosts, RoamBench expects TLS unless `allow_insecure_http = true` is explicitly enabled

## Project Layout

- [cmd/roambench](cmd/roambench) - CLI entrypoint
- [internal/auth](internal/auth) - authentication and sessions
- [internal/server](internal/server) - HTTP server and API
- [internal/terminal](internal/terminal) - terminal session manager
- [internal/filebrowser](internal/filebrowser) - file browser backend
- [web](web) - embedded front-end assets

## More

- [Roadmap](docs/roadmap.md)
- [Configuration Guide](docs/configuration.md)
- [Authentication Guide](docs/authentication.md)
- [Deployment Hardening](docs/deployment-hardening.md)
- [Contributing](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [License](LICENSE)

## Status

RoamBench is already usable for self-hosted single-user workflows, especially terminal-first coding, agent supervision, and lightweight remote intervention. It is intentionally compact and opinionated today, with a clear path toward becoming a project control layer for multi-agent development work.
