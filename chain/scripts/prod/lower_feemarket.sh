#!/usr/bin/env bash
# ===========================================================================
# Lower the feemarket module's min_gas_price via governance so browser wallets
# (Keplr/OKX/Leap), whose default gas-price steps are tiny, can pay a fee that
# clears the chain's "minimum global fee" check.
#
# The feemarket MinGasPriceDecorator rejects cosmos txs whose fee is below
# (min_gas_price * gas). The chain ships with min_gas_price = 1e10 uecy/gas,
# which forces a 1e16 uecy fee for a 1M-gas tx — far above any wallet default.
#
# This submits a feemarket MsgUpdateParams that only changes min_gas_price
# (all other params preserved), fast-passes it (gov_pass), then proves the fix
# by broadcasting a low-fee bank tx that would previously have been rejected.
# ===========================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

export BIN="${BIN:-$(command -v energychaind || echo "$CHAIN_DIR/energychaind")}"
export HOME_T="${HOME_T:-$HOME/.energychaind}"
export CHAINID="${CHAINID:-energychain_9001-1}"
export NODE="${NODE:-tcp://127.0.0.1:26657}"
export DENOM="${DENOM:-uecy}"
export KEYRING="${KEYRING:-test}"
export GAS_PRICES="${GAS_PRICES:-10000000000${DENOM}}"

# shellcheck source=../test/lib.sh
source "${CHAIN_DIR}/scripts/test/lib.sh"

NEW_MIN="${NEW_MIN:-0.001000000000000000}"   # uecy per gas
TYPE_URL="/cosmos.evm.feemarket.v1.MsgUpdateParams"

section "current feemarket params"
CUR="$(q feemarket params | jq '.params // empty')"
[ -z "$CUR" ] && { echo "ERROR: cannot read feemarket params"; exit 1; }
echo "$CUR" | jq -c .

section "build MsgUpdateParams (min_gas_price -> $NEW_MIN)"
GOV="$(gov_addr)"
[ -z "$GOV" ] && { echo "ERROR: no gov module address"; exit 1; }
echo "  gov authority = $GOV"
TMP="$(mktemp -d)"
# Lower min_gas_price AND disable the dynamic EIP-1559 base fee. For cosmos txs
# the ante enforces the (dynamic) base_fee as the required gas price; with
# no_base_fee=true only min_gas_price applies, giving a stable low floor that
# browser-wallet default fees clear. base_fee is also reset low for good measure.
NEWP="$(echo "$CUR" | jq --arg m "$NEW_MIN" '.min_gas_price=$m | .base_fee=$m | .no_base_fee=true')"
jq -n --arg gov "$GOV" --argjson p "$NEWP" --arg t "$TYPE_URL" \
  '{"@type":$t, authority:$gov, params:$p}' > "$TMP/params.json"
cat "$TMP/params.json"

gov_pass "feemarket: lower min_gas_price to $NEW_MIN" "$TMP/params.json" \
  || { echo "GOV_FAILED"; rm -rf "$TMP"; exit 1; }
rm -rf "$TMP"

section "verify new params"
NOWMIN="$(q feemarket params | jq -r '.params.min_gas_price')"
echo "  min_gas_price now = $NOWMIN"
case "$NOWMIN" in
  "$NEW_MIN"|0.001*) echo "  OK changed" ;;
  *) echo "  WARN: min_gas_price did not change as expected" ;;
esac

section "TEST: low-fee cosmos tx (0.025uecy gas price) must now be accepted"
DEV0="$(addr dev0)"
OUT=$("$BIN" tx bank send dev0 "$DEV0" "1${DENOM}" \
  --from dev0 "${CObj[@]}" "${NObj[@]}" --chain-id "$CHAINID" \
  --gas 200000 --gas-prices "0.025${DENOM}" --broadcast-mode sync -y -o json 2>&1 | _ej)
HASH=$(echo "$OUT" | jq -r '.txhash // empty' 2>/dev/null)
CCODE=$(echo "$OUT" | jq -r '.code // empty' 2>/dev/null)
if [ -z "$HASH" ] || { [ -n "$CCODE" ] && [ "$CCODE" != "0" ]; }; then
  echo "LOWFEE_REJECTED: $(echo "$OUT" | jq -r '.raw_log // .' 2>/dev/null | head -c 220)"
  exit 1
fi
sleep 3
RES=$("$BIN" q tx "$HASH" "${NObj[@]}" -o json 2>/dev/null)
RC=$(echo "$RES" | jq -r '.code // empty' 2>/dev/null)
echo "  committed code=$RC hash=$HASH"
[ "$RC" = "0" ] && echo "LOWFEE_OK" || { echo "LOWFEE_FAIL($RC)"; exit 1; }
