#!/usr/bin/env bash
# Retrying ssh/rsync wrapper for the Primcast devnet host.
#
# The link to us-east-1 drops often enough that a bare `ssh host cmd` fails
# several times an hour mid-transfer. Every call here is retried, and long jobs
# are expected to run detached on the host (see _deploy_all.sh) so a dropped
# control connection never kills the work.
#
#   _ecssh.sh run  '<remote command>'   -- run a command, retrying on failure
#   _ecssh.sh put  <local> <remote>     -- rsync a path up, resumable
set -uo pipefail

KEY="${PRIMCAST_KEY:-$HOME/Desktop/code/Primcast.pem}"
HOST="${PRIMCAST_HOST:-ubuntu@54.90.254.198}"
TRIES="${TRIES:-6}"

SSH_ARGS=(
  -i "$KEY"
  -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null
  -o LogLevel=ERROR
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=8
  -o TCPKeepAlive=yes
  -o ConnectTimeout=20
  -o ConnectionAttempts=3
)

case "${1:-}" in
  run)
    shift
    for i in $(seq 1 "$TRIES"); do
      ssh "${SSH_ARGS[@]}" "$HOST" "$@" && exit 0
      echo "[ecssh] attempt $i/$TRIES failed; retrying in $((i * 5))s" >&2
      sleep $((i * 5))
    done
    echo "[ecssh] giving up after $TRIES attempts" >&2
    exit 1
    ;;
  put)
    shift
    src="$1"; dst="$2"
    for i in $(seq 1 "$TRIES"); do
      # --partial keeps half-sent files so a retry resumes instead of restarting.
      rsync -az --partial --timeout=90 \
        -e "ssh ${SSH_ARGS[*]}" \
        --exclude=.git \
        --exclude=node_modules \
        --exclude=.next \
        --exclude=.DS_Store \
        --exclude=energychaind \
        --exclude=tsconfig.tsbuildinfo \
        --exclude='*.bak' \
        "$src" "$HOST:$dst" && exit 0
      echo "[ecssh] rsync attempt $i/$TRIES failed; retrying in $((i * 5))s" >&2
      sleep $((i * 5))
    done
    echo "[ecssh] rsync gave up after $TRIES attempts" >&2
    exit 1
    ;;
  *)
    echo "usage: _ecssh.sh {run <cmd> | put <local> <remote>}" >&2
    exit 2
    ;;
esac
