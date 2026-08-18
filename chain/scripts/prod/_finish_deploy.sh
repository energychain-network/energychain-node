#!/usr/bin/env bash
# Wait out the intermittent block on the cross-border link, then finish the
# devnet bringup: top up whatever repo content is still missing, verify the
# chain source is complete, and launch the detached deploy.
#
# The link to the host passes traffic in windows of a few minutes separated by
# stalls of half an hour or more, so every step polls until it succeeds rather
# than failing the run.
#
#   PRIMCAST_KEY=... PRIMCAST_HOST=ubuntu@1.2.3.4 PUBLIC_HOST=1.2.3.4 \
#     bash _finish_deploy.sh
set -uo pipefail

KEY="${PRIMCAST_KEY:?set PRIMCAST_KEY}"
HOST="${PRIMCAST_HOST:?set PRIMCAST_HOST}"
PUBLIC_HOST="${PUBLIC_HOST:?set PUBLIC_HOST}"
SRC_ROOT="${SRC_ROOT:-$HOME/Desktop/code/nd-pro}"
MAX_WAIT_MIN="${MAX_WAIT_MIN:-180}"

SSH_CMD="ssh -i $KEY -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
SSH_CMD="$SSH_CMD -o LogLevel=ERROR -o ServerAliveInterval=15 -o ConnectTimeout=15"

log() { echo "[$(date '+%H:%M:%S')] $*"; }

deadline=$(( $(date +%s) + MAX_WAIT_MIN * 60 ))

# wait_link — block until a trivial remote command succeeds.
wait_link() {
  local n=0
  while :; do
    # shellcheck disable=SC2086
    $SSH_CMD "$HOST" 'true' >/dev/null 2>&1 && { [ "$n" -gt 0 ] && log "link back after ${n} probe(s)"; return 0; }
    n=$((n + 1))
    [ "$(date +%s)" -ge "$deadline" ] && { log "no window within ${MAX_WAIT_MIN}m"; return 1; }
    sleep 20
  done
}

# try_remote <cmd> — run a remote command, waiting for a window first.
try_remote() {
  local i
  for i in $(seq 1 40); do
    wait_link || return 1
    # shellcheck disable=SC2086
    $SSH_CMD "$HOST" "$@" && return 0
    sleep 10
  done
  return 1
}

sync_one() {
  local src="$1" dst="$2" i
  for i in $(seq 1 40); do
    wait_link || return 1
    rsync -az --partial --timeout=120 -e "$SSH_CMD" \
      --exclude=.git \
      --exclude=node_modules \
      --exclude=.next \
      --exclude=.DS_Store \
      `# anchored: an unanchored "energychaind" also drops cmd/energychaind` \
      --exclude=/energychaind \
      --exclude=tsconfig.tsbuildinfo \
      --exclude='*.bak' \
      "$src/" "${HOST}:energychain/${dst}/" && { log "$dst synced"; return 0; }
    log "$dst rsync attempt $i failed; waiting for another window"
    sleep 10
  done
  return 1
}

log "=== topping up repos ==="
sync_one "$SRC_ROOT/energychain-node/chain" chain    || { log "chain sync failed"; exit 1; }
sync_one "$SRC_ROOT/energychain-explorer"   explorer || { log "explorer sync failed"; exit 1; }
sync_one "$SRC_ROOT/energychain-dex"        dex      || { log "dex sync failed"; exit 1; }

log "=== verifying source completeness ==="
try_remote 'test -d ~/energychain/chain/cmd/energychaind \
  && test -f ~/energychain/chain/go.mod \
  && test -d ~/energychain/explorer/deploy \
  && test -d ~/energychain/dex/deploy \
  && test -d ~/energychain/dex/web \
  && echo SOURCE_COMPLETE' || { log "source incomplete"; exit 1; }

log "=== stopping any previous run ==="
# A failed bringup leaves ec-node in a Restart=always crash loop, which both
# spams the journal and can hold the ports the fresh node needs.
try_remote 'sudo systemctl stop ec-loadgen 2>/dev/null; sudo systemctl stop ec-node 2>/dev/null; \
  pkill -f "energychaind start" 2>/dev/null; sleep 2; echo STOPPED' || true

log "=== launching detached deploy ==="
try_remote "cd ~/energychain/chain/scripts/prod \
  && chmod +x *.sh \
  && rm -f ~/primcast-deploy.log ~/primcast-deploy.phase \
  && PUBLIC_HOST=${PUBLIC_HOST} nohup setsid bash _deploy_all.sh > ~/primcast-deploy.log 2>&1 < /dev/null & \
  sleep 4; pgrep -f _deploy_all.sh >/dev/null && echo LAUNCHED" \
  || { log "launch failed"; exit 1; }

log "=== deploy running; polling phase ==="
last=""
while :; do
  ph=$(try_remote 'cat ~/primcast-deploy.phase 2>/dev/null' 2>/dev/null | tail -1)
  if [ -n "$ph" ] && [ "$ph" != "$last" ]; then
    log "phase: $ph"
    last="$ph"
  fi
  case "$ph" in
    DONE)    log "DEPLOY COMPLETE"; break ;;
    FAILED*) log "DEPLOY FAILED: $ph"; break ;;
  esac
  [ "$(date +%s)" -ge "$deadline" ] && { log "polling deadline reached (deploy may still be running)"; break; }
  sleep 30
done

log "=== tail of remote log ==="
try_remote 'tail -40 ~/primcast-deploy.log' || true
