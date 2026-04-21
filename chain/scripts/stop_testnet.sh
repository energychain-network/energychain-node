#!/bin/bash
#
# stop_testnet.sh — Gracefully stop all Energy Chain testnet validators.
#
# Sends SIGTERM first, waits up to 10 seconds, then SIGKILL if still running.
#
# Usage: ./stop_testnet.sh

set -euo pipefail

# ─────────────────────────── Configuration ───────────────────────────

NUM_VALIDATORS=4
HOME_DIR="${HOME}/.energychain"
SHUTDOWN_TIMEOUT=10

# ─────────────────────────── Helper Functions ───────────────────────────

log() {
    echo -e "\033[1;33m[STOP]\033[0m $1"
}

node_home() {
    echo "${HOME_DIR}/node${1}"
}

# ─────────────────────────── Stop Validators ───────────────────────────

log "Stopping ${NUM_VALIDATORS} validators..."
echo ""

STOPPED=0

for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    PIDFILE="$(node_home $i)/node.pid"

    if [ ! -f "$PIDFILE" ]; then
        log "  validator-${i}: no PID file found, skipping"
        continue
    fi

    PID=$(cat "$PIDFILE")

    if ! kill -0 "$PID" 2>/dev/null; then
        log "  validator-${i}: not running (stale PID ${PID})"
        rm -f "$PIDFILE"
        continue
    fi

    # Send SIGTERM for graceful shutdown
    log "  validator-${i}: sending SIGTERM to PID ${PID}..."
    kill -TERM "$PID" 2>/dev/null || true

    # Wait for process to exit
    WAITED=0
    while kill -0 "$PID" 2>/dev/null && [ "$WAITED" -lt "$SHUTDOWN_TIMEOUT" ]; do
        sleep 1
        WAITED=$((WAITED + 1))
    done

    if kill -0 "$PID" 2>/dev/null; then
        log "  validator-${i}: still running after ${SHUTDOWN_TIMEOUT}s, sending SIGKILL..."
        kill -9 "$PID" 2>/dev/null || true
        sleep 1
    fi

    if ! kill -0 "$PID" 2>/dev/null; then
        log "  validator-${i}: stopped (PID ${PID})"
        STOPPED=$((STOPPED + 1))
    else
        log "  validator-${i}: WARNING — could not stop PID ${PID}"
    fi

    rm -f "$PIDFILE"
done

echo ""
log "Stopped ${STOPPED}/${NUM_VALIDATORS} validators."

# Clean up any remaining energychaind processes (fallback)
REMAINING=$(pgrep -f "energychaind start" 2>/dev/null | wc -l | tr -d ' ')
if [ "$REMAINING" -gt 0 ]; then
    echo ""
    log "Found ${REMAINING} remaining energychaind process(es)."
    log "Run 'pkill -f \"energychaind start\"' to force-kill all."
fi
