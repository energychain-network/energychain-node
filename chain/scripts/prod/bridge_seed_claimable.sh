#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — seed ATTESTED (claimable) bridge inbounds for testpack wallets.
#
# For every wallet in ~/.energychaind/testpack.json this creates PER inbound
# bridge deposits that stop at ATTESTED (witness quorum reached) and are NOT
# released. Testers connect the wallet on /bridge → 入金 and click 「领取」 to
# mint usdc 1:1 on-chain.
#
# Prereqs:
#   * usdc denom + bridge asset (scripts/prod/bridge_demo.sh)
#   * dev0 is a bridge attestor; reserve topic funded
#   * testpack manifest (scripts/prod/testpack.sh)
#
#   PER=3 AMT=2000000000 ./bridge_seed_claimable.sh   # 3 × 2000 usdc each
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
AMT="${AMT:-2000000000}"                 # 2000 usdc (6dp) per inbound
PER="${PER:-3}"                           # claimable inbounds per wallet
MANIFEST="${MANIFEST:-$HOME_T/testpack.json}"

section "node liveness"
H="$(height)"; [ -z "$H" ] && H=0
[ "$H" = "0" ] && { echo "ERROR: node not reachable at $NODE"; exit 1; }
[ -f "$MANIFEST" ] || { echo "ERROR: wallet manifest $MANIFEST missing — run scripts/prod/testpack.sh first"; exit 1; }
echo "  height=$H denom=$UDENOM amount=$AMT per_wallet=$PER manifest=$MANIFEST"

ASSET_ID="$(q bridge assets | jq -r '[.assets[]|select(.denom=="'"$UDENOM"'")|.id]|min // empty')"
CHAIN_ID="$(q bridge chains | jq -r '[.chains[]|select(.name=="ethereum")|.id]|min // 1')"
[ -z "$ASSET_ID" ] && { echo "ERROR: $UDENOM bridge asset not found — run scripts/prod/bridge_demo.sh first"; exit 1; }
echo "  usdc asset_id=$ASSET_ID eth_chain_id=$CHAIN_ID"

mapfile -t ROWS < <(jq -c '.[]' "$MANIFEST")
[ "${#ROWS[@]}" -gt 0 ] || { echo "ERROR: no wallets in manifest"; exit 1; }

n=0
created=0
for row in "${ROWS[@]}"; do
  name="$(jq -r '.name' <<<"$row")"
  a="$(jq -r '.address' <<<"$row")"
  [ -z "$a" ] || [ "$a" = "null" ] && continue
  section "$name — $PER × $AMT $UDENOM (ATTESTED only)"
  for j in $(seq 1 "$PER"); do
    NONCE="$(( $(date +%s%N) / 1000 + n * 10 + j ))"
    if expect_ok "$name[$j] attest inbound" dev0 bridge attest "$CHAIN_ID" "$NONCE" "$a" "$ASSET_ID" "$AMT"; then
      IB="$(q bridge inbounds | jq -r '[.inbounds[]|select(.src_nonce=="'"$NONCE"'")|.id]|max // empty')"
      st="$(q bridge inbound "$IB" 2>/dev/null | jq -r '.inbound.status // .status // empty' 2>/dev/null || true)"
      echo "    inbound id=$IB status=$st"
      created=$((created + 1))
    fi
    wait_blocks 1 >/dev/null
  done
  n=$((n + 1))
done

section "summary"
echo "  wallets=${#ROWS[@]} per=$PER created=$created (left ATTESTED — claim on /bridge)"
summary || true
echo "BRIDGE_SEED_CLAIMABLE_DONE"
