# Agent Integrations

RoamBench's project control layer is agent-neutral. Any terminal-first AI agent can integrate via the Agent API or OSC terminal notifications.

## Quick Start

```bash
# 1. Generate an agent token (from the RoamBench UI or curl)
curl -X POST http://localhost:3000/api/project-control/agent-token \
  -b "roambench_session=<your-session-cookie>"
# Returns: {"token":"<agent-token>"}

# 2. Set environment variables
export ROAMBENCH_URL=http://localhost:3000
export ROAMBENCH_TOKEN=<agent-token>

# 3. Use the CLI
roambench-agent status
roambench-agent artifact --kind plan --outcome recorded --value "Implementation plan"
roambench-agent checkpoint --reason "Need human review"
roambench-agent notify --title "Done" --body "Task complete"
```

## Integration Guides

- [Claude Code](claude-code.md)
- [OpenCode](opencode.md)
- [Generic (any agent)](generic.md)

## How It Works

```
Agent hook script
  → roambench-agent artifact --kind test_result --outcome pass
    → POST /api/agent/v1/artifact (Bearer token)
      → completes current phase
      → auto-progression starts next phase + tool
      → chain continues until human-required phase
```

The agent doesn't need to understand RoamBench's internal model. It just submits evidence when work is done.
