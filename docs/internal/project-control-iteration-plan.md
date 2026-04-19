# Project Control Layer — Iteration Plan

Created: 2026-04-19
Status: active

## Context

On 2026-04-19, six core capabilities were implemented for the project control layer:

1. **Extensible Tool Registry** — data-driven tool definitions replacing hardcoded commands
2. **Auto-Progression Engine** — tool pass → auto-complete phase → auto-start next phase → chain
3. **Full Orchestration** — `start_execution` launches the entire runbook automatically
4. **OSC Parser** — streaming parser for OSC 9/99/777 terminal notification sequences
5. **Notification Pipeline** — OSC → WebSocket push → workspace tab badge → browser notification
6. **Agent Adapter API** — HTTP endpoints for any agent to get tasks, submit artifacts, request checkpoints

These cover the three core requirements:
- Automatic work: tool registry + auto-progression + one-click orchestration
- Monitor all workstreams: OSC parsing + WebSocket push + badges
- Agent-neutral: extensible tools + agent HTTP API

## What Remains

### Iteration 1: Make Current Capabilities Usable (1–2 days)

Goal: connect the backend to the frontend and ship a release.

#### 1a. Frontend: Start Execution Button

Add a "Start Execution" button in the Project Panel Task Detail view. On click, call `action: "start_execution"`. The UI should refresh to show phase progression status.

Files: `web/js/project-panel.js`

#### 1b. Frontend: Auto-Progress Status Display

In the Task Detail phase list:
- Running phase: show spinner
- Completed phase: show ✓ with duration
- Waiting phase: show dimmed

This lets users see the runbook advancing in real time.

Files: `web/js/project-panel.js`, `web/css/style.css`

#### 1c. Tool CRUD API (Backend Only)

Add `POST/PUT/DELETE /api/project-control/tools` endpoints so users can register custom tools (e.g., `npm test`, `cargo test`, `pytest`) via API. Frontend UI can follow later.

Files: `internal/server/project_control.go`, `internal/server/server.go`

#### 1d. Release v0.4.0

Package all project control work into a versioned release:
- Update `CHANGELOG.md` with: tool registry, auto-progression, OSC notifications, agent API
- Tag `v0.4.0`
- Build release packages

### Iteration 2: Make Automation Robust (3–5 days)

Goal: reliable unattended execution and easy agent integration.

#### 2a. Failure Recovery (Plan Task 5)

When a tool run fails:
1. Auto-retry once (configurable per tool via `AutoRetryOnFail` field)
2. If retry fails: create a checkpoint in the approvals inbox
3. When `fix_or_replan` phase completes: auto-resume the failed phase

Add `RetryCount` and `MaxRetries` to `projectControlToolRun`. Add `AutoRetryOnFail` to `projectControlToolDef`.

Files: `internal/server/project_control.go`

#### 2b. Agent CLI (Plan Task 10)

Create `cmd/roambench-agent/main.go` — a standalone binary with subcommands:

```
roambench-agent status                          # print current task and phase
roambench-agent artifact --kind test_result \
  --outcome pass --file ./output.txt            # submit evidence
roambench-agent checkpoint --reason "..."       # request human review
roambench-agent notify --title "Done"           # send OSC notification
```

Reads `ROAMBENCH_URL` and `ROAMBENCH_TOKEN` from environment. Minimal dependencies (net/http + flag).

Files: `cmd/roambench-agent/main.go`, `Makefile`

#### 2c. Health Monitor Goroutine

Background goroutine that periodically checks:
- Phase running longer than tool timeout × 3 → create checkpoint
- Terminal session idle > threshold → flag as potentially stuck
- Task with no new artifacts for > N hours → push notification

On anomaly: create checkpoint + broadcast OSC-style notification via `notifHub`.

Files: `internal/server/project_control.go`

#### 2d. Agent Integration Docs

Create `docs/agent-integrations/` with copy-paste-ready configs:
- `claude-code.md` — Claude Code hooks calling `roambench-agent`
- `opencode.md` — OpenCode integration
- `generic.md` — curl-based integration for any agent

Files: `docs/agent-integrations/`

### Iteration 3: Complete the Product (1–2 weeks)

Goal: full multi-agent orchestration and polished UI.

#### 3a. Agent-as-Tool (Plan Task 11)

Add `Kind: "agent"` to `projectControlToolDef`. Agent-kind tools:
- Start in `"waiting"` status instead of executing a command
- Complete when the agent calls `POST /api/agent/v1/artifact` with matching tool run ID
- Timeout watchdog marks as failed if agent doesn't report within `def.Timeout`

This enables runbooks like: plan(human) → implement(agent) → test(go_test) → review(agent) → validate(go_test).

Files: `internal/server/project_control.go`, `web/js/project-panel.js`

#### 3b. Scheduled / Triggered Tool Execution

- Periodic: run `go test` every N minutes on all running tasks
- Trigger: detect new commits in workspace → auto-run diff_capture + test
- Configuration via tool def or project-level settings

