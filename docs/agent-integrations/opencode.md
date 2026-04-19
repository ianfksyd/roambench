# OpenCode Integration

## Prerequisites

- RoamBench running with an agent token generated
- `roambench-agent` binary in PATH
- OpenCode installed

## Setup

### 1. Create a wrapper script

```bash
cat > ~/.config/opencode/hooks/roambench.sh << 'EOF'
#!/bin/bash
[ -z "$ROAMBENCH_URL" ] && exit 0
[ -z "$ROAMBENCH_TOKEN" ] && exit 0

ACTION="${1:-}"
case "$ACTION" in
    "complete")
        roambench-agent notify --title "OpenCode" --body "Task complete"
        roambench-agent artifact --kind completion_check --outcome pass \
            --label "OpenCode session" --value "Session completed"
        ;;
    "error")
        roambench-agent notify --title "OpenCode" --body "Error occurred"
        roambench-agent checkpoint --reason "OpenCode encountered an error"
        ;;
esac
EOF
chmod +x ~/.config/opencode/hooks/roambench.sh
```

### 2. Set environment variables

```bash
export ROAMBENCH_URL=http://localhost:3000
export ROAMBENCH_TOKEN=<your-agent-token>
```

### 3. Run OpenCode in a RoamBench terminal

Use the hook script in your OpenCode workflow. The exact hook mechanism depends on your OpenCode version — check its documentation for lifecycle hooks.

## Using OSC Notifications Directly

If your OpenCode version doesn't support hooks, you can add OSC notifications to your shell:

```bash
# Add to ~/.bashrc or ~/.zshrc
notify_roambench() {
    printf '\033]777;notify;%s;%s\007' "$1" "$2"
}

# After a long command
opencode run && notify_roambench "OpenCode" "Task finished"
```

RoamBench will detect the OSC 777 sequence and show a badge on the workspace tab.
