#!/bin/bash
#
# init_testnet.sh — Initialize a 4-validator local testnet for Energy Chain.
#
# This script sets up a complete local testnet from scratch:
#   1. Cleans previous state
#   2. Initializes 4 validator nodes
#   3. Creates validator keys and genesis accounts
#   4. Generates and collects genesis transactions
#   5. Distributes the final genesis to all nodes
#   6. Configures networking (peers, ports)
#
# Usage: ./init_testnet.sh

set -euo pipefail

# ─────────────────────────── Configuration ───────────────────────────

CHAIN_ID="energychain_9001-1"
DENOM="uecy"
BINARY="energychaind"
NUM_VALIDATORS=4
KEYRING_BACKEND="test"

# 25M ECY per validator (18 decimals)
VALIDATOR_STAKE="25000000000000000000000000${DENOM}"

# Genesis account allocations (matching chain_params.json distribution)
TEAM_AMOUNT="150000000000000000000000000${DENOM}"     # 150M ECY — Team (15%)
ECOSYSTEM_AMOUNT="300000000000000000000000000${DENOM}" # 300M ECY — Ecosystem (30%)
TREASURY_AMOUNT="200000000000000000000000000${DENOM}"  # 200M ECY — Treasury (20%)
CIRCULATION_AMOUNT="250000000000000000000000000${DENOM}" # 250M ECY — Circulation (25%)
VALIDATOR_ALLOC="25000000000000000000000000${DENOM}"   # 25M ECY per validator

HOME_DIR="${HOME}/.energychain"

# Port assignments per node (avoid conflicts when running on same host)
#                     Node0   Node1   Node2   Node3
P2P_PORTS=(          26656   26666   26676   26686)
RPC_PORTS=(          26657   26667   26677   26687)
GRPC_PORTS=(          9090    9091    9092    9093)
GRPC_WEB_PORTS=(      9091    9191    9291    9391)
API_PORTS=(           1317    1318    1319    1320)
EVM_RPC_PORTS=(       8545    8546    8547    8548)
EVM_WS_PORTS=(        8546    8556    8566    8576)
PPROF_PORTS=(         6060    6061    6062    6063)

# ─────────────────────────── Helper Functions ───────────────────────────

log() {
    echo -e "\033[1;32m[INIT]\033[0m $1"
}

err() {
    echo -e "\033[1;31m[ERROR]\033[0m $1" >&2
    exit 1
}

node_home() {
    echo "${HOME_DIR}/node${1}"
}

# ─────────────────────────── Step 1: Clean Up ───────────────────────────

log "Removing old testnet data..."
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    rm -rf "$(node_home $i)"
done
log "Cleanup complete."

# ─────────────────────────── Step 2: Initialize Nodes ───────────────────────────

log "Initializing ${NUM_VALIDATORS} validator nodes..."
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    NODE_HOME="$(node_home $i)"
    MONIKER="validator-${i}"

    $BINARY init "$MONIKER" \
        --chain-id "$CHAIN_ID" \
        --home "$NODE_HOME" \
        > /dev/null 2>&1

    log "  Node ${i} initialized: ${NODE_HOME}"
done

# ─────────────────────────── Step 3: Create Validator Keys ───────────────────────────

log "Creating validator keys..."
declare -a VALIDATOR_ADDRESSES
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    NODE_HOME="$(node_home $i)"
    KEY_NAME="validator${i}"

    $BINARY keys add "$KEY_NAME" \
        --keyring-backend "$KEYRING_BACKEND" \
        --home "$NODE_HOME" \
        --output json > "${NODE_HOME}/key_info.json" 2>&1

    ADDR=$($BINARY keys show "$KEY_NAME" \
        --keyring-backend "$KEYRING_BACKEND" \
        --home "$NODE_HOME" \
        --address)

    VALIDATOR_ADDRESSES+=("$ADDR")
    log "  Validator ${i} key created: ${ADDR}"
done

# ─────────────────────────── Step 4: Genesis Accounts ───────────────────────────

# Use node0 as the primary genesis builder
NODE0_HOME="$(node_home 0)"

log "Adding genesis accounts to node0..."

