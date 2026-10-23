# Shared Project Memory

Every agent that works on a project reads the same memory. Instead of each
coding agent keeping a private copy of the project facts in its own memory
file, RoamBench stores one canonical memory per project and the agent reads
it through the `roambench-agent` CLI.

That is where the repeated re-reading used to cost tokens: with N agents
keeping N copies, the same facts were paid for N times. With one canonical
memory, each agent reads the same (server-side, windowed) document.

## Quick Start

```bash
export ROAMBENCH_URL=http://localhost:3000
export ROAMBENCH_TOKEN=<agent-token>

# Read the memory of a project
roambench-agent memory get --project project-abc

# Append what you learned
roambench-agent memory append --project project-abc --text "网关在 127.0.0.1:3425"

# Replace it (rare)
roambench-agent memory set --project project-abc --text "..."

# Write the pointer into the agent memory files of a workspace
roambench-agent memory pointer --project project-abc \
    --file /path/to/repo/CLAUDE.md --file /path/to/repo/AGENTS.md
```

`memory get` also accepts `--task <taskId>` when the agent only knows its
task; the project is then resolved from the task.

## Automatic pointers (M3)

Registering a project writes the pointer for you:

- into the workspace of the project — `CLAUDE.md` and `AGENTS.md` are created
  when the matching agent is installed on the machine, and updated in place
  when they already exist (your own instructions are kept);
- into the global agent memory files that already exist
  (`~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, …) — as a **generic** block
  that does not name a project, so one block per file is enough. The agent
  learns the project from the task it claimed.

Both writes are idempotent, backed up (`<file>.roambench-bak`) and reversible:

```bash
# remove the pointers again
curl -X POST -b "roambench_session=..." \
     -H 'Content-Type: application/json' \
     -d '{"projectId":"project-abc","remove":true}' \
     http://localhost:3000/api/project-control/memory/pointers
```

Three switches control it:

```toml
[agent]
# Insert the pointer into the agent memory files that already exist in the
# workspace of a new project (detection only, nothing is created).
workspace_memory_pointer = true     # default
# Also create CLAUDE.md / AGENTS.md in a registered project's workspace when
# the matching agent is installed.
create_missing_memory_files = false # default
# Write the project agnostic (generic) pointer into the global agent memory
# files that already exist: ~/.claude/CLAUDE.md, ~/.codex/AGENTS.md. A global
# memory file is the user's own instruction sheet, so this is opt-in.
global_memory_pointer = false       # default
```

## The pointer

`memory pointer` writes a managed block into the target file:

```markdown
<!-- roambench:memory:begin project=project-abc -->
## 项目记忆(RoamBench canonical memory)

本项目的共享事实只有一份,存放在 RoamBench 里。不要在本地另存副本,
也不要凭记忆复述:每次需要项目上下文时读取,需要补充时追加。

- 读取: roambench-agent memory get --project project-abc
- 追加: roambench-agent memory append --project project-abc --text "..."
<!-- roambench:memory:end -->
```

The block is idempotent: running it again updates the block in place and
leaves the rest of the file alone. Blocks of other projects in the same file
are preserved, and the previous file content is copied to
`<file>.roambench-bak` before the first change.

## How the memory is seeded

When a project is created, the canonical memory starts from the project
brief (goal, understanding, deliverables, out of scope, assumptions). Agents
append to it from then on. The revision counter increases on every write, so
a reader can tell whether its copy is stale.

## Where it travels

The canonical memory is part of the project control state, so:

- every agent of the owning user sees the same document;
- it survives agent restarts and machine reboots;
- it is included in the claim response (`projectMemory`), so an agent that
  claims a task already receives the shared facts.

## HTTP API

The same memory is reachable over HTTP:

```bash
# Agent token
curl -H "Authorization: Bearer $ROAMBENCH_TOKEN" \
     "http://localhost:3000/api/agent/v1/memory?project=project-abc"

curl -X POST -H "Authorization: Bearer $ROAMBENCH_TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"projectId":"project-abc","content":"...","mode":"append"}' \
     http://localhost:3000/api/agent/v1/memory

# Web UI session
curl -b "roambench_session=..." \
     "http://localhost:3000/api/project-control/memory?project=project-abc"
```

`mode` is `append` (default) or `replace`.

## Which file to point at

| Agent | File | Note |
|---|---|---|
| Claude Code | `CLAUDE.md` | read at session start |
| Codex | `AGENTS.md` | |
| OpenCode | `AGENTS.md` | |
| Any agent | `AGENTS.md` | the de-facto convention |

Point at the workspace of the project, not at a global file, so a file that
serves several projects ends up with one block per project.
