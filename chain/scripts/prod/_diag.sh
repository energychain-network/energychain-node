#!/usr/bin/env bash
# Run a diagnostic command bundle on the devnet host, retrying until the
# intermittent cross-border link gives us a window.
#
#   PRIMCAST_KEY=... PRIMCAST_HOST=ubuntu@1.2.3.4 bash _diag.sh '<remote cmd>'
set -uo pipefail

KEY="${PRIMCAST_KEY:?set PRIMCAST_KEY}"
HOST="${PRIMCAST_HOST:?set PRIMCAST_HOST}"
TRIES="${TRIES:-60}"

SSH_CMD="ssh -i $KEY -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
SSH_CMD="$SSH_CMD -o LogLevel=ERROR -o ServerAliveInterval=15 -o ConnectTimeout=15"

for i in $(seq 1 "$TRIES"); do
  # shellcheck disable=SC2086
  $SSH_CMD "$HOST" "$@" && exit 0
  echo "[diag] no window (attempt $i/$TRIES)" >&2
  sleep 15
done
echo "[diag] gave up" >&2
exit 1