# Create additional keys for non-validator accounts on node0
$BINARY keys add team \
    --keyring-backend "$KEYRING_BACKEND" \
    --home "$NODE0_HOME" \
    > /dev/null 2>&1
TEAM_ADDR=$($BINARY keys show team --keyring-backend "$KEYRING_BACKEND" --home "$NODE0_HOME" --address)

$BINARY keys add ecosystem \
    --keyring-backend "$KEYRING_BACKEND" \
    --home "$NODE0_HOME" \
    > /dev/null 2>&1
ECOSYSTEM_ADDR=$($BINARY keys show ecosystem --keyring-backend "$KEYRING_BACKEND" --home "$NODE0_HOME" --address)

$BINARY keys add treasury \
    --keyring-backend "$KEYRING_BACKEND" \
    --home "$NODE0_HOME" \
    > /dev/null 2>&1
TREASURY_ADDR=$($BINARY keys show treasury --keyring-backend "$KEYRING_BACKEND" --home "$NODE0_HOME" --address)

$BINARY keys add circulation \
    --keyring-backend "$KEYRING_BACKEND" \
    --home "$NODE0_HOME" \
    > /dev/null 2>&1
CIRCULATION_ADDR=$($BINARY keys show circulation --keyring-backend "$KEYRING_BACKEND" --home "$NODE0_HOME" --address)

# Add team/ecosystem/treasury/circulation genesis accounts.
# Cosmos SDK 0.50+ relocated add-genesis-account / gentx /
# collect-gentxs / validate-genesis under the `genesis` subcommand.
$BINARY genesis add-genesis-account "$TEAM_ADDR" "$TEAM_AMOUNT" \
    --home "$NODE0_HOME" --keyring-backend "$KEYRING_BACKEND"
log "  Team account:        ${TEAM_ADDR}"

$BINARY genesis add-genesis-account "$ECOSYSTEM_ADDR" "$ECOSYSTEM_AMOUNT" \
    --home "$NODE0_HOME" --keyring-backend "$KEYRING_BACKEND"
log "  Ecosystem account:   ${ECOSYSTEM_ADDR}"

$BINARY genesis add-genesis-account "$TREASURY_ADDR" "$TREASURY_AMOUNT" \
    --home "$NODE0_HOME" --keyring-backend "$KEYRING_BACKEND"
log "  Treasury account:    ${TREASURY_ADDR}"

$BINARY genesis add-genesis-account "$CIRCULATION_ADDR" "$CIRCULATION_AMOUNT" \
    --home "$NODE0_HOME" --keyring-backend "$KEYRING_BACKEND"
log "  Circulation account: ${CIRCULATION_ADDR}"

# Add each validator's genesis account
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    $BINARY genesis add-genesis-account "${VALIDATOR_ADDRESSES[$i]}" "$VALIDATOR_ALLOC" \
        --home "$NODE0_HOME" --keyring-backend "$KEYRING_BACKEND"
    log "  Validator ${i} account: ${VALIDATOR_ADDRESSES[$i]}"
done

# ─────────────────────────── Step 5: Genesis Transactions ───────────────────────────

log "Creating genesis transactions..."

# Copy node0's genesis (with all accounts) to other nodes first
for i in $(seq 1 $((NUM_VALIDATORS - 1))); do
    cp "${NODE0_HOME}/config/genesis.json" "$(node_home $i)/config/genesis.json"
done

# Each validator creates a gentx
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    NODE_HOME="$(node_home $i)"
    KEY_NAME="validator${i}"

    $BINARY genesis gentx "$KEY_NAME" "$VALIDATOR_STAKE" \
        --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING_BACKEND" \
        --home "$NODE_HOME" \
        --moniker "validator-${i}" \
        --commission-rate "0.10" \
        --commission-max-rate "0.20" \
        --commission-max-change-rate "0.01" \
        --min-self-delegation "1" \
        > /dev/null 2>&1

    log "  Gentx created for validator-${i}"
done

# ─────────────────────────── Step 6: Collect Genesis Transactions ───────────────────────────

log "Collecting gentxs into node0..."

