# Generic Agent Integration

This guide works with any terminal-first agent: Codex, Kimi-CLI, Aider, Goose, or custom scripts.

## Option 1: roambench-agent CLI

```bash
export ROAMBENCH_URL=http://localhost:3000
export ROAMBENCH_TOKEN=<your-agent-token>

# Check current task
roambench-agent status

# Submit evidence
roambench-agent artifact --kind plan --outcome recorded --value "Plan description"
roambench-agent artifact --kind test_result --outcome pass --file ./output.txt

# Request human review
roambench-agent checkpoint --reason "Need approval for API change"

# Send notification
roambench-agent notify --title "Agent" --body "Work complete"
```

## Option 2: curl (no CLI needed)

```bash
TOKEN="<your-agent-token>"
URL="http://localhost:3000"

# Get current task
curl -H "Authorization: Bearer $TOKEN" "$URL/api/agent/v1/task"

# Submit artifact
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"taskId":"<id>","artifactKind":"test_result","outcome":"pass","label":"Tests","value":"All passed"}' \
  "$URL/api/agent/v1/artifact"

# Request checkpoint
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"taskId":"<id>","reason":"Need human review"}' \
  "$URL/api/agent/v1/checkpoint"
```

## Option 3: OSC Terminal Notifications (zero config)

Any process running in a RoamBench terminal can send notifications via escape sequences:

```bash
# OSC 777 (simple, widely supported)
printf '\033]777;notify;Build Done;All tests passed\007'

# OSC 9 (title only)
printf '\033]9;Task complete\007'

# Shell helper function
notify() { printf '\033]777;notify;%s;%s\007' "$1" "$2"; }
notify "Agent" "Implementation finished"
```

## Option 4: Post-command hook in shell

```bash
# Add to ~/.bashrc — notify after any long-running command
notify_after() {
    "$@"
    local rc=$?
    if [ $rc -eq 0 ]; then
        printf '\033]777;notify;✓ Done;%s\007' "$1"
    else
        printf '\033]777;notify;✗ Failed;%s (exit %d)\007' "$1" "$rc"
    fi
    return $rc
}

# Usage
notify_after go test ./...
notify_after npm run build
```

## API Reference

| Endpoint | Method | Auth | Description |
|---|---|---|---|
| `/api/project-control/agent-token` | POST | Cookie | Generate agent token |
| `/api/agent/v1/task` | GET | Bearer | Get active task info |
| `/api/agent/v1/artifact` | POST | Bearer | Submit phase evidence |
| `/api/agent/v1/checkpoint` | POST | Bearer | Request human review |

Artifact submission triggers auto-progression: if the submitted evidence satisfies the current phase, the runbook automatically advances to the next phase.
