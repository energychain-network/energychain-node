#!/usr/bin/env bash
# ===========================================================================
# Re-derive the test-pack wallets from their SAVED mnemonics using the SAME
# derivation Keplr/Leap use for this Ethermint chain (eth_secp256k1, coin
# type 60), then fund + provision THOSE addresses. Fixes the earlier run that
# created keys without --coin-type 60 and therefore funded addresses no
# browser wallet derives.
#
# Reads mnemonics from $HOME_T/testpack.json (written by testpack.sh), rewrites
# the manifest with the corrected eth/60 addresses, and reprints the table.
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

MANIFEST="${HOME_T}/testpack.json"
RWA_ID="${RWA_ID:-1}"
USD="usd"; EUR="eur"; USDC="usdc"

GAS_EACH="2000000000000000000000"  # 2000 ECY
USD_EACH="1000000000000"           # 1,000,000 usd
EUR_EACH="500000000000"            #   500,000 eur
USDC_EACH="500000000000"           #   500,000 usdc
RWA_UNITS="1000000000"             #     1,000 SOLAR1
INBOUND_AMT="2000000000"           #     2,000 usdc

[ -f "$MANIFEST" ] || { echo "ERROR: $MANIFEST not found (run testpack.sh first)"; exit 1; }

section "re-derive wallets from saved mnemonics (eth_secp256k1 / coin type 60)"
mapfile -t NAMES < <(jq -r '.[].name' "$MANIFEST")
NEW_ADDRS=(); MNES=()
for k in "${NAMES[@]}"; do
  mn="$(jq -r --arg n "$k" '.[]|select(.name==$n)|.mnemonic' "$MANIFEST")"
  "$BIN" keys delete "$k" -y "${CObj[@]}" >/dev/null 2>&1
  a="$(printf '%s\n' "$mn" | "$BIN" keys add "$k" --recover --algo eth_secp256k1 --coin-type 60 \
        "${CObj[@]}" --output json 2>/dev/null | jq -r '.address')"
  [ -z "$a" ] || [ "$a" = "null" ] && { echo "  ✗ recover $k failed"; exit 1; }
  NEW_ADDRS+=("$a"); MNES+=("$mn")
  echo "  $k = $a"
done

section "fund gas (bank multi-send, 2000 ECY each)"
"$BIN" tx bank multi-send dev0 "${NEW_ADDRS[@]}" "${GAS_EACH}${DENOM}" \
  "${CObj[@]}" "${NObj[@]}" --chain-id "$CHAINID" \
  --gas auto --gas-adjustment 1.4 --gas-prices "$GAS_PRICES" \
  --broadcast-mode sync -y -o json >/dev/null 2>&1
wait_blocks 2 >/dev/null

mint_to() { expect_ok "mint $1 -> $2" dev0 stableusd mint "$1" "$2" "$3" >/dev/null; }

section "per-wallet: identity (KYC) + usd/eur/usdc + RWA units"
for idx in "${!NEW_ADDRS[@]}"; do
  a="${NEW_ADDRS[$idx]}"; k="${NAMES[$idx]}"
  expect_ok "$k identity" dev0 identity set-account \
    --address "$a" --did "did:ec:$k" --kyc-cleared=true --accredited=true \
    --jurisdiction US --kyc-expires-at 0 >/dev/null
  mint_to "$USD"  "$a" "$USD_EACH"
  mint_to "$EUR"  "$a" "$EUR_EACH"
  mint_to "$USDC" "$a" "$USDC_EACH"
  expect_ok "$k rwa SOLAR1" dev0 rwatoken mint "$RWA_ID" "$a" "$RWA_UNITS" >/dev/null
  wait_blocks 1 >/dev/null
done

section "bridge: ATTESTED usdc inbound for w1/w2 (correct eth/60 addresses)"
ASSET_ID="$(q bridge assets | jq -r '[.assets[]|select(.denom=="'"$USDC"'")|.id]|min // empty')"
CHAIN_ID="$(q bridge chains | jq -r '[.chains[]|select(.name=="ethereum")|.id]|min // 1')"
if [ -n "$ASSET_ID" ]; then
  for idx in 0 1; do
    a="${NEW_ADDRS[$idx]}"; k="${NAMES[$idx]}"
    NONCE="$(( $(date +%s) + 100 + idx ))"
    expect_ok "$k attest inbound" dev0 bridge attest \
      "$CHAIN_ID" "$NONCE" "$a" "$ASSET_ID" "$INBOUND_AMT" >/dev/null
  done
else
  echo "  WARN: usdc bridge asset not found"
fi

section "rewrite manifest with corrected addresses"
{
  echo -n "["
  for idx in "${!NAMES[@]}"; do
    [ "$idx" -gt 0 ] && echo -n ","
    jq -nc --arg n "${NAMES[$idx]}" --arg a "${NEW_ADDRS[$idx]}" --arg m "${MNES[$idx]}" \
      '{name:$n,address:$a,mnemonic:$m}'
  done
  echo "]"
} > "$MANIFEST"

section "summary"
summary || true
echo "================ TESTPACK_WALLETS ================"
jq -r '.[] | "\(.name)\t\(.address)"' "$MANIFEST"
echo "================ END_TESTPACK ===================="
echo "TESTPACK_FIX_DONE"