# Copy gentxs from other nodes to node0
for i in $(seq 1 $((NUM_VALIDATORS - 1))); do
    cp "$(node_home $i)/config/gentx/"*.json "${NODE0_HOME}/config/gentx/" 2>/dev/null || true
done

$BINARY genesis collect-gentxs --home "$NODE0_HOME" > /dev/null 2>&1

# Validate the genesis
$BINARY genesis validate-genesis --home "$NODE0_HOME"
log "Genesis validated successfully."

# ─────────────────────────── Step 7: Seed native module app_state ───────────────────────────
#
# At this point node0's genesis.json contains:
#   - every SDK / EVM module hydrated from the binary's default genesis
#   - every native module hydrated from its DefaultGenesis (empty entries)
#   - collected gentxs for all 4 validators
#
# Before we distribute the genesis we patch in the *minimum* governance
# seeds needed for the 22 native modules to be immediately useful on a
# fresh testnet:
#
#   - one default sanctions list  (OFAC_SDN_TESTNET, ACTIVE, 0 entries)
#   - one default compliance policy (ACTIVE, no rules ⇒ allow-all)
#   - three oracle topics (power-spot price, USD reserve attest,
#     EAC bridge attest) covering the three cross-module flows
#     (stablecoin, EAC issuance, market settlement) that need
#     external data on day 1
#
# All seeds use the team account as `created_by`; the rows are still
# governance-mutable post-genesis. ValidateGenesis is re-run at the
# end of this step to fail fast on shape regressions.

log "Seeding native module app_state..."

if ! command -v jq >/dev/null 2>&1; then
    err "jq is required to seed native module state. Install with: brew install jq"
fi

GENESIS="${NODE0_HOME}/config/genesis.json"
NOW="$(date -u +%s)"

# 7.a  sanctions: register an empty OFAC list so subsequent
# add-entry txs do not need governance to first create the list.
jq --argjson now "$NOW" \
   --arg creator "$TEAM_ADDR" \
   '.app_state.sanctions.lists = [
       {
           "id": "ofac_sdn_testnet",
           "name": "OFAC SDN (testnet seed)",
           "description": "Default testnet sanctions list. Entries added via x/sanctions MsgAddEntry under the gov authority.",
           "source_uri": "https://www.treasury.gov/ofac/downloads/sdn.xml",
           "jurisdiction": "US",
           "authority": $creator,
           "status": 1,
           "created_by": $creator,
           "created_at": $now,
           "updated_at": $now,
           "entry_count": 0
       }
   ]' "$GENESIS" > "${GENESIS}.tmp" && mv "${GENESIS}.tmp" "$GENESIS"

# 7.b  policy: register a default allow-all compliance policy.
# Empty rules ⇒ every transfer eval short-circuits to ALLOW. Operators
# bind real DSL rules via MsgUpdatePolicy or governance proposals.
jq --argjson now "$NOW" \
   --arg creator "$TEAM_ADDR" \
   '.app_state.policy.policies = [
       {
           "id": "default_compliance_v1",
           "name": "Default Compliance Pipeline",
           "description": "Allow-all testnet baseline. Replace with jurisdiction-specific DSL via x/policy MsgUpdatePolicy.",
           "version": "1",
           "status": 1,
           "rules": [],
           "created_by": $creator,
           "created_at": $now,
           "updated_at": $now
       }
   ]' "$GENESIS" > "${GENESIS}.tmp" && mv "${GENESIS}.tmp" "$GENESIS"

