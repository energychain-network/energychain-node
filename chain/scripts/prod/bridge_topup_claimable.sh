#!/usr/bin/env bash
# Top up attested inbounds for wallets that have fewer than PER claimable rows.
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

PER="${PER:-3}"
AMT="${AMT:-2000000000}"
MANIFEST="${MANIFEST:-$HOME_T/testpack.json}"
ASSET_ID="$(q bridge assets | jq -r '[.assets[]|select(.denom=="usdc")|.id]|min // empty')"
CHAIN_ID="$(q bridge chains | jq -r '[.chains[]|select(.name=="ethereum")|.id]|min // 1')"

mapfile -t ROWS < <(jq -c '.[]' "$MANIFEST")
for row in "${ROWS[@]}"; do
  name="$(jq -r '.name' <<<"$row")"
  a="$(jq -r '.address' <<<"$row")"
  have="$(q bridge inbounds | jq -r '[.inbounds[]|select(.recipient=="'"$a"'" and (.status|test("ATTESTED")))]|length')"
  need=$((PER - have))
  [ "$need" -le 0 ] && { echo "$name: $have attested (ok)"; continue; }
  echo "$name: have=$have need=$need"
  for j in $(seq 1 "$need"); do
    NONCE="$(( $(date +%s%N) / 1000 + RANDOM + j ))"
    expect_ok "$name top-up[$j]" dev0 bridge attest "$CHAIN_ID" "$NONCE" "$a" "$ASSET_ID" "$AMT"
    wait_blocks 1 >/dev/null
  done
done
