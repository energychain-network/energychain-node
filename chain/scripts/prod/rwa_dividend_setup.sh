#!/usr/bin/env bash
# ===========================================================================
# Create one rwatoken snapshot-dividend round for an existing RWA token.
#
# Runs FundPool -> TakeSnapshot -> CreateDistribution using the token admin
# key (default: read admin from chain and use matching testpack wallet).
#
#   TOKEN_ID=4 DIST_USDC=5000 ./rwa_dividend_setup.sh
# ===========================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

export BIN="${BIN:-$CHAIN_DIR/energychaind}"
export HOME_T="${HOME_T:-$HOME/.energychaind}"
export CHAINID="${CHAINID:-energychain_9001-1}"
export NODE="${NODE:-tcp://127.0.0.1:26657}"
export DENOM="${DENOM:-uecy}"
export KEYRING="${KEYRING:-test}"
export GAS_PRICES="${GAS_PRICES:-10000000000${DENOM}}"

source "${CHAIN_DIR}/scripts/test/lib.sh"

TOKEN_ID="${TOKEN_ID:-4}"
# Human usdc amounts (6dp on chain).
FUND_USDC="${FUND_USDC:-10000}"
DIST_USDC="${DIST_USDC:-5000}"
ADMIN_KEY="${ADMIN_KEY:-}"
MANIFEST="${MANIFEST:-$HOME_T/testpack.json}"

usdc_raw() { echo $(( ${1%.*} * 1000000 )); }

section "resolve token $TOKEN_ID admin"
TOK="$(q rwatoken token "$TOKEN_ID" | jq '.token // empty')"
[ -z "$TOK" ] && { echo "ERROR: token $TOKEN_ID not found"; exit 1; }
ADMIN_ADDR="$(echo "$TOK" | jq -r '.admin')"
SETTLE="$(echo "$TOK" | jq -r '.settlement_denom')"
SYM="$(echo "$TOK" | jq -r '.symbol')"
echo "  symbol=$SYM admin=$ADMIN_ADDR settlement=$SETTLE"

if [ -z "$ADMIN_KEY" ] && [ -f "$MANIFEST" ]; then
  ADMIN_KEY="$(jq -r --arg a "$ADMIN_ADDR" '.[]|select(.address==$a)|.name' "$MANIFEST" | head -1)"
fi
[ -z "$ADMIN_KEY" ] && ADMIN_KEY="dev0"
echo "  signing as key=$ADMIN_KEY"

FUND_RAW="$(usdc_raw "$FUND_USDC")"
DIST_RAW="$(usdc_raw "$DIST_USDC")"

section "fund pool +$FUND_USDC $SETTLE"
expect_ok "fund pool" "$ADMIN_KEY" rwatoken fund-pool "$TOKEN_ID" "$FUND_RAW"
wait_blocks 1 >/dev/null

section "take holder snapshot"
expect_ok "take snapshot" "$ADMIN_KEY" rwatoken snapshot "$TOKEN_ID"
wait_blocks 1 >/dev/null
# Snapshots use a global id sequence; pick the newest for this token.
# List by probing recent ids (global seq, typically small).
SNAP_ID=""
for sid in $(seq 20 -1 1); do
  stid="$(q rwatoken snapshot "$sid" 2>/dev/null | jq -r '.snapshot.token_id // empty')"
  [ "$stid" = "$TOKEN_ID" ] && { SNAP_ID="$sid"; break; }
done
[ -z "$SNAP_ID" ] && { echo "ERROR: could not find snapshot for token $TOKEN_ID"; exit 1; }
echo "  snapshot_id=$SNAP_ID"

section "create distribution $DIST_USDC $SETTLE against snapshot $SNAP_ID"
expect_ok "create distribution" "$ADMIN_KEY" rwatoken distribute "$TOKEN_ID" "$SNAP_ID" "$DIST_RAW"
wait_blocks 1 >/dev/null

DIST_ID=""
for did in $(seq 20 -1 1); do
  dtid="$(q rwatoken distribution "$did" 2>/dev/null | jq -r '.distribution.token_id // empty')"
  [ "$dtid" = "$TOKEN_ID" ] && { DIST_ID="$did"; break; }
done
echo "  distribution_id=$DIST_ID"
summary || true
echo "RWA_DIVIDEND_SETUP_DONE token=$TOKEN_ID snapshot=$SNAP_ID distribution=$DIST_ID"
