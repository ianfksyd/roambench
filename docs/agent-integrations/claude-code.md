# Claude Code Integration

## Prerequisites

- RoamBench running with an agent token generated
- `roambench-agent` binary in PATH (or use full path)
- Claude Code installed

## Setup

### 1. Create the hook script

```bash
cat > ~/.claude/hooks/roambench-hook.sh << 'EOF'
#!/bin/bash
[ -z "$ROAMBENCH_URL" ] && exit 0
[ -z "$ROAMBENCH_TOKEN" ] && exit 0

EVENT=$(cat)
EVENT_TYPE=$(echo "$EVENT" | jq -r '.hook_event_name // "unknown"')
TOOL=$(echo "$EVENT" | jq -r '.tool_name // ""')

case "$EVENT_TYPE" in
    "Stop")
        roambench-agent notify --title "Claude Code" --body "Session complete"
        roambench-agent artifact --kind completion_check --outcome pass \
            --label "Claude Code session" --value "Session completed normally"
        ;;
    "PostToolUse")
        if [ "$TOOL" = "Task" ]; then
            roambench-agent notify --title "Claude Code" --body "Sub-task finished"
        fi
        ;;
esac
EOF
chmod +x ~/.claude/hooks/roambench-hook.sh
```

### 2. Configure Claude Code

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/roambench-hook.sh"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Task",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/roambench-hook.sh"
          }
        ]
      }
    ]
  }
}
```

### 3. Set environment variables

Add to your shell profile or RoamBench terminal startup:

```bash
export ROAMBENCH_URL=http://localhost:3000
export ROAMBENCH_TOKEN=<your-agent-token>
```

### 4. Run Claude Code in a RoamBench terminal

When Claude Code completes a task or sub-task, the hook will:
- Send a notification (visible as a badge on the workspace tab)
- Submit a completion artifact (advances the runbook phase if applicable)

## Manual Artifact Submission

For more control, call `roambench-agent` directly in your prompts or scripts:

```bash
# After implementing a feature
roambench-agent artifact --kind diff_summary --outcome pass \
    --label "Feature implementation" --value "$(git diff --stat)"

# After running tests
roambench-agent artifact --kind test_result --outcome pass \
    --label "Test results" --file ./test-output.txt

# When stuck
roambench-agent checkpoint --reason "API design needs human review"
```
