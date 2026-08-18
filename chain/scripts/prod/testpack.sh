#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — UI walkthrough test pack.
#
# Creates N demo wallets (default 10) and provisions each so a tester can
# exercise EVERY DEX page end to end:
#   Trade (market usd/eur) · Assets (usd/eur/usdc + RWA SOLAR1) · Mincast ·
#   Offerings (IRO) · Bridge (usdc deposit/withdraw) · Energy (read) · Issue.
#
# Each wallet gets: gas (ECY), KYC+accredited identity, usd/eur/usdc balances,
# RWA SOLAR1 units. Wallets w1/w2 additionally get an ATTESTED (un-released)
# usdc inbound so the Bridge "领取/Release" (1:1 mint) button is testable.
#
# Mnemonics are eth_secp256k1 / coin type 60 — import directly into Keplr; the
# DEX auto-suggests the chain on connect and derives the same energy1 address.
#
# Idempotent: if the manifest exists it just reprints it (RESEED=1 to rebuild,
# which DELETES + recreates w1..wN and re-provisions).
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

# shellcheck source=../test/lib.sh
source "${CHAIN_DIR}/scripts/test/lib.sh"

N="${N:-10}"
PREFIX="${PREFIX:-w}"
RESEED="${RESEED:-0}"
MANIFEST="${HOME_T}/testpack.json"

USD="usd"; EUR="eur"; USDC="usdc"
RWA_ID="${RWA_ID:-1}"

GAS_EACH="2000000000000000000000"  # 2000 ECY (18dp)
USD_EACH="1000000000000"           # 1,000,000 usd  (6dp)
EUR_EACH="500000000000"            #   500,000 eur  (6dp)
USDC_EACH="500000000000"           #   500,000 usdc (6dp)
RWA_UNITS="1000000000"             #     1,000 SOLAR1 (6dp)
INBOUND_AMT="2000000000"           #     2,000 usdc attested inbound (w1/w2)

print_manifest() {
  echo "================ TESTPACK_WALLETS ================"
  jq -r '.[] | "\(.name)\t\(.address)\t\(.mnemonic)"' "$MANIFEST"
  echo "================ END_TESTPACK ===================="
}

if [ -f "$MANIFEST" ] && [ "$RESEED" != "1" ]; then
  echo "[testpack] manifest present (RESEED=1 to rebuild). Reprinting."
  print_manifest
  exit 0
fi

section "node liveness"
H="$(height)"; [ -z "$H" ] && H=0
[ "$H" = "0" ] && { echo "ERROR: node not reachable at $NODE"; exit 1; }
DEV0="$(addr dev0)"
echo "  height=$H dev0=$DEV0 rwa_id=$RWA_ID"

section "create $N wallets (${PREFIX}1..${PREFIX}${N})"
NAMES=(); ADDRS=(); MNES=()
for i in $(seq 1 "$N"); do
  k="${PREFIX}${i}"
  "$BIN" keys delete "$k" -y "${CObj[@]}" >/dev/null 2>&1
  out="$("$BIN" keys add "$k" --algo eth_secp256k1 --coin-type 60 "${CObj[@]}" --output json 2>/dev/null)"
  a="$(echo "$out" | jq -r '.address')"
  mn="$(echo "$out" | jq -r '.mnemonic')"
  [ -z "$a" ] || [ "$a" = "null" ] && { echo "  ✗ failed to create $k"; exit 1; }
  NAMES+=("$k"); ADDRS+=("$a"); MNES+=("$mn")
  echo "  $k = $a"
done

section "fund gas (bank multi-send, 2000 ECY each)"
"$BIN" tx bank multi-send dev0 "${ADDRS[@]}" "${GAS_EACH}${DENOM}" \
  "${CObj[@]}" "${NObj[@]}" --chain-id "$CHAINID" \
  --gas auto --gas-adjustment 1.4 --gas-prices "$GAS_PRICES" \
  --broadcast-mode sync -y -o json >/dev/null 2>&1
wait_blocks 2 >/dev/null

mint_to() { expect_ok "mint $1 -> $2" dev0 stableusd mint "$1" "$2" "$3" >/dev/null; }

section "per-wallet: identity (KYC) + usd/eur/usdc + RWA units"
for idx in "${!ADDRS[@]}"; do
  a="${ADDRS[$idx]}"; k="${NAMES[$idx]}"
  expect_ok "$k identity (kyc+accredited)" dev0 identity set-account \
    --address "$a" --did "did:ec:$k" --kyc-cleared=true --accredited=true \
    --jurisdiction US --kyc-expires-at 0 >/dev/null
  mint_to "$USD"  "$a" "$USD_EACH"
  mint_to "$EUR"  "$a" "$EUR_EACH"
  mint_to "$USDC" "$a" "$USDC_EACH"
  expect_ok "$k rwa SOLAR1 units" dev0 rwatoken mint "$RWA_ID" "$a" "$RWA_UNITS" >/dev/null
  wait_blocks 1 >/dev/null
done

section "bridge: ATTESTED usdc inbound for w1/w2 (testable Release in UI)"
ASSET_ID="$(q bridge assets | jq -r '[.assets[]|select(.denom=="'"$USDC"'")|.id]|min // empty')"
CHAIN_ID="$(q bridge chains | jq -r '[.chains[]|select(.name=="ethereum")|.id]|min // 1')"
if [ -n "$ASSET_ID" ]; then
  for idx in 0 1; do
    a="${ADDRS[$idx]}"; k="${NAMES[$idx]}"
    [ -z "$a" ] && continue
    NONCE="$(( $(date +%s) + idx ))"
    expect_ok "$k attest inbound (usdc, dev0 quorum=1)" dev0 bridge attest \
      "$CHAIN_ID" "$NONCE" "$a" "$ASSET_ID" "$INBOUND_AMT" >/dev/null
  done
  echo "  (left ATTESTED, NOT released — tester clicks 领取/Release on /bridge)"
else
  echo "  WARN: usdc bridge asset not found; run bridge_demo.sh first"
fi

section "write manifest"
{
  echo -n "["
  for idx in "${!NAMES[@]}"; do
    [ "$idx" -gt 0 ] && echo -n ","
    jq -nc --arg n "${NAMES[$idx]}" --arg a "${ADDRS[$idx]}" --arg m "${MNES[$idx]}" \
      '{name:$n,address:$a,mnemonic:$m}'
  done
  echo "]"
} > "$MANIFEST"
echo "  manifest -> $MANIFEST"

section "summary"
summary || true
print_manifest
echo "TESTPACK_DONE"
