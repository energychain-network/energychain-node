#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — continuous on-chain LOAD GENERATOR (~POOL_SIZE tx / block).
#
# Each cycle fires exactly one tx per pool account, in parallel, then waits one
# block so every account's sequence commits before the next cycle. Account i is
# pinned to module (i % 9), so all nine modules receive traffic every block:
#
#   stableusd transfer · rwatoken transfer · mincast transfer · market place ·
#   assethub submit-value · identity set-account · offering subscribe ·
#   automation create-schedule · bridge lock
#
# Run under systemd (Restart=always). Stop with: systemctl stop ec-loadgen
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
export GAS_LIMIT="${GAS_LIMIT:-900000}"

# shellcheck source=../test/lib.sh
source "${CHAIN_DIR}/scripts/test/lib.sh"

MANIFEST="${HOME_T}/loadpool.env"
[ -f "$MANIFEST" ] || { echo "ERROR: $MANIFEST missing — run scripts/prod/seed.sh first"; exit 1; }
# shellcheck disable=SC1090
source "$MANIFEST"
N="${POOL_SIZE:?}"
MARKET_PAIR="${MARKET_PAIR:-1}"
OFFER_ID="${OFFER_ID:-1}"
RWA_ID="${RWA_ID:-1}"
MCX_ID="${MCX_ID:-1}"
BRIDGE_ASSET_ID="${BRIDGE_ASSET_ID:-1}"
BRIDGE_CHAIN_ID="${BRIDGE_CHAIN_ID:-1}"
BURN_ADDR="0x000000000000000000000000000000000000dEaD"

section "loadgen start"
H="$(height)"; [ -z "$H" ] && H=0
[ "$H" = "0" ] && { echo "ERROR: node not reachable at $NODE"; exit 1; }
echo "  pool=$N node@$H pair=$MARKET_PAIR offer=$OFFER_ID rwa=$RWA_ID mcx=$MCX_ID"

# Pre-resolve every pool address once (keyring lookups are slow to repeat).
declare -a A
for i in $(seq 0 $((N-1))); do A[$i]="$(addr "load${i}")"; done
echo "  resolved ${#A[@]} pool addresses"

# fire <i> <cycle> — submit one module tx for account i (fire-and-forget sync).
fire() {
  local i="$1" c="$2" m=$(( i % 9 )) k="load${i}" peer pos phase drift noise price
  peer=$(( i + 9 )); [ "$peer" -ge "$N" ] && peer=$(( i % 9 ))
  local pa="${A[$peer]}"
  local -a t
  case "$m" in
    0) t=(stableusd transfer "$USD" "$pa" 1000) ;;
    1) t=(rwatoken transfer "$RWA_ID" "$pa" 100) ;;
    2) t=(mincast transfer "$MCX_ID" "$pa" 1) ;;
    3) pos=$(( i / 9 ))
       # Drive a lively, bounded price on the main pair so its candlestick chart
       # actually moves. An up-biased zig-zag over a 400-cycle period (rise for
       # 300, ease back for 100) keeps the price between ~0.85 and ~1.42 forever
       # — no runaway, no balance drain. Per-cycle ±1.5% noise gives each 1m
       # candle a realistic O/H/L/C body instead of a flat doji.
       phase=$(( c % 400 ))
       if [ "$phase" -lt 300 ]; then drift=$(( phase * 1400 )); else drift=$(( (400 - phase) * 4200 )); fi
       noise=$(( (RANDOM % 30001) - 15000 ))
       price=$(( 1000000 + drift + noise ))
       [ "$price" -lt 850000 ] && price=850000
       # Buy-heavy by ORDER COUNT (6 buys : 2 sells across the 8 market
       # accounts) yet quantity-balanced (6×1000 == 2×3000) so every batch fully
       # clears at the reference price: more demand on the book, clean fills.
       if [ $(( pos % 4 )) -eq 3 ]; then t=(market place "$MARKET_PAIR" sell "$price" 3000)
       else t=(market place "$MARKET_PAIR" buy "$price" 1000); fi ;;
    4) t=(assethub submit-value reserve.usd $(( 100000000 + (c % 1000000) ))) ;;
    5) t=(identity set-account --address "$pa" --did "did:ec:load${i}" --kyc-cleared=true --accredited=true --jurisdiction US --kyc-expires-at 0) ;;
    6) t=(offering subscribe "$OFFER_ID" 1000000) ;;
    7) t=(automation create-schedule rwa-snapshot "$RWA_ID" 0 60 0 0 0) ;;
    8) t=(bridge lock "$BRIDGE_ASSET_ID" 1000 "$BRIDGE_CHAIN_ID" "$BURN_ADDR") ;;
  esac
  "$BIN" tx "${t[@]}" --from "$k" "${CObj[@]}" "${NObj[@]}" \
    --chain-id "$CHAINID" --gas "$GAS_LIMIT" --gas-prices "$GAS_PRICES" \
    --broadcast-mode sync -y >/dev/null 2>&1
}

trap 'echo "[loadgen] stopping"; exit 0' TERM INT

# Firing the whole pool at once emptied it into a single block (~70 tx) and left
# the next two or three empty, because a cycle costs as long as the slowest CLI
# invocation. Staggering the launches spreads the same volume evenly instead:
# with STAGGER_MS=40 a 72-account pool submits over ~2.9s, so each ~2s block
# receives roughly 50 tx.
#
# The stagger doubles as the sequence guard the old CYCLE_SLEEP provided: an
# account's next tx is one full cycle away, which is longer than the block it
# needs to commit in, so its sequence is never read stale.
STAGGER_MS="${STAGGER_MS:-40}"
CYCLE_SLEEP="${CYCLE_SLEEP:-0}"
c=0
while true; do
  for i in $(seq 0 $((N-1))); do
    ( sleep "$(awk -v i="$i" -v s="$STAGGER_MS" 'BEGIN{printf "%.3f", i*s/1000}')"
      fire "$i" "$c" ) &
  done
  wait
  [ "$CYCLE_SLEEP" != "0" ] && sleep "$CYCLE_SLEEP"
  c=$(( c + 1 ))
  if [ $(( c % 10 )) -eq 0 ]; then
    ntx=$(curl -s "http://${NODE#tcp://}/num_unconfirmed_txs" 2>/dev/null | jq -r '.result.total // "?"')
    echo "[loadgen] cycle=$c height=$(height) mempool=${ntx} (~${N} tx/cycle across 9 modules, ${STAGGER_MS}ms stagger)"
  fi
done
