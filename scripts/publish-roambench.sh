#!/usr/bin/env bash

set -euo pipefail

REPO="ianfksyd/roambench"
TAG="${ROAMBENCH_TAG:-v0.3.0}"
DRY_RUN=1
TARGETS="x,reddit,hn,ph,v2ex,cn"

usage() {
	echo "Usage: ${0} [--tag v0.3.0] [--targets x,reddit,hn,ph,v2ex,cn] [--send] [--dry-run]" >&2
	exit 1
}

while [[ $# -gt 0 ]]; do
	case "${1}" in
	--tag)
		TAG="${2}"
		shift 2
		;;
	--send)
		DRY_RUN=0
		shift
		;;
	--targets)
		TARGETS="${2}"
		shift 2
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	*)
		usage
		;;
	esac
done

RELEASE_URL="https://github.com/${REPO}/releases/tag/${TAG}"

X_TWEET=$(cat <<EOF
Keep your AI coding sessions running.

RoamBench is a lightweight, self-hosted remote workbench for terminal-first workflows.
Keep tmux sessions alive, reopen 2/4 split CLI panes, and reconnect from another device when you need to.

${RELEASE_URL}
EOF
)

X_TWEET2=$(cat <<EOF
Built for Codex, Claude Code, Kimi-CLI, OpenCode, and other long-running terminal workflows.
It sits between SSH and a heavy browser IDE when you just need to resume work, inspect output, or make a small fix.
EOF
)

EN_HN_TITLE="Show HN: RoamBench, a small self-hosted remote workbench between SSH and browser IDEs"
EN_HN_BODY=$(cat <<EOF
I built RoamBench because SSH felt too raw on mobile and full browser IDEs felt too heavy for long-running CLI workflows.

RoamBench keeps the pieces I actually wanted:

- tmux-backed session recovery
- 2/4-pane CLI workspaces for Codex, Claude Code, and similar tools
- lightweight file browsing and small edits next to the terminal
- a single-user self-hosted setup that stays fast to reopen from phone or laptop

Release:
${RELEASE_URL}
EOF
)

EN_POST_CN=$(cat <<EOF
我做了一个轻量自托管远程工作台 RoamBench，让 AI coding session 可以一直跑着，你随时都能从别的设备接回。

核心体验：
- 2/4 分屏 CLI 工作区并发
- tmux 会话持久，任务不中断
- 不用完整浏览器 IDE 也能看输出、补小修改
- Linux amd64 / arm64 静态发布包

下载体验：${RELEASE_URL}
EOF
)

REDDIT_TEMPLATE=$(cat <<EOF
I built RoamBench because I wanted reconnectable CLI sessions across devices without a heavy browser IDE.
It is a lightweight self-hosted remote workbench for terminal-first coding:

- 2/4 split terminal panes
- Persistent tmux sessions
- Lightweight file edits next to the terminal
- Linux static binaries for amd64 / arm64

Release notes: ${RELEASE_URL}
EOF
)

PH_TITLE="RoamBench: lightweight remote workbench for terminal-first AI coding"
PH_BODY=$(cat <<EOF
RoamBench helps terminal-first coders keep long-running work alive across desktop and phone.
Built for split-workspace CLI workflows, it keeps tmux sessions stable and supports quick remote edits without a heavy browser IDE.

Highlights:
- 2/4 pane workflows
- Mobile reconnection and monitoring
- Self-hosted Linux static release (amd64, arm64)

${RELEASE_URL}
EOF
)

post_x() {
	local text="${1}"
	if [[ -z "${ROAMBENCH_X_BEARER_TOKEN:-}" ]]; then
		echo "跳过 X 发帖：未设置 ROAMBENCH_X_BEARER_TOKEN（OAuth 2.0 user token）"
		return 0
	fi
	local payload
	payload=$(jq -nc --arg text "${text}" '{text:$text}')
	curl -sS \
		-H "Authorization: Bearer ${ROAMBENCH_X_BEARER_TOKEN}" \
		-H "Content-Type: application/json" \
		-d "${payload}" \
		"https://api.twitter.com/2/tweets"
}

post_if_enabled() {
	local target="${1}"
	case "${target}" in
	x)
		echo "=== X / Twitter ==="
		if [[ "${DRY_RUN}" -eq 1 ]]; then
			echo "[Dry-run] first post:"
			echo "${X_TWEET}"
			echo
			echo "[Dry-run] second post:"
			echo "${X_TWEET2}"
		else
			echo "发送 X 主帖..."
			post_x "${X_TWEET}"
			echo
			echo "发送 X 续发..."
			post_x "${X_TWEET2}"
		fi
		;;
	reddit)
		echo "=== Reddit Draft ==="
		echo "标题建议: RoamBench: lightweight remote terminal workbench"
		echo
		echo "${REDDIT_TEMPLATE}"
		;;
	hn)
		echo "=== Hacker News Draft ==="
		echo "标题:"
		echo "${EN_HN_TITLE}"
		echo
		echo "${EN_HN_BODY}"
		;;
	ph)
		echo "=== Product Hunt Draft ==="
		echo "标题: ${PH_TITLE}"
		echo
		echo "${PH_BODY}"
		;;
	v2ex)
		echo "=== V2EX Draft ==="
		echo "${EN_POST_CN}"
		;;
	cn)
		echo "=== 中文社区 Draft ==="
		echo "${EN_POST_CN}"
		;;
	*)
		echo "未知渠道: ${target}" >&2
		;;
	esac
}

IFS=',' read -r -a TARGET_ARR <<< "${TARGETS}"
for target in "${TARGET_ARR[@]}"; do
	post_if_enabled "${target}"
	echo
done