# 7.c  oracle: register the three day-1 topics. Providers must register
# + bond before they can submit (Step 7 of seed_testnet.sh).
jq --argjson now "$NOW" \
   '.app_state.oracle.topics = [
       {
           "id": "power.spot.day_ahead",
           "description": "Day-ahead spot power price (USD per MWh, 6-decimal fixed point).",
           "kind": 1,
           "aggregation": 0,
           "min_submissions": 3,
           "max_data_age_seconds": 600,
           "outlier_band_bps": 2000,
           "value_decimals": 6,
           "quote": "USD",
           "paused": false,
           "enabled": true,
           "created_at": $now,
           "updated_at": $now,
           "allow_list": []
       },
       {
           "id": "usd.reserve.bank_attest",
           "description": "Stablecoin reserve attestation (USD cents per stablecoin unit).",
           "kind": 4,
           "aggregation": 0,
           "min_submissions": 1,
           "max_data_age_seconds": 86400,
           "outlier_band_bps": 0,
           "value_decimals": 2,
           "quote": "USD",
           "paused": false,
           "enabled": true,
           "created_at": $now,
           "updated_at": $now,
           "allow_list": []
       },
       {
           "id": "eac.bridge.attest",
           "description": "Cross-registry EAC bridge attestation (opaque payload).",
           "kind": 0,
           "aggregation": 3,
           "min_submissions": 2,
           "max_data_age_seconds": 3600,
           "outlier_band_bps": 0,
           "value_decimals": 0,
           "quote": "",
           "paused": false,
           "enabled": true,
           "created_at": $now,
           "updated_at": $now,
           "allow_list": []
       }
   ]' "$GENESIS" > "${GENESIS}.tmp" && mv "${GENESIS}.tmp" "$GENESIS"

# Re-validate the patched genesis. If a future schema change breaks
# any of the seeds above, this will fail loudly before peers diverge.
$BINARY genesis validate-genesis --home "$NODE0_HOME"
log "Native module app_state seeded:"
log "  sanctions: 1 list (ofac_sdn_testnet, ACTIVE)"
log "  policy:    1 policy (default_compliance_v1, ACTIVE, allow-all)"
log "  oracle:    3 topics (power.spot.day_ahead, usd.reserve.bank_attest, eac.bridge.attest)"

# ─────────────────────────── Step 8: Distribute Genesis ───────────────────────────

log "Copying final genesis to all nodes..."
for i in $(seq 1 $((NUM_VALIDATORS - 1))); do
    cp "${NODE0_HOME}/config/genesis.json" "$(node_home $i)/config/genesis.json"
done
log "Genesis distributed."

# ─────────────────────────── Step 9: Configure Persistent Peers ───────────────────────────

log "Configuring persistent peers..."

# Collect node IDs
declare -a NODE_IDS
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    NODE_HOME="$(node_home $i)"
    NODE_ID=$($BINARY comet show-node-id --home "$NODE_HOME")
    NODE_IDS+=("$NODE_ID")
    log "  Node ${i} ID: ${NODE_ID}"
done

