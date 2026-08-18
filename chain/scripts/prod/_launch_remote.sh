#!/usr/bin/env bash
# Wait for a usable window on the flaky link, then push the updated prod
# scripts and launch the detached full-stack deploy on the host.
#
# The path to us-east-1 only passes traffic intermittently, so every step
# retries for up to ~40 minutes. Once _deploy_all.sh is launched with
# nohup+setsid it owns itself; later drops only affect polling.
set -uo pipefail

KEY="${PRIMCAST_KEY:-$HOME/Desktop/code/Primcast.pem}"
HOST="${PRIMCAST_HOST:-ubuntu@54.90.254.198}"
PORT="${PRIMCAST_PORT:-22}"
LOCAL_PROD="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MAX_WAIT_MIN="${MAX_WAIT_MIN:-40}"

SSH_ARGS=(
  -i "$KEY" -p "$PORT"
  -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null
  -o LogLevel=ERROR
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=6
  -o ConnectTimeout=15
)

deadline=$(( $(date +%s) + MAX_WAIT_MIN * 60 ))
attempt=0

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# --- 1. wait for a window ---------------------------------------------------
while :; do
  attempt=$((attempt + 1))
  if ssh "${SSH_ARGS[@]}" "$HOST" 'echo alive' >/dev/null 2>&1; then
    log "link up on port $PORT after $attempt attempt(s)"
    break
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    log "no window on port $PORT within ${MAX_WAIT_MIN}m — giving up"
    exit 1
  fi
  sleep 20
done

# --- 2. push the updated scripts -------------------------------------------
log "syncing prod scripts"
for i in $(seq 1 12); do
  rsync -az --partial --timeout=60 -e "ssh ${SSH_ARGS[*]}" \
    --exclude='*.bak' \
    "$LOCAL_PROD/" "$HOST:energychain/chain/scripts/prod/" && { log "scripts synced"; break; }
  log "rsync retry $i"
  sleep 15
done

# --- 3. launch detached ----------------------------------------------------
log "launching _deploy_all.sh detached"
for i in $(seq 1 12); do
  if ssh "${SSH_ARGS[@]}" "$HOST" \
      'cd ~/energychain/chain/scripts/prod \
       && chmod +x _deploy_all.sh \
       && rm -f ~/primcast-deploy.log ~/primcast-deploy.phase \
       && nohup setsid bash _deploy_all.sh > ~/primcast-deploy.log 2>&1 < /dev/null & \
       sleep 3; echo "launched pid=$(pgrep -f _deploy_all.sh | head -1)"'; then
    log "launched"
    exit 0
  fi
  log "launch retry $i"
  sleep 15
done
log "could not launch"
exit 1
