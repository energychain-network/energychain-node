#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — top up the testpack wallets (w1..wN) with bridged usdc.
#
# For every wallet in ~/.energychaind/testpack.json this performs a full
# inbound bridge deposit (attest + release), minting AMT usdc 1:1 to the
# wallet. This both funds the wallets for a fresh end-to-end walkthrough AND
# leaves a RELEASED inbound visible on the /bridge page ("我的入金").
#
# Prereqs (already true on the prod host):
#   * usdc denom + bridge asset exist (scripts/prod/bridge_demo.sh)
#   * dev0 is a bridge attestor and the usdc reserve topic is funded
#   * scripts/prod/testpack.sh has written the wallet manifest
#
# Idempotent-ish: each release uses a unique nonce, so re-running simply mints
# another AMT to every wallet (balances accumulate).
#
#   AMT=2000000000 ./bridge_seed_wallets.sh         # 2000 usdc (6dp) each
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

UDENOM="${UDENOM:-usdc}"
AMT="${AMT:-2000000000}"                 # 2000 usdc (6dp) per wallet
MANIFEST="${MANIFEST:-$HOME_T/testpack.json}"

section "node liveness"
H="$(height)"; [ -z "$H" ] && H=0
[ "$H" = "0" ] && { echo "ERROR: node not reachable at $NODE"; exit 1; }
[ -f "$MANIFEST" ] || { echo "ERROR: wallet manifest $MANIFEST missing — run scripts/prod/testpack.sh first"; exit 1; }
echo "  height=$H denom=$UDENOM amount=$AMT manifest=$MANIFEST"

ASSET_ID="$(q bridge assets | jq -r '[.assets[]|select(.denom=="'"$UDENOM"'")|.id]|min // empty')"
CHAIN_ID="$(q bridge chains | jq -r '[.chains[]|select(.name=="ethereum")|.id]|min // 1')"
[ -z "$ASSET_ID" ] && { echo "ERROR: $UDENOM bridge asset not found — run scripts/prod/bridge_demo.sh first"; exit 1; }
echo "  usdc asset_id=$ASSET_ID eth_chain_id=$CHAIN_ID"

# Iterate wallets from the manifest: name<TAB>address.
mapfile -t ROWS < <(jq -r '.[] | "\(.name)\t\(.address)"' "$MANIFEST")
[ "${#ROWS[@]}" -gt 0 ] || { echo "ERROR: no wallets in manifest"; exit 1; }

n=0
for row in "${ROWS[@]}"; do
  name="${row%%$'\t'*}"; a="${row##*$'\t'}"
  [ -z "$a" ] && continue
  section "$name <- $AMT $UDENOM ($a)"
  NONCE="$(( $(date +%s%N) / 1000 + n ))"     # microsecond-unique nonce
  expect_ok "$name attest inbound" dev0 bridge attest "$CHAIN_ID" "$NONCE" "$a" "$ASSET_ID" "$AMT" || { n=$((n+1)); continue; }
  IB="$(q bridge inbounds | jq -r '[.inbounds[]|select(.src_nonce=="'"$NONCE"'")|.id]|max // empty')"
  [ -z "$IB" ] && IB="$(q bridge inbounds | jq -r '.inbounds[-1].id // empty')"
  expect_ok "$name release -> mint $UDENOM 1:1 (inbound $IB)" dev0 bridge release "$IB" || true
  wait_blocks 1 >/dev/null
  bal="$(q stableusd balance "$UDENOM" "$a" | jq -rc '.balance // .amount // .')"
  echo "  $name $UDENOM balance now: $bal"
  n=$((n+1))
done

section "summary"
summary || true
echo "BRIDGE_SEED_WALLETS_DONE ($n wallets, +$AMT $UDENOM each)"