# Build peer strings and configure each node
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    NODE_HOME="$(node_home $i)"
    CONFIG_FILE="${NODE_HOME}/config/config.toml"
    APP_CONFIG="${NODE_HOME}/config/app.toml"

    # Build peer list excluding self
    PEERS=""
    for j in $(seq 0 $((NUM_VALIDATORS - 1))); do
        if [ "$i" != "$j" ]; then
            if [ -n "$PEERS" ]; then
                PEERS="${PEERS},"
            fi
            PEERS="${PEERS}${NODE_IDS[$j]}@127.0.0.1:${P2P_PORTS[$j]}"
        fi
    done

    # ─── config.toml: P2P and RPC ports ───

    # P2P listen address
    sed -i.bak "s|laddr = \"tcp://0.0.0.0:26656\"|laddr = \"tcp://0.0.0.0:${P2P_PORTS[$i]}\"|g" "$CONFIG_FILE"

    # RPC listen address
    sed -i.bak "s|laddr = \"tcp://127.0.0.1:26657\"|laddr = \"tcp://0.0.0.0:${RPC_PORTS[$i]}\"|g" "$CONFIG_FILE"

    # Persistent peers
    sed -i.bak "s|persistent_peers = \"\"|persistent_peers = \"${PEERS}\"|g" "$CONFIG_FILE"

    # Allow duplicate IPs (required for local testnet on same machine)
    sed -i.bak "s|allow_duplicate_ip = false|allow_duplicate_ip = true|g" "$CONFIG_FILE"

    # Enable CORS for local development
    sed -i.bak "s|cors_allowed_origins = \[\]|cors_allowed_origins = [\"*\"]|g" "$CONFIG_FILE"

    # Pprof listen address
    sed -i.bak "s|pprof_laddr = \"localhost:6060\"|pprof_laddr = \"localhost:${PPROF_PORTS[$i]}\"|g" "$CONFIG_FILE"

    # ─── app.toml: API, gRPC, and EVM ports ───

    # API server
    sed -i.bak "s|address = \"tcp://0.0.0.0:1317\"|address = \"tcp://0.0.0.0:${API_PORTS[$i]}\"|g" "$APP_CONFIG"
    sed -i.bak "s|address = \"tcp://localhost:1317\"|address = \"tcp://0.0.0.0:${API_PORTS[$i]}\"|g" "$APP_CONFIG"

    # gRPC server
    sed -i.bak "s|address = \"0.0.0.0:9090\"|address = \"0.0.0.0:${GRPC_PORTS[$i]}\"|g" "$APP_CONFIG"

    # gRPC-web server
    sed -i.bak "s|address = \"0.0.0.0:9091\"|address = \"0.0.0.0:${GRPC_WEB_PORTS[$i]}\"|g" "$APP_CONFIG"

    # EVM JSON-RPC
    sed -i.bak "s|address = \"0.0.0.0:8545\"|address = \"0.0.0.0:${EVM_RPC_PORTS[$i]}\"|g" "$APP_CONFIG"
    sed -i.bak "s|address = \"127.0.0.1:8545\"|address = \"0.0.0.0:${EVM_RPC_PORTS[$i]}\"|g" "$APP_CONFIG"

    # EVM WebSocket
    sed -i.bak "s|ws-address = \"0.0.0.0:8546\"|ws-address = \"0.0.0.0:${EVM_WS_PORTS[$i]}\"|g" "$APP_CONFIG"
    sed -i.bak "s|ws-address = \"127.0.0.1:8546\"|ws-address = \"0.0.0.0:${EVM_WS_PORTS[$i]}\"|g" "$APP_CONFIG"

    # Enable API and EVM JSON-RPC
    sed -i.bak '/^\[api\]$/,/^\[/ s|enable = false|enable = true|' "$APP_CONFIG"
    sed -i.bak '/^\[json-rpc\]$/,/^\[/ s|enable = false|enable = true|' "$APP_CONFIG"
    sed -i.bak '/^\[mempool\]$/,/^\[/ s|^type = .*|type = "app"|' "$CONFIG_FILE"
    sed -i.bak '/^\[mempool\]$/,/^\[/ s|^size = .*|size = 20000|' "$CONFIG_FILE"
    sed -i.bak '/^\[evm.mempool\]$/,/^\[/ s|^operate-exclusively = .*|operate-exclusively = true|' "$APP_CONFIG"

    # Minimum gas prices
    sed -i.bak "s|^minimum-gas-prices = .*|minimum-gas-prices = \"10000000000${DENOM}\"|g" "$APP_CONFIG"

    # Clean up sed backup files
    rm -f "${CONFIG_FILE}.bak" "${APP_CONFIG}.bak"

    log "  Node ${i} configured (P2P:${P2P_PORTS[$i]} RPC:${RPC_PORTS[$i]} EVM:${EVM_RPC_PORTS[$i]})"
done

# ─────────────────────────── Summary ───────────────────────────

echo ""
echo "=============================================="
echo "  Energy Chain Local Testnet Initialized"
echo "=============================================="
echo ""
echo "  Chain ID:     ${CHAIN_ID}"
echo "  Validators:   ${NUM_VALIDATORS}"
echo "  Home:         ${HOME_DIR}"
echo ""

for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
    echo "  validator-${i}:"
    echo "    Address:  ${VALIDATOR_ADDRESSES[$i]}"
    echo "    P2P:      127.0.0.1:${P2P_PORTS[$i]}"
    echo "    RPC:      http://127.0.0.1:${RPC_PORTS[$i]}"
    echo "    EVM RPC:  http://127.0.0.1:${EVM_RPC_PORTS[$i]}"
    echo ""
done

echo "  Genesis accounts:"
echo "    Team:         ${TEAM_ADDR}"
echo "    Ecosystem:    ${ECOSYSTEM_ADDR}"
echo "    Treasury:     ${TREASURY_ADDR}"
echo "    Circulation:  ${CIRCULATION_ADDR}"
echo ""
echo "  Next: run ./start_testnet.sh to start the network"
echo "=============================================="
