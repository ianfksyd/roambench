# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by Keep a Changelog, with a lightweight structure for this repository.

## [Unreleased]

### Changed

- Release-prep cleanup now uses generic publishable defaults, a template `systemd` unit, and sanitized public-facing assets and docs

## [0.2.0] - 2026-04-05

### Added

- Server-side workspace state sync with browser-local fallback, so the same user can recover views across browsers and devices
- Completed file-workspace actions for `New File`, `New Folder`, `Save As`, rename / move, copy, drag-and-drop upload, and upload cancel
- Safer editor features including unsaved-change warning, draft restore, find / replace, go-to-line, optional line numbers, and clearer save-state feedback
- File browser breadcrumb navigation and current-directory filtering
- A quickstart configuration for faster trusted local/LAN trials, plus a launch playbook for GitHub, Hacker News, Reddit, and community posts
- A lightweight evidence page with a real runtime snapshot from a live multi-terminal session
- A formal [RoamBench rebrand checklist](docs/rebrand-checklist.md) covering the public rename and later code-level migration

### Changed

- Terminal panes now expose a visible right-side scrollbar instead of relying only on overlay system scrollbars or keyboard history navigation
- Workspace persistence is no longer browser-only; the server now keeps the authoritative workspace copy for the current signed-in user
- README positioning now leads with phone-friendly Codex / Claude Code reconnection and a shorter fast-path setup
- Public-facing docs and launch copy now use the `RoamBench` product name while technical identifiers are now `roambench` with `liteterm` retained for compatibility

### Fixed

- Logging in from another browser or computer no longer drops back to only the first view when synced workspace state is available

### Notes

- Front-end assets are embedded into the Go binary, so updating the UI requires rebuilding and restarting the service

## [0.1.0] - 2026-04-03

Initial public release.

### Added

- Single-user web terminal with password or PAM authentication
- IP allowlist support and TLS-aware startup validation
- Persistent terminal sessions backed by `tmux`
- Disk-backed terminal metadata persistence with idle cleanup and storage limits
- Browser workspaces with `1 / 2 / 4` terminal layouts
- Unique terminal assignment across workspaces
- Renameable workspace tabs
- Built-in file browser, text editor, and image viewer
- Browser-local UI preferences for language, fonts, and terminal theme
- Live memory indicator for process RSS, system used memory, and total memory

### Notes

- Terminal session state lives on the server
- Workspace layout state lives in the browser
