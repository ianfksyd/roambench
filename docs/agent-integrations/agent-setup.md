# Agent Setup Wizard

The wizard moves the coding agents installed on a machine onto one model
access point — typically a local gateway that fronts several provider
subscriptions. One access point means one place to change models, and one
memory surface for the agents (see [Shared project memory](shared-memory.md)).

## What it does

1. **Scans** which agent CLIs are installed (`internal/agentprobe`) and
   where each of them currently points. Resolution goes through PATH first,
   then through the common install directories of the agent ecosystems
   (`~/.local/bin`, `~/.opencode/bin`, `~/.volta/bin`, `~/go/bin`,
   `~/.openclaw/tools/*/bin`, `~/.nvm/versions/node/*/bin`) — agent managers
   often install into directories a service process does not inherit.
2. **Shows the difference**: which settings would change, key by key.
3. **Applies** the profile with a deterministic writer — one small adapter
   per CLI — and keeps a backup of the previous file.
4. **Prints the manual steps** that only the user can do: install, sign in,
   activate a subscription.

No language model edits user configuration. The changes are computed and
applied by the server; an agent may only explain them.

## Where it lives in the UI

"新项目" (new project) → "接入 agent" (agent setup). The table lists every
supported agent with its state, its current access point, the number of
changes, and an apply button.

## Configuration

```toml
[agent.gateway]
name = "magpie"
base_url = "http://127.0.0.1:3425"
auth_token = "magpie"          # usually a placeholder for a local gateway
model = "auto/glm-5"
codex_path = "/backend-api/codex"
```

`codex_path` exists because most gateways expose the OpenAI-compatible API
on another path than the Anthropic one.

## Supported agents and their files

| Agent | File | Keys written |
|---|---|---|
| Claude Code | `~/.claude/settings.json` | `env.ANTHROPIC_BASE_URL`, `env.ANTHROPIC_AUTH_TOKEN`, `env.ANTHROPIC_DEFAULT_*_MODEL` |
| Codex | `~/.codex/config.toml` | `openai_base_url`, `model` |
| OpenCode | `~/.config/opencode/opencode.json` | `provider.<name>.baseURL`, `provider.<name>.model` |
| Hermes | `~/.hermes/config.yaml` | `providers.<name>.base_url/api_mode/key_cmd/discover_models`, `model.provider`, `model.default` |

The hermes edit is surgical on the YAML tree: every other provider, model
entry and key survives untouched, and the gateway credential rides on
`key_cmd` (a command that prints the key), so no login and no env plumbing.

`kiro-cli` is listed as **worker only**: its CLI has no configuration
surface for a custom model endpoint, so no adapter can ever point it at a
gateway. It still claims tasks and submits evidence through
`roambench-agent`; its models come from its own login.

Unrelated settings in those files are preserved: the JSON adapters keep every
other key, and the TOML adapter edits line by line so comments survive.

Backups are written next to the file as `<file>.roambench-bak` before the
first change.

## From the command line

```bash
roambench-agent config show --agent claude
roambench-agent config apply --agent codex \
    --base-url http://127.0.0.1:3425 --codex-path /backend-api/codex \
    --model auto/glm-5
```

## HTTP API

```
GET  /api/project-control/agent-config                  overview
GET  /api/project-control/agent-config?agent=claude     manual steps
POST /api/project-control/agent-config?agent=claude              plan
POST /api/project-control/agent-config?agent=claude&action=apply  apply
```

The plan and apply requests take the profile in the body:

```json
{"profile": {"name": "magpie", "baseUrl": "http://127.0.0.1:3425", "model": "auto/glm-5"}}
```

An empty profile falls back to `[agent.gateway]` from the instance
configuration.

## Local task worker

The setup wizard configures the executor; the **local worker** is what
actually runs tasks:

```bash
export ROAMBENCH_URL=http://localhost:3000
export ROAMBENCH_TOKEN=<agent-token>
roambench-worker                      # executor from config, default claude
```

Each claimed task runs in a terminal session created through the server, so
the web UI can attach to it and show exactly what the executor is doing.
Flags: `-executor`, `-model`, `-poll`, `-max-runtime`, `-executor-args`
(off by default — permission bypass flags are the operator's explicit
choice).

## What it deliberately does not do

- It does not install an agent. The wizard prints the command; the user runs
  it. Installing silently would write to the machine of whoever runs the
  instance, and the service account is often not the interactive user.
- It does not sign in for the user. Subscription and login steps are shown
  as commands.
- It does not touch agents without an adapter (`kiro-cli`, `hermes`); they
  are listed as "no adapter" so the gap is visible.
