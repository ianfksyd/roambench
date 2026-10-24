#!/bin/bash
# roambench-worker 安装器: 输一次 RoamBench 密码,之后 worker 常驻自启。
# 用法: bash ~/.local/bin/setup-roambench-worker.sh
set -e

URL="http://127.0.0.1:3000"
USER="ian"
TOKEN_FILE="$HOME/.config/roambench/agent-token"
SERVICE_DIR="$HOME/.config/systemd/user"
SERVICE="$SERVICE_DIR/roambench-worker.service"
WORKER="$HOME/.local/bin/roambench-worker"

echo "== RoamBench worker 安装 =="
printf 'RoamBench 密码(%s): ' "$USER"
read -rs PASSWORD
echo

COOKIE=$(mktemp)
trap 'rm -f "$COOKIE"' EXIT

LOGIN=$(curl -s -c "$COOKIE" -X POST "$URL/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$USER\",\"password\":\"$PASSWORD\"}")
case "$LOGIN" in
    *authenticated*true*) echo "✓ 登录成功" ;;
    *) echo "✗ 登录失败: $LOGIN"; exit 1 ;;
esac

TOKEN=$(curl -s -b "$COOKIE" -X POST "$URL/api/project-control/agent-token" \
    | python3 -c 'import json,sys;print(json.load(sys.stdin).get("token",""))')
[ -n "$TOKEN" ] || { echo "✗ token 生成失败"; exit 1; }
echo "✓ agent token 已获取"

mkdir -p "$(dirname "$TOKEN_FILE")"
printf '%s' "$TOKEN" > "$TOKEN_FILE"
chmod 600 "$TOKEN_FILE"
echo "✓ token 已存 $TOKEN_FILE"

mkdir -p "$SERVICE_DIR"
cat > "$SERVICE" <<EOF
[Unit]
Description=RoamBench local task worker (claims board tasks, runs them in a visible terminal)
After=roambench.service

[Service]
Environment=ROAMBENCH_URL=$URL
Environment=ROAMBENCH_TOKEN=$TOKEN
ExecStart=$WORKER
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
chmod 600 "$SERVICE"

systemctl --user daemon-reload
systemctl --user enable --now roambench-worker.service
sleep 2
systemctl --user is-active roambench-worker.service && echo "✓ worker 已启动并设为开机自启"
echo
echo "查看运行日志:  journalctl --user -u roambench-worker -f"
echo "看板发起新任务后,worker 会自动领取并在任务终端里执行。"
