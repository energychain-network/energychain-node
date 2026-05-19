#!/bin/bash
#
# seed_testnet.sh — Runtime seeding of the local testnet (run AFTER
# ./init_testnet.sh and ./start_testnet.sh).
#
# init_testnet.sh seeds the *governance side* of the chain at genesis
# time (one default sanctions list, one allow-all policy, three oracle
# topics).
#
# This script seeds the *operational side* by sending real Msgs from
# the team account:
#
#   1. register team DID document + a "team-as-issuer" verification method
#   2. register team as an oracle provider on the three pre-seeded topics
#   3. (optional) register a sample stablecoin issuer/denom
#   4. (optional) register a sample EAC issuer
#
# Every step is idempotent on failure: rerun with `--continue` to skip
# already-registered objects. Per-step failures log a warning and
# continue rather than abort, so a partially-bootstrapped testnet can
# be re-seeded without manual cleanup.
#
# Usage:
#   ./seed_testnet.sh                # full seed
#   ./seed_testnet.sh --dry-run      # print the txs but don't broadcast
#   ./seed_testnet.sh --only=did     # only run a single phase
#

set -uo pipefail

# ─────────────────────────── Configuration ───────────────────────────

CHAIN_ID="${CHAIN_ID:-energychain_9001-1}"
BINARY="${BINARY:-energychaind}"
KEYRING_BACKEND="${KEYRING_BACKEND:-test}"
HOME_DIR="${HOME_DIR:-${HOME}/.energychain}"
NODE_HOME="${HOME_DIR}/node0"
RPC_NODE="${RPC_NODE:-tcp://127.0.0.1:26657}"
DENOM="${DENOM:-uecy}"
GAS_PRICES="${GAS_PRICES:-10000000000${DENOM}}"
KEY_NAME="${KEY_NAME:-team}"

DRY_RUN=0
ONLY=""
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=1 ;;
        --only=*) ONLY="${arg#--only=}" ;;
        *) echo "unknown arg: $arg" >&2; exit 2 ;;
    esac
done

# ─────────────────────────── Helpers ───────────────────────────

log() {
    echo -e "\033[1;32m[SEED]\033[0m $1"
}

warn() {
    echo -e "\033[1;33m[SEED]\033[0m $1" >&2
}

err() {
    echo -e "\033[1;31m[ERROR]\033[0m $1" >&2
    exit 1
}

want() {
    # want <phase> — returns 0 if phase should run.
    [ -z "$ONLY" ] || [ "$ONLY" = "$1" ]
}

submit_tx() {
    # submit_tx <description> <module> <subcommand> <args...>
    local desc="$1"; shift
    log "tx: ${desc}"
    if [ "$DRY_RUN" = "1" ]; then
        echo "  $BINARY tx" "$@" "--from=$KEY_NAME --keyring-backend=$KEYRING_BACKEND --home=$NODE_HOME --chain-id=$CHAIN_ID --node=$RPC_NODE --gas=auto --gas-adjustment=1.5 --gas-prices=$GAS_PRICES --yes"
        return 0
    fi
    local out
    out=$($BINARY tx "$@" \
        --from="$KEY_NAME" \
        --keyring-backend="$KEYRING_BACKEND" \
        --home="$NODE_HOME" \
        --chain-id="$CHAIN_ID" \
        --node="$RPC_NODE" \
        --gas=auto \
        --gas-adjustment=1.5 \
        --gas-prices="$GAS_PRICES" \
        --yes 2>&1) || { warn "${desc}: tx submission failed → ${out}"; return 1; }
    local code
    code=$(echo "$out" | grep -E '"code"' | head -1 | sed -E 's/.*"code": ?([0-9]+).*/\1/')
    if [ -n "$code" ] && [ "$code" != "0" ]; then
        warn "${desc}: tx accepted but check_tx code=${code} → ${out}"
        return 1
    fi
    sleep 2
    return 0
}

# ─────────────────────────── Pre-flight ───────────────────────────

if ! command -v "$BINARY" >/dev/null 2>&1; then
    err "${BINARY} not found in PATH"
fi
if ! command -v jq >/dev/null 2>&1; then
    err "jq is required"
fi
if [ ! -d "$NODE_HOME" ]; then
    err "${NODE_HOME} not found. Run ./init_testnet.sh first."
