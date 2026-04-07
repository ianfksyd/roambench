#!/usr/bin/env bash
set -euo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly UNIT_SRC="${ROAMBENCH_UNIT_SRC:-$SCRIPT_DIR/roambench.service}"
readonly UNIT_DST="${ROAMBENCH_UNIT_DST:-/etc/systemd/system/roambench.service}"
readonly NEW_SERVICE="${ROAMBENCH_SERVICE_NAME:-roambench.service}"
readonly LEGACY_SERVICE="${LEGACY_SERVICE_NAME:-rstudio-server.service}"
readonly HEALTHCHECK_URL="${ROAMBENCH_HEALTHCHECK_URL:-http://127.0.0.1:8787/}"

rollback() {
  systemctl stop "$NEW_SERVICE" >/dev/null 2>&1 || true
  systemctl start "$LEGACY_SERVICE" >/dev/null 2>&1 || true
}

if [[ ! -f "$UNIT_SRC" ]]; then
  echo "RoamBench unit template not found: $UNIT_SRC" >&2
  exit 1
fi

install -D -m 0644 "$UNIT_SRC" "$UNIT_DST"
systemctl daemon-reload
systemctl enable "$NEW_SERVICE" >/dev/null

systemctl stop "$LEGACY_SERVICE"

if ! systemctl start "$NEW_SERVICE"; then
  rollback
  journalctl -u "$NEW_SERVICE" -n 80 --no-pager || true
  exit 1
fi

ready=0
for _ in $(seq 1 15); do
  if curl -fsS -o /dev/null "$HEALTHCHECK_URL"; then
    ready=1
    break
  fi
  sleep 1
done

if [[ "$ready" -ne 1 ]]; then
  rollback
  journalctl -u "$NEW_SERVICE" -n 80 --no-pager || true
  exit 1
fi

systemctl disable "$LEGACY_SERVICE" >/dev/null || true

echo "RoamBench is active at $HEALTHCHECK_URL"
