#!/usr/bin/env bash
# Push the three repos (chain / explorer / dex) to a devnet host, with retries.
#
# Run with bash, not zsh: zsh does not word-split unquoted parameters and it
# treats "$VAR:e" as a history modifier, both of which silently corrupt the
# ssh/rsync invocations this needs.
#
#   PRIMCAST_KEY=... PRIMCAST_HOST=ubuntu@1.2.3.4 bash _sync_repos.sh
set -uo pipefail

KEY="${PRIMCAST_KEY:?set PRIMCAST_KEY to the .pem path}"
HOST="${PRIMCAST_HOST:?set PRIMCAST_HOST to user@ip}"
SRC_ROOT="${SRC_ROOT:-$HOME/Desktop/code/nd-pro}"
TRIES="${TRIES:-6}"

SSH_CMD="ssh -i $KEY -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
SSH_CMD="$SSH_CMD -o LogLevel=ERROR -o ServerAliveInterval=15 -o ConnectTimeout=20"

remote() {
  local i
  for i in $(seq 1 "$TRIES"); do
    # shellcheck disable=SC2086
    $SSH_CMD "$HOST" "$@" && return 0
    echo "  [remote] retry $i/$TRIES" >&2
    sleep $((i * 5))
  done
  return 1
}

sync_one() {
  local src="$1" dst="$2" i
  echo "=== $dst ==="
  for i in $(seq 1 "$TRIES"); do
    rsync -az --partial --timeout=120 -e "$SSH_CMD" \
      --exclude=.git \
      --exclude=node_modules \
      --exclude=.next \
      --exclude=.DS_Store \
      `# anchored: an unanchored "energychaind" also drops cmd/energychaind` \
      --exclude=/energychaind \
      --exclude=tsconfig.tsbuildinfo \
      --exclude='*.bak' \
      "$src/" "${HOST}:energychain/${dst}/" && { echo "  $dst OK"; return 0; }
    echo "  retry $i/$TRIES"
    sleep $((i * 5))
  done
  echo "  $dst FAILED"
  return 1
}

remote 'mkdir -p ~/energychain/chain ~/energychain/explorer ~/energychain/dex && echo DIRS_READY' \
  || { echo "cannot reach host"; exit 1; }

rc=0
sync_one "$SRC_ROOT/energychain-node/chain"  chain    || rc=1
sync_one "$SRC_ROOT/energychain-explorer"    explorer || rc=1
sync_one "$SRC_ROOT/energychain-dex"         dex      || rc=1

echo "=== remote sizes ==="
remote 'du -sh ~/energychain/*'
exit "$rc"