Files: `internal/server/project_control.go`

#### 3c. Dashboard Enhancement

- Global "which agent is doing what" overview
- Per-workstream progress swimlane
- Anomaly / blocked task highlighting
- Real-time update via notification WebSocket

Files: `web/js/project-panel.js`, `web/css/style.css`

#### 3d. Tool CRUD UI

- Tool management section in Project Settings
- Add / edit / delete custom tools with form
- Tool dry-run (test execution without recording artifact)

Files: `web/js/project-panel.js`

## Recommended Minimum Path

If time is limited, the highest-value sequence is:

1. **1a** (Start Execution button) — makes today's work usable
2. **1d** (Release v0.4.0) — lets external users try it
3. **2b** (Agent CLI) — last mile for agent-neutral integration
4. **2a** (Failure Recovery) — reliability for unattended runs

Everything else is valuable but can be prioritized based on user feedback.

## Future Directions

These are not planned for a specific iteration but are the natural next steps.

### F1. fix_or_replan Auto-Resume

When a tool fails and routes to `fix_or_replan`, the human or agent fixes the code and completes the phase. Currently the task stays at the next phase after `fix_or_replan` and waits for manual action. The improvement: after `fix_or_replan` completes, automatically restart the phase that originally failed.

Implementation sketch:
- Record the failed phase ID on the task when entering `fix_or_replan` (e.g. `FailedPhaseID` field)
- In `completeProjectControlTaskPhase`, when the completed phase is `fix_or_replan`, check `FailedPhaseID` and auto-start that phase + matching tool
- Clear `FailedPhaseID` after resuming

Complexity: medium. Touches `projectControlTask` struct, `completeProjectControlTaskPhase`, and `failProjectControlTaskPhase`.

### F2. Dashboard Real-Time Refresh

The project panel dashboard currently requires manual navigation to refresh. Notification badges update in real time (via WebSocket), but the dashboard content (stats, agent activity, task list) does not.

Implementation sketch:
- In the notification WebSocket `onmessage` handler in `app.js`, trigger `refreshSnapshotSilently()` on the project panel when a notification arrives
- Debounce to avoid excessive refreshes (e.g. at most once per 2 seconds)
- Alternatively, add a dedicated project control WebSocket that pushes snapshot diffs

Complexity: small for the debounced refresh approach. The dedicated WebSocket approach is medium.

### F3. Git Commit Trigger

Instead of only running tools on a fixed 60-second timer, detect new commits in the workspace and trigger `diff_capture` + test tools immediately.

Implementation sketch:
- In `runScheduledTools`, before checking for matching tools, run `git rev-parse HEAD` in the workspace and compare with a cached value per task
- If the HEAD changed since last check, trigger `diff_capture` followed by the phase's matching tool
- Store the last-seen HEAD per task in `projectControlState` or in memory

Complexity: medium. Needs a per-task HEAD cache and a `git rev-parse` call in the scheduled loop.

## Architecture Notes

### Auto-Progression Chain

```
start_execution
  → startProjectControlTaskPhase(first phase)
  → projectControlToolForPhase(match tool)
  → startProjectControlTaskPhaseTool(queue tool)
  → projectControlRunToolAsync(goroutine)
    → completeProjectControlToolRun
      → projectControlExecuteTool (exec.Command)
      → applyProjectControlToolResult
        → completeProjectControlTaskPhase (advance to next)
        → autoProgressAfterPhaseComplete
          → start next phase + tool → chain continues
```

Stops when: no matching tool for next phase, or `ready_for_acceptance`.

### Notification Pipeline

```
Terminal pty output
  → OSCScanner.Feed() strips OSC 9/99/777
  → notificationHub.broadcast()
  → /api/notifications/ws streams JSON
  → Frontend: badge on workspace tab + browser Notification API
```

### Agent API Flow

```
Agent hook script
  → GET  /api/agent/v1/task (Bearer token)
  → POST /api/agent/v1/artifact (submits evidence)
    → updateTask(complete_phase, rowVersion bypass)
      → auto-progression chain fires
  → POST /api/agent/v1/checkpoint (requests human review)
    → creates pending checkpoint in approvals inbox
```

## File Impact Summary

| Area | Key Files |
|---|---|
| Tool Registry | `internal/server/project_control.go` (projectControlToolDef, defaultProjectControlTools) |
| Auto-Progression | `internal/server/project_control.go` (autoProgressAfterPhaseComplete, projectControlToolForPhase) |
| OSC Parser | `internal/terminal/osc.go`, `internal/terminal/osc_test.go` |
| Notification Hub | `internal/server/server.go` (notificationHub, handleNotificationWebSocket) |
| Agent API | `internal/server/project_control.go` (agent*), `internal/server/server.go` (handleAgent*) |
| Frontend Badges | `web/js/app.js` (notifWs, notifUnread), `web/css/style.css` (.tab-notif-badge) |
