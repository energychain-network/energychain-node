#!/bin/bash
# Step 5 (run by EACH VALIDATOR after 04_verify_genesis.sh succeeds).
#
# Starts the validator with production-grade flags. Reads node-specific
# values from the environment so this script is reusable across every
# validator without per-machine edits.
#
# Required:
#   PEERS=<comma-separated list of node-id@host:port from coordinator>
#
# Optional:
#   PRUNING="custom"                         # default; use "nothing" only for archive nodes
#   PRUNING_KEEP_RECENT=362880               # ~21 days at 5s blocks
#   PRUNING_INTERVAL=10
#   SNAPSHOT_INTERVAL=1000                   # state-sync snapshots every 1000 blocks
#   SNAPSHOT_KEEP_RECENT=2
#   INV_CHECK_PERIOD=1000                    # crisis invariant check every 1000 blocks
#   LOG_LEVEL=info
#   JSON_RPC_API=eth,net,web3                # txpool/admin disabled by default

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/00_params.sh"

: "${PEERS:?PEERS is required (comma-separated nodeID@host:port)}"

PRUNING="${PRUNING:-custom}"
PRUNING_KEEP_RECENT="${PRUNING_KEEP_RECENT:-362880}"
PRUNING_INTERVAL="${PRUNING_INTERVAL:-10}"
SNAPSHOT_INTERVAL="${SNAPSHOT_INTERVAL:-1000}"
SNAPSHOT_KEEP_RECENT="${SNAPSHOT_KEEP_RECENT:-2}"
INV_CHECK_PERIOD="${INV_CHECK_PERIOD:-1000}"
LOG_LEVEL="${LOG_LEVEL:-info}"
JSON_RPC_API="${JSON_RPC_API:-eth,net,web3}"

VAL_HOME="$HOME/.energychaind"
APP_TOML="$VAL_HOME/config/app.toml"
CONFIG_TOML="$VAL_HOME/config/config.toml"

# Pre-flight tweaks. We use sed-with-backup because BSD/GNU sed differ on -i.
update_app_toml() {
  sed -i.bak "s|^persistent_peers = .*|persistent_peers = \"$PEERS\"|" "$CONFIG_TOML" 2>/dev/null || true
  sed -i.bak "s|^pruning = .*|pruning = \"$PRUNING\"|" "$APP_TOML"
  sed -i.bak "s|^pruning-keep-recent = .*|pruning-keep-recent = \"$PRUNING_KEEP_RECENT\"|" "$APP_TOML"
  sed -i.bak "s|^pruning-interval = .*|pruning-interval = \"$PRUNING_INTERVAL\"|" "$APP_TOML"
  sed -i.bak "s|^snapshot-interval = .*|snapshot-interval = $SNAPSHOT_INTERVAL|" "$APP_TOML"
  sed -i.bak "s|^snapshot-keep-recent = .*|snapshot-keep-recent = $SNAPSHOT_KEEP_RECENT|" "$APP_TOML"
  sed -i.bak "s|^prometheus = .*|prometheus = true|" "$CONFIG_TOML"
  rm -f "$APP_TOML.bak" "$CONFIG_TOML.bak" 2>/dev/null || true
}
update_app_toml

echo "==> Starting validator with PEERS=$PEERS"
exec "$BINARY" start \
  --home "$VAL_HOME" \
  --chain-id "$CHAIN_ID" \
  --log_level "$LOG_LEVEL" \
  --pruning "$PRUNING" \
  --inv-check-period "$INV_CHECK_PERIOD" \
  --minimum-gas-prices "${MIN_GAS_PRICE}${DENOM}" \
  --json-rpc.api "$JSON_RPC_API"
