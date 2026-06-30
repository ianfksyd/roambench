# Changelog

All notable changes to this project will be documented in this file.

The format is inspired by Keep a Changelog, with a lightweight structure for this repository.

## [Unreleased]

## [0.4.1] - 2026-04-19

### Added

- `roambench-agent` standalone CLI for hook integration
- Agent-as-tool support for tools with `kind: agent` that wait for external callback
- Failure recovery with auto-retry on transient errors and checkpoint creation when retries are exhausted
- Health monitor coverage for phase timeout, stall detection, and agent tool run timeout watchdogs
- Scheduled execution for periodic tool runs on active task phases
- Dashboard agent activity view for running and waiting tool runs

## [0.4.0] - 2026-04-19

### Added

- Extensible tool registry: tool definitions are now data-driven with per-tool timeout, max output, artifact kind, and allowed phases; default tools (repo_status, diff_capture, go_test) preserved as built-in defaults
- Tool CRUD API: `POST/PUT/DELETE /api/project-control/tools` for registering custom tools such as `npm test`, `cargo test`, or `pytest`
- Auto-progression engine: when a tool run passes, the runbook automatically advances to the next phase and starts the matching tool, chaining through phases without manual clicks
- Full runbook orchestration: `start_execution` action launches the first phase, matches a tool, and triggers the auto-progression chain; a single click can drive plan → implement → test → review
- OSC terminal notification parser: streaming parser for OSC 9/99/777 escape sequences with support for BEL and ST terminators and partial sequence buffering
- Notification pipeline: OSC notifications are stripped from terminal output, broadcast via a WebSocket endpoint (`/api/notifications/ws`), and displayed as badges on workspace tabs with browser Notification API support
- Agent adapter API: `GET /api/agent/v1/task`, `POST /api/agent/v1/artifact`, `POST /api/agent/v1/checkpoint` with bearer token auth for agent-neutral integration; any terminal-first agent can get tasks, submit evidence, and request human review
- Agent token management: `POST /api/project-control/agent-token` generates a persistent bearer token for agent API access
- Start Execution button in the project panel replaces Add to queue / Start phase for planned and queued tasks
- Phase status icons: ✓ (completed), ○ with pulse animation (running), ✗ (failed) in the task detail phase list
- Documentation reorganization: docs split into user guides, internal/operational, releases, and competitive analysis with a new index page

### Changed

- `normalizeProjectControlToolID` now accepts any registered tool ID instead of only the three hardcoded defaults
- `projectControlToolAllowedInPhase` uses the tool registry `AllowedPhases` field instead of a hardcoded switch
- `executeLocalProjectControlTool` uses per-tool timeout and max output from the tool definition
- `updateTask` supports agent bypass mode (`ExpectedRowVersion: -1`) for agent API artifact submission
- `start_execution` is now a phase action that starts the first runbook phase instead of only setting task state

## [0.3.2] - 2026-04-13

### Added

- Viewer can now preview DOCX and XLSX files directly in the browser
- PPTX preview now uses in-view slide rendering with next / previous navigation

### Changed

- Release docs and release-note references now point at the `v0.3.2` flow

### Fixed

- File selections now open in Viewer first, so extensionless plain-text files no longer force the split editor layout
- Single-pane desktop workspaces can widen the embedded file panel with a horizontal drag handle while keeping the current default width as the minimum
- Inline PDF preview no longer gets blocked by Chromium-family extension-backed PDF viewers
- PDF / Office file rows now use clearer file-type badges in the file browser

## [0.3.1] - 2026-04-12

### Added

- Mobile reconnect screenshots are now included in the README and launch materials
- Added a reusable release packaging flow for Linux `amd64` / `arm64` archives plus `.sha256` files
- Added a generic GitHub-release installer script and a release packaging guide

### Changed

- README and launch copy now lean on cross-device reconnection with proof assets, not just phone-first claims
- Release helpers now have a clean path for building tagged binaries with injected version strings

### Fixed

- File browser rows now show compact file type badges instead of generic file icons
- Japanese README messaging now matches the updated English and Chinese positioning more closely

## [0.3.0] - 2026-04-11

### Added

- Viewer empty state can open a new draft directly from pasted clipboard text or images
- Added release-note drafts and installer defaults for the `v0.3.0` release flow

### Changed

- Runtime identifiers are now fully aligned on `roambench`; legacy `liteterm` environment variable, cookie, config-path, and state-directory fallbacks were removed
- Viewer empty-state copy now makes the clipboard-first draft flow more explicit
- Publish helpers and README release-note links now point at the current `v0.3.0` release draft

### Fixed

- Closing viewer edit mode now returns the viewer to its empty state instead of forcing the editor into split layout
- Opening a file preview while a viewer draft has content now goes through the same discard flow instead of silently replacing draft state

## [0.2.2] - 2026-04-10

### Added

- Viewer can create a new draft from pasted or typed text and save it into the current file browser directory
- Viewer can accept pasted images, preview them, and upload them as new files
- Workspace tabs now have scroll controls for overflowing tab lists
- Added a Windows native app planning document covering Wails, ConPTY, packaging, and opencode installation
- Added launch posting helper docs and dynamic install script release notes

### Changed

- Release-prep cleanup now uses generic publishable defaults, a template `systemd` unit, and sanitized public-facing assets and docs
- Workspace tab scrolling and drag reordering behavior is more consistent across desktop and mobile layouts

### Fixed

- Terminal attachment processes are now reaped so tmux attach clients do not remain as zombie child processes
- Workspace tab clicks are no longer swallowed by tiny pointer movement during drag detection
- Workspace tab dragging now reorders correctly in both directions

## [0.2.0] - 2026-04-05

### Added

- Server-side workspace state sync with browser-local fallback, so the same user can recover views across browsers and devices
- Completed file-workspace actions for `New File`, `New Folder`, `Save As`, rename / move, copy, drag-and-drop upload, and upload cancel
- Safer editor features including unsaved-change warning, draft restore, find / replace, go-to-line, optional line numbers, and clearer save-state feedback
- File browser breadcrumb navigation and current-directory filtering
- A quickstart configuration for faster trusted local/LAN trials, plus a launch playbook for GitHub, Hacker News, Reddit, and community posts
- A lightweight evidence page with a real runtime snapshot from a live multi-terminal session
- A formal [RoamBench rebrand checklist](docs/internal/rebrand-checklist.md) covering the public rename and later code-level migration

### Changed

- Terminal panes now expose a visible right-side scrollbar instead of relying only on overlay system scrollbars or keyboard history navigation
- Workspace persistence is no longer browser-only; the server now keeps the authoritative workspace copy for the current signed-in user
- README positioning now leads with phone-friendly Codex / Claude Code reconnection and a shorter fast-path setup
- Public-facing docs and launch copy now use the `RoamBench` product name with technical identifiers aligned on `roambench`

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
