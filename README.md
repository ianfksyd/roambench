# RoamBench

[English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/branding/roambench-lockup-dark.svg">
    <img src="docs/branding/roambench-lockup.svg" alt="RoamBench lockup" width="720">
  </picture>
</p>

> Reconnect to Codex, Claude Code, and other terminal-first coding workflows from your phone.

- Keep terminal sessions alive with `tmux`
- Run `2 / 4`-pane workspaces for Codex, Claude Code, Kimi-CLI, OpenCode, and other terminal-first tools
- Start, supervise, resume, and lightly edit long-running work with low overhead from anywhere

RoamBench is a compact self-hosted remote workbench for one person.

RoamBench is the public-facing product name. The current repo, binary, config, and environment variable identifiers still use `liteterm` until the code-level rename is completed.

Think of it as the "enough" layer between SSH and a full browser IDE: open your machine from anywhere, keep terminal sessions alive, inspect files, copy and edit files, and kick off long-running work from a laptop or phone without dragging in a heavy stack.

Built for:

- vibe coding and agent-driven development
- side-by-side Codex, Claude Code, Kimi-CLI, OpenCode, and other long-running CLI workflows
- remote scripts, data jobs, and long-running CLI tasks
- checking progress and making small fixes away from your desk
- directing terminal-first tools such as `openclaw` without needing a heavy desktop IDE

It combines:

- persistent terminal sessions backed by `tmux`
- server-synced workspace tabs with `1 / 2 / 4` terminal layouts, including practical `2 / 4`-pane split views
- a built-in file browser, text editor, and image viewer
- file copy, rename / move, upload, download, and other lightweight file actions
- safer editing tools and large-directory navigation helpers
- browser-local UI settings for language, font, colors, and layout
- a lightweight, low-overhead footprint that stays fast to reconnect and resume

## Why It Exists

RoamBench takes inspiration from tools like `rstudio-server`, but strips the idea down aggressively.

The goal is not to recreate a full desktop IDE in the browser. The goal is to keep the highest-value pieces:

- terminal access
- file access
- fast edits
- session recovery
- workspace layout that follows you across devices

That tradeoff matters on phones and small screens, where a "full IDE in the browser" usually becomes slow and awkward. RoamBench is designed for "get in, start work, check work, fix small things, get out."

## What It Is And Isn't

- It is a compact, single-user, self-hosted remote workbench.
- It is good for starting, supervising, and lightly intervening in work from anywhere.
- It is especially useful when the terminal is the main control surface for coding agents and automation tasks.
- It is comfortable for running multiple terminal-first tools side by side in `2 / 4`-pane layouts.
- It is not a multi-user platform.
- It is not trying to replace a full local editor for heavy code editing.
- It stays intentionally opinionated so the UI can remain fast, small, and usable on mobile.

## Screenshot

![RoamBench screenshot](docs/screenshot-main.png)

## Features

- Single-user authentication with `password` or `pam`
- IP allowlist support
- terminal session persistence across page reloads and server restarts when `tmux` is available
- disk-backed terminal metadata with a configurable storage cap
- multi-pane workspace tabs with `1 / 2 / 4` layouts and cross-browser sync for the same RoamBench user
- unique terminal assignment across workspaces
- well suited for keeping multiple terminal-first tools running side by side, including Codex, Claude Code, Kimi-CLI, OpenCode, and similar CLI workflows
- built-in file workspace for `New File`, `New Folder`, `Save As`, rename / move, copy, upload, download, delete, and inline image viewing
- draft restore, unsaved-change warnings, clearer save-state feedback, find / replace, go-to-line, and optional line numbers in the editor
- breadcrumb navigation and current-directory filtering in the file browser
- visible right-side terminal scrollbar in each pane
- lightweight, low-overhead behavior intended to stay responsive on modest self-hosted machines
- live memory indicator in the header
- interface language switching for English, Simplified Chinese, and Japanese

## Current Model

RoamBench is intentionally simple:

- it is designed for one Unix user per server process
- the login username must match the Unix account running `liteterm`
- terminal sessions live on the server
- workspace tabs are stored on the server and cached in the browser
- UI preferences remain browser-local

That means:

- terminal sessions can survive refresh, reconnect, and server restart when `tmux` is available
- workspace names, `1 / 2 / 4` layouts, and per-view terminal placement can follow the same RoamBench user across browsers and devices
- language, font, theme, and editor view preferences remain local to each browser

## Requirements

- Go `1.22+`
- `tmux` recommended for persistent terminal sessions
- Linux or another Unix-like environment with PTY support

Without `tmux`, RoamBench still runs, but terminal session persistence is reduced.

## Fast Path

Fastest setup for trusted local or LAN testing:

```bash
make build
cp configs/liteterm.quickstart.toml liteterm.toml
./liteterm --password-hash
export LITETERM_USER="$(whoami)"
export LITETERM_PASSWORD_HASH='<paste the generated hash>'
./liteterm --config liteterm.toml
```

Notes:

- this path is intentionally optimized for setup speed, not hardening
- `configs/liteterm.quickstart.toml` enables insecure HTTP and disables IP filtering
- use it only on trusted local or LAN environments
- for a safer deployment, follow the full setup below

## Full Setup

1. Build the binary:

   ```bash
   make build
   ```

2. Copy the example config:

   ```bash
   cp configs/liteterm.example.toml liteterm.toml
   ```

3. Edit `liteterm.toml`:

   - set `[auth].single_user` to the Unix account that runs the service today via `./liteterm`
   - set `[server].allowed_ips` or enable `allow_all_ips = true` for trusted testing
   - review the terminal persistence settings

4. Generate a password hash:

   ```bash
   ./liteterm --password-hash
   ```

5. Put the generated hash into `password_hash` in `liteterm.toml`.

6. Start RoamBench:

   ```bash
   ./liteterm --config liteterm.toml
   ```

7. Open the server in your browser.

## Build And Run

```bash
make build
make run
go test ./...
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

1. `./liteterm.toml`
2. `~/.config/liteterm/liteterm.toml`
3. `/etc/liteterm/liteterm.toml`

You can also pass an explicit config path:

```bash
./liteterm --config /path/to/liteterm.toml
```

Useful CLI flags:

- `--config`: explicit config file path
- `--host`: override config host
- `--port`: override config port
- `--password-hash`: generate a `bcrypt` hash from stdin and exit

More details:

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

## Terminal Persistence

When `tmux` is available, RoamBench uses it as the terminal backend.

- terminal metadata is persisted to disk
- idle sessions are removed after `terminal.idle_timeout`
- persisted metadata is capped by `terminal.persist_max_bytes`
- by default the metadata directory is `~/.local/state/liteterm/terminals`

This keeps memory usage low while still allowing session recovery after restart.

Without `tmux`, RoamBench still works, but server restart safety is reduced and active tasks may be interrupted.

## Workspaces

The browser UI supports workspace tabs with `1 / 2 / 4` terminal panes.

- each workspace can be renamed
- each workspace can use a different layout
- each terminal can appear only once across workspaces
- workspace state is persisted on the server for the current RoamBench user
- the browser keeps a local cached copy as a fallback

## File Tools

RoamBench includes a lightweight browser-side workspace for server files:

- directory listing with sorting, hidden-file toggle, breadcrumb navigation, and current-directory filtering
- text file editing with draft recovery, find / replace, go-to-line, and optional line numbers
- `New File`, `New Folder`, `Save As`, rename / move, and copy
- upload and download
- image preview in the built-in viewer

The file browser is rooted in the authenticated user's home directory.

## Security Notes

- RoamBench is single-user only
- default deployments should keep IP filtering enabled
- for non-loopback hosts, RoamBench expects TLS unless `allow_insecure_http = true` is explicitly enabled

## Project Layout

- [cmd/liteterm](cmd/liteterm) - CLI entrypoint
- [internal/auth](internal/auth) - authentication and sessions
- [internal/server](internal/server) - HTTP server and API
- [internal/terminal](internal/terminal) - terminal session manager
- [internal/filebrowser](internal/filebrowser) - file browser backend
- [web](web) - embedded front-end assets

## Status

The project is already usable for self-hosted single-user workflows, especially terminal-first coding, agent supervision, and lightweight remote intervention, but it is still intentionally compact and opinionated.
