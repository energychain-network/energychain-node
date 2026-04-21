#!/bin/bash
#
# start_testnet.sh — Start all 4 validators of the Energy Chain local testnet.
#
# Each validator runs as a background process with logs written to
# $HOME/.energychain/nodeN/node.log
#
# Usage: ./start_testnet.sh

set -euo pipefail

# ─────────────────────────── Configuration ───────────────────────────

BINARY="energychaind"
NUM_VALIDATORS=4
HOME_DIR="${HOME}/.energychain"
LOG_DIR="${HOME_DIR}/logs"
JSON_RPC_API="${JSON_RPC_API:-eth,txpool,net,web3}"

# Port assignments (must match init_testnet.sh)
RPC_PORTS=(  26657  26667  26677  26687)
EVM_RPC_PORTS=( 8545  8546  8547  8548)
API_PORTS=(  1317   1318   1319   1320)

# ─────────────────────────── Helper Functions ───────────────────────────

log() {
    echo -e "\033[1;32m[START]\033[0m $1"
}

err() {
    echo -e "\033[1;31m[ERROR]\033[0m $1" >&2
    exit 1
}

node_home() {
    echo "${HOME_DIR}/node${1}"
}

# ─────────────────────────── Pre-flight Checks ───────────────────────────

if ! command -v "$BINARY" &> /dev/null; then
    err "${BINARY} not found in PATH. Build it first."
fi

# Check that nodes have been initialized
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    if [ ! -f "$(node_home $i)/config/genesis.json" ]; then
        err "Node ${i} not initialized. Run ./init_testnet.sh first."
    fi
done

# Check for already running instances
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    PIDFILE="$(node_home $i)/node.pid"
    if [ -f "$PIDFILE" ]; then
        PID=$(cat "$PIDFILE")
        if kill -0 "$PID" 2>/dev/null; then
            err "Validator ${i} already running (PID ${PID}). Run ./stop_testnet.sh first."
        else
            rm -f "$PIDFILE"
        fi
    fi
done

# ─────────────────────────── Start Validators ───────────────────────────

mkdir -p "$LOG_DIR"

log "Starting ${NUM_VALIDATORS} validators..."
echo ""

for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    NODE_HOME="$(node_home $i)"
    LOG_FILE="${LOG_DIR}/node${i}.log"
    PIDFILE="${NODE_HOME}/node.pid"

    $BINARY start \
        --home "$NODE_HOME" \
        --log_level "info" \
        --json-rpc.api "$JSON_RPC_API" \
        > "$LOG_FILE" 2>&1 &

    PID=$!
    echo "$PID" > "$PIDFILE"

    log "  validator-${i} started (PID: ${PID})"
    log "    Log: ${LOG_FILE}"
done

# Wait briefly for nodes to start
sleep 3

# ─────────────────────────── Health Check ───────────────────────────

echo ""
log "Checking node status..."
echo ""

ALL_HEALTHY=true
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    PIDFILE="$(node_home $i)/node.pid"
    PID=$(cat "$PIDFILE" 2>/dev/null || echo "")

    if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
        STATUS="\033[1;32mRUNNING\033[0m"
    else
        STATUS="\033[1;31mFAILED\033[0m"
        ALL_HEALTHY=false
    fi

    echo -e "  validator-${i}: ${STATUS} (PID: ${PID})"
done

# ─────────────────────────── Summary ───────────────────────────

echo ""
echo "=============================================="
echo "  Energy Chain Local Testnet"
echo "=============================================="
echo ""
echo "  RPC Endpoints:"
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    echo "    validator-${i}: http://127.0.0.1:${RPC_PORTS[$i]}"
done
echo ""
echo "  EVM JSON-RPC Endpoints:"
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    echo "    validator-${i}: http://127.0.0.1:${EVM_RPC_PORTS[$i]}"
done
echo ""
echo "  REST API Endpoints:"
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    echo "    validator-${i}: http://127.0.0.1:${API_PORTS[$i]}"
done
echo ""
echo "  Logs: ${LOG_DIR}/"
echo ""

if [ "$ALL_HEALTHY" = true ]; then
    echo "  All validators are running."
else
    echo "  WARNING: Some validators failed to start. Check logs."
fi

echo ""
echo "  Stop with: ./stop_testnet.sh"
echo "=============================================="
