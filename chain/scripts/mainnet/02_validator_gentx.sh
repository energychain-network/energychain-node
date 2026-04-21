#!/bin/bash
# Step 2 (run by EACH VALIDATOR on their own machine).
#
# Inputs (must be set in the environment OR passed via flags):
#   PRE_GENESIS=/path/to/coordinator/pre-genesis/genesis.json
#   PRE_GENESIS_SHA256=<expected sha256>      <- from coordinator's announcement
#   MONIKER=<your validator name>
#   VAL_KEY_NAME=<your local key name in keyring>
#   STAKE_AMOUNT=<integer in $DENOM>          <- e.g. 100000000000000000000000 = 100k ECY
#
# Optional:
#   COMMISSION_RATE=0.05
#   COMMISSION_MAX_RATE=0.20
#   COMMISSION_MAX_CHANGE=0.01
#   MIN_SELF_DELEGATION=1
#   KEYRING=os                                <- default; "test" only for non-prod
#   GENESIS_BALANCE=<integer>                 <- defaults to STAKE_AMOUNT
#
# Output:
#   $GENTX_DIR/gentx-${MONIKER}-${OPADDR}.json    <- send back to coordinator

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/00_params.sh"

: "${PRE_GENESIS:?PRE_GENESIS is required}"
: "${PRE_GENESIS_SHA256:?PRE_GENESIS_SHA256 is required}"
: "${MONIKER:?MONIKER is required}"
: "${VAL_KEY_NAME:?VAL_KEY_NAME is required}"
: "${STAKE_AMOUNT:?STAKE_AMOUNT is required (integer, in $DENOM)}"

KEYRING="${KEYRING:-os}"
COMMISSION_RATE="${COMMISSION_RATE:-0.05}"
COMMISSION_MAX_RATE="${COMMISSION_MAX_RATE:-0.20}"
COMMISSION_MAX_CHANGE="${COMMISSION_MAX_CHANGE:-0.01}"
MIN_SELF_DELEGATION="${MIN_SELF_DELEGATION:-1}"
GENESIS_BALANCE="${GENESIS_BALANCE:-$STAKE_AMOUNT}"

VAL_HOME="$HOME/.energychaind"
mkdir -p "$VAL_HOME/config"

# Step 2a: verify pre-genesis hash before doing anything.
ACTUAL_HASH="$(sha256sum "$PRE_GENESIS" | awk '{print $1}')"
if [[ "$ACTUAL_HASH" != "$PRE_GENESIS_SHA256" ]]; then
  echo "FATAL: pre-genesis sha256 mismatch"
  echo "  expected: $PRE_GENESIS_SHA256"
  echo "  got:      $ACTUAL_HASH"
  echo "Refuse to proceed. Re-fetch pre-genesis from the coordinator."
  exit 1
fi

# Step 2b: init the local home if not already initialized; copy in the
# verified pre-genesis. We never trust whatever genesis `init` produces.
if [[ ! -f "$VAL_HOME/config/priv_validator_key.json" ]]; then
  "$BINARY" init "$MONIKER" --chain-id "$CHAIN_ID" --home "$VAL_HOME" >/dev/null 2>&1
fi
cp "$PRE_GENESIS" "$VAL_HOME/config/genesis.json"

# Step 2c: ensure the key exists & resolve its bech32 address.
if ! "$BINARY" keys show "$VAL_KEY_NAME" --keyring-backend "$KEYRING" --home "$VAL_HOME" >/dev/null 2>&1; then
  echo "FATAL: key '$VAL_KEY_NAME' not found in keyring '$KEYRING'."
  echo "Create it with:  $BINARY keys add $VAL_KEY_NAME --keyring-backend $KEYRING --algo eth_secp256k1"
  exit 1
fi
VAL_ADDR="$("$BINARY" keys show "$VAL_KEY_NAME" -a --keyring-backend "$KEYRING" --home "$VAL_HOME")"

# Step 2d: add a genesis account funding this validator.
"$BINARY" genesis add-genesis-account "$VAL_ADDR" "${GENESIS_BALANCE}${DENOM}" --home "$VAL_HOME"

# Step 2e: produce the gentx.
"$BINARY" genesis gentx "$VAL_KEY_NAME" "${STAKE_AMOUNT}${DENOM}" \
  --chain-id "$CHAIN_ID" \
  --keyring-backend "$KEYRING" \
  --home "$VAL_HOME" \
  --moniker "$MONIKER" \
  --commission-rate "$COMMISSION_RATE" \
  --commission-max-rate "$COMMISSION_MAX_RATE" \
  --commission-max-change-rate "$COMMISSION_MAX_CHANGE" \
  --min-self-delegation "$MIN_SELF_DELEGATION" \
  --gas-prices "${MIN_GAS_PRICE}${DENOM}"

GENTX_FILE="$(ls -t "$VAL_HOME/config/gentx/" | head -1)"
SRC="$VAL_HOME/config/gentx/$GENTX_FILE"

OUT_DIR="${GENTX_OUT_DIR:-$PWD/gentx-out}"
mkdir -p "$OUT_DIR"

# Pack the gentx + the validator-account-snippet that the coordinator needs
# to merge into accounts. We do NOT ship the priv_validator_key, only the
# signed transaction.
DEST="$OUT_DIR/gentx-${MONIKER}-${VAL_ADDR}.json"
cp "$SRC" "$DEST"

# Also export an account snippet for coordinator convenience.
ACCT_DEST="$OUT_DIR/account-${MONIKER}-${VAL_ADDR}.json"
cat > "$ACCT_DEST" <<JSON
{
  "moniker": "$MONIKER",
  "validator_address": "$VAL_ADDR",
  "amount": "$GENESIS_BALANCE",
  "denom": "$DENOM"
}
JSON

echo ""
echo "============================================================"
echo "GENTX READY"
echo "  validator address:  $VAL_ADDR"
echo "  gentx:              $DEST"
echo "  account snippet:    $ACCT_DEST"
echo "============================================================"
echo ""
echo "Send BOTH files to the ceremony coordinator over a verified channel."
echo "Do NOT send your priv_validator_key.json or any keyring file."