fi
TEAM_ADDR=$($BINARY keys show "$KEY_NAME" --keyring-backend="$KEYRING_BACKEND" --home="$NODE_HOME" --address 2>/dev/null) \
    || err "key '${KEY_NAME}' not in keyring of ${NODE_HOME}"
log "Seeding from ${KEY_NAME} (${TEAM_ADDR})"

# ─────────────────────────── Phase 1: DID ───────────────────────────
#
# Register a baseline DID document for the team account. Most native
# modules treat a registered DID as evidence of a known operator
# (x/eac, x/stablecoin, x/policy bindings). The DID id format here
# matches the documented `did:energy:<bech32>` convention.

if want did; then
    DID_ID="did:energy:${TEAM_ADDR}"
    submit_tx "register team DID (${DID_ID})" did register-document \
        "${DID_ID}" \
        "${TEAM_ADDR}" \
        '[]' \
        '[]' \
        '[]' \
        || warn "did register skipped (may already exist)"
fi

# ─────────────────────────── Phase 2: Oracle ───────────────────────────
#
# Register the team account as an oracle provider on each of the three
# pre-seeded topics. Each provider must bond at least Params.bond_min_amount
# of Params.bond_denom (defaults: 0 of uecy, so no actual escrow on
# testnet). Operators tightening reserves should override BOND_AMOUNT
# from the environment before running.

if want oracle; then
    BOND_AMOUNT="${BOND_AMOUNT:-1000000${DENOM}}"
    submit_tx "register team as oracle provider (bond ${BOND_AMOUNT})" \
        oracle register-provider \
        "team-operator" \
        "https://team.energychain.local" \
        "${BOND_AMOUNT}" \
        || warn "oracle register-provider skipped (may already exist)"

    for topic in power.spot.day_ahead usd.reserve.bank_attest eac.bridge.attest; do
        submit_tx "subscribe team-operator to topic ${topic}" \
            oracle subscribe-provider \
            "team-operator" "${topic}" \
            || warn "oracle subscribe-provider ${topic} skipped"
    done
fi

# ─────────────────────────── Phase 3: Stablecoin (sample) ───────────────────────────
#
# Optional: register the team account as a sample stablecoin issuer
# and create a USDX denom with a 100M supply cap. Off-by-default
# because mainnet operators want to drive this via gov proposals.

if want stablecoin; then
    submit_tx "register sample stablecoin issuer (team)" \
        stablecoin register-issuer \
        "team-stablecoin-issuer" \
        "${TEAM_ADDR}" \
        "US" \
        "https://team.energychain.local/sc" \
        || warn "stablecoin register-issuer skipped"

    submit_tx "register sample USDX denom (issuer=team)" \
        stablecoin register-denom \
        "team-stablecoin-issuer" \
        "USDX" \
        "Energy Chain Team USDX (testnet)" \
        "100000000000000" \
        "USD" \
        || warn "stablecoin register-denom USDX skipped"
fi

# ─────────────────────────── Phase 4: EAC (sample) ───────────────────────────

if want eac; then
    submit_tx "register sample EAC issuer (team)" \
        eac register-issuer \
        "team-eac-issuer" \
        "${TEAM_ADDR}" \
        "US" \
        "https://team.energychain.local/eac" \
        || warn "eac register-issuer skipped"
fi

# ─────────────────────────── Summary ───────────────────────────

echo ""
echo "=============================================="
echo "  Energy Chain Testnet Seeded"
echo "=============================================="
echo "  Team account: ${TEAM_ADDR}"
echo ""
echo "  Verify with:"
echo "    ${BINARY} q oracle topics --node=${RPC_NODE}"
echo "    ${BINARY} q policy policies --node=${RPC_NODE}"
echo "    ${BINARY} q sanctions lists --node=${RPC_NODE}"
echo "    ${BINARY} q did documents --node=${RPC_NODE}"
echo "    ${BINARY} q stablecoin denoms --node=${RPC_NODE}"
echo "    ${BINARY} q eac issuers --node=${RPC_NODE}"
echo ""
echo "  Note: per-module CLI argument signatures may evolve."
echo "  This script is a template — adapt arg ordering to match"
echo "  your installed binary via '${BINARY} tx <module> --help'."
echo "=============================================="
