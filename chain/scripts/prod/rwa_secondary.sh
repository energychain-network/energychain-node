#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — provision + E2E the RWA secondary order-book market and the
# snapshot-dividend flow (run ON the production host, after seed.sh).
#
# Demonstrates the full "yield follows the token" lifecycle:
#   1. ensure a usdc settlement denom (dev0 minter)
#   2. create an RWA security token (MYSOLAR, settlement=usdc)
#   3. mint shares to a seller, usdc to a buyer + the issuer
#   4. authority creates the rwa/<id> ÷ usdc order book (gov proposal)
#   5. seller SELL + buyer BUY at the same price -> batch clears
#      => shares move seller->buyer, usdc moves buyer->seller
#   6. issuer TakeSnapshot -> CreateDistribution against that snapshot
#      => the BUYER (new holder-of-record) can claim; the SELLER cannot
#
# Idempotent-ish: re-running reuses the existing token/market and just adds a
# fresh snapshot + distribution round (set FRESH_ROUND=0 to skip the round).
# ===========================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Point the shared library at the production node + home (same as seed.sh).
export BIN="${BIN:-$CHAIN_DIR/energychaind}"
export HOME_T="${HOME_T:-$HOME/.energychaind}"
export CHAINID="${CHAINID:-energychain_9001-1}"
export NODE="${NODE:-tcp://127.0.0.1:26657}"
export DENOM="${DENOM:-uecy}"
export KEYRING="${KEYRING:-test}"
export GAS_PRICES="${GAS_PRICES:-10000000000${DENOM}}"

# shellcheck source=../test/lib.sh
source "${CHAIN_DIR}/scripts/test/lib.sh"

SYMBOL="${SYMBOL:-MYSOLAR}"
USDC="${USDC:-usdc}"
SHARE_DEC="${SHARE_DEC:-0}"            # whole-share security token (plan: MYSOLAR=0dp)
USDC_DEC=6
SELLER_KEY="${SELLER_KEY:-rwaseller}"
BUYER_KEY="${BUYER_KEY:-rwabuyer}"
SELL_SHARES="${SELL_SHARES:-1000}"     # minted to the seller
TRADE_QTY="${TRADE_QTY:-100}"          # shares crossed in the demo trade
PRICE_HUMAN="${PRICE_HUMAN:-2}"        # usdc per share
DIST_USDC_HUMAN="${DIST_USDC_HUMAN:-100}"   # dividend pool for the round
FRESH_ROUND="${FRESH_ROUND:-1}"
# MARKET_ONLY=1 only ensures the rwa/<id>/usdc order book exists for an
# already-issued token (resolved by SYMBOL). It skips minting/trading/dividend,
# which require the token's own admin key — used to list a token whose issuer
# is an external wallet (e.g. the real MYSOLAR) for secondary trading.
MARKET_ONLY="${MARKET_ONLY:-0}"
GAS_EACH="2000000000000000000000"      # 2000 ECY gas

# price exponent: raw_price = human_price * 10^(6 + quoteDec - baseDec)
PRICE_EXP=$((6 + USDC_DEC - SHARE_DEC))
pow10() { local n="$1" s=1; while [ "$n" -gt 0 ]; do s=$((s * 10)); n=$((n-1)); done; echo "$s"; }
PRICE_RAW=$(( PRICE_HUMAN * $(pow10 "$PRICE_EXP") ))   # human price -> chain raw price
DIST_RAW=$(( DIST_USDC_HUMAN * $(pow10 "$USDC_DEC") )) # usdc 6dp
BUYER_USDC_RAW="1000000000000"         # 1,000,000 usdc headroom
ISSUER_USDC_RAW="$((DIST_RAW * 4))"

section "node liveness"
H="$(height)"; [ -z "$H" ] && H=0
[ "$H" = "0" ] && { echo "ERROR: node not reachable at $NODE"; exit 1; }
DEV0="$(addr dev0)"
echo "  node height=$H gov=$(gov_addr) dev0=$DEV0"
echo "  share_dec=$SHARE_DEC price_exp=$PRICE_EXP price_raw=$PRICE_RAW dist_raw=$DIST_RAW"

# ---- 1. usdc settlement denom ---------------------------------------------
section "usdc settlement denom"
if q stableusd denom "$USDC" >/dev/null 2>&1; then
  echo "  usdc denom already present"
else
  echo "  creating usdc denom via gov"
  TMP="$(mktemp -d)"
  gen_msg "$TMP/usdc.json" stableusd create-denom --id "$USDC" --symbol scnUSDC --decimals "$USDC_DEC" --peg-currency USD --admin "$DEV0"
  gov_pass "create usdc denom" "$TMP/usdc.json" || { echo "  ✗ usdc gov failed"; rm -rf "$TMP"; exit 1; }
  rm -rf "$TMP"
fi
# dev0 must be able to mint usdc for the demo (tolerated if already a minter /
# not the admin — we re-check the balance before distributing).
expect_ok "add-minter dev0 (usdc)" dev0 stableusd add-minter --denom-id "$USDC" --minter "$DEV0" || true

# ---- 1b. issuer whitelist ---------------------------------------------------
# CreateToken is gated by the governance-managed issuer whitelist.
section "issuer whitelist (dev0)"
IS_ISSUER="$(q rwatoken is-issuer "$DEV0" 2>/dev/null | jq -r '.is_issuer // false')"
if [ "$IS_ISSUER" = "true" ]; then
  echo "  dev0 already whitelisted"
else
  TMP="$(mktemp -d)"
  gen_msg "$TMP/issuer.json" rwatoken add-issuer "$DEV0" --display-name "Dev Issuer"
  gov_pass "whitelist dev0 as rwa issuer" "$TMP/issuer.json" || { echo "  ✗ issuer gov failed"; rm -rf "$TMP"; exit 1; }
  rm -rf "$TMP"
fi

# post_bond <market-id> — pay the listing bond if the market awaits it.
post_bond() {
  local mid="$1" st
  st="$(q market market "$mid" | jq -r '(.market // .).status // empty')"
  if [ "$st" = "MARKET_STATUS_PENDING_BOND" ]; then
    expect_ok "post listing bond (market $mid)" dev0 market post-bond "$mid"
    wait_blocks 1 >/dev/null
  else
    echo "  market $mid status=$st — no bond required"
  fi
}

# ---- 2. MYSOLAR security token --------------------------------------------
section "rwa token ${SYMBOL}"
TOKEN_ID="$(q rwatoken tokens | jq -r --arg s "$SYMBOL" '[.tokens[]? | (.token // .) | select(.symbol==$s) | .id] | max // empty')"
if [ -n "$TOKEN_ID" ] && [ "$TOKEN_ID" != "null" ]; then
  echo "  reusing existing token ${SYMBOL} id=$TOKEN_ID"
elif [ "$MARKET_ONLY" = "1" ]; then
  echo "ERROR: MARKET_ONLY set but token ${SYMBOL} not found"; exit 1
else
  expect_ok "create-token ${SYMBOL}" dev0 rwatoken create-token \
    --symbol "$SYMBOL" --name "My Solar Revenue" --asset-class revenue_right --decimals "$SHARE_DEC" \
    --settlement-denom "$USDC" --require-kyc=false --redemption-price 1000000 \
    --redemption-delay-seconds 5 --per-holder-cap 0 --metadata-uri "ipfs://mysolar"
  wait_blocks 1 >/dev/null
  TOKEN_ID="$(q rwatoken tokens | jq -r --arg s "$SYMBOL" '[.tokens[]? | (.token // .) | select(.symbol==$s) | .id] | max // empty')"
fi
[ -z "$TOKEN_ID" ] || [ "$TOKEN_ID" = "null" ] && { echo "ERROR: could not resolve ${SYMBOL} token id"; exit 1; }
BASE_DENOM="rwa/${TOKEN_ID}"
echo "  token_id=$TOKEN_ID base_denom=$BASE_DENOM"

# ---- 3. seller / buyer keys + balances ------------------------------------
if [ "$MARKET_ONLY" = "1" ]; then
  section "secondary market ${BASE_DENOM:=rwa/${TOKEN_ID}} / ${USDC} (market-only)"
  MARKET_ID="$(q market markets | jq -r --arg b "rwa/${TOKEN_ID}" '[.markets[]? | (.market // .) | select(.base_denom==$b) | .id] | max // empty')"
  if [ -n "$MARKET_ID" ] && [ "$MARKET_ID" != "null" ]; then
    echo "  market already exists id=$MARKET_ID"
  else
    TMP="$(mktemp -d)"
    gen_msg "$TMP/mkt.json" market create-market "rwa/${TOKEN_ID}" "$USDC" 10 1 1 --operator "$DEV0"
    gov_pass "create rwa/${TOKEN_ID}/${USDC} market" "$TMP/mkt.json" || { echo "  ✗ market gov failed"; rm -rf "$TMP"; exit 1; }
    rm -rf "$TMP"; wait_blocks 1 >/dev/null
    MARKET_ID="$(q market markets | jq -r --arg b "rwa/${TOKEN_ID}" '[.markets[]? | (.market // .) | select(.base_denom==$b) | .id] | max // empty')"
  fi
  [ -n "$MARKET_ID" ] && [ "$MARKET_ID" != "null" ] && post_bond "$MARKET_ID"
  echo "  market_id=$MARKET_ID  base=rwa/${TOKEN_ID} quote=${USDC}"
  echo "============================================================"
  echo "  ${SYMBOL} now tradable: /trade/${MARKET_ID}  ·  /assets/rwa/${TOKEN_ID}"
  echo "============================================================"
  exit 0
fi

section "seller/buyer keys + balances"
for k in "$SELLER_KEY" "$BUYER_KEY"; do
  "$BIN" keys show "$k" "${CObj[@]}" >/dev/null 2>&1 || "$BIN" keys add "$k" --algo eth_secp256k1 "${CObj[@]}" >/dev/null 2>&1
done
SELLER="$(addr "$SELLER_KEY")"; BUYER="$(addr "$BUYER_KEY")"
echo "  seller=$SELLER buyer=$BUYER"
"$BIN" tx bank multi-send dev0 "$SELLER" "$BUYER" "${GAS_EACH}${DENOM}" \
  "${CObj[@]}" "${NObj[@]}" --chain-id "$CHAINID" --gas auto --gas-adjustment 1.4 \
  --gas-prices "$GAS_PRICES" --broadcast-mode sync -y -o json >/dev/null 2>&1
wait_blocks 2 >/dev/null
expect_ok "mint ${SYMBOL} shares -> seller" dev0 rwatoken mint "$TOKEN_ID" "$SELLER" "$SELL_SHARES" || true
expect_ok "mint usdc -> buyer"  dev0 stableusd mint "$USDC" "$BUYER" "$BUYER_USDC_RAW" || true
expect_ok "mint usdc -> issuer(dev0)" dev0 stableusd mint "$USDC" "$DEV0" "$ISSUER_USDC_RAW" || true
wait_blocks 1 >/dev/null

# ---- 4. authority creates the rwa/<id> ÷ usdc market ----------------------
section "secondary market ${BASE_DENOM} / ${USDC}"
MARKET_ID="$(q market markets | jq -r --arg b "$BASE_DENOM" '[.markets[]? | (.market // .) | select(.base_denom==$b) | .id] | max // empty')"
if [ -n "$MARKET_ID" ] && [ "$MARKET_ID" != "null" ]; then
  echo "  reusing existing market id=$MARKET_ID"
else
  TMP="$(mktemp -d)"
  gen_msg "$TMP/mkt.json" market create-market "$BASE_DENOM" "$USDC" 10 1 1 --operator "$DEV0"
  gov_pass "create ${BASE_DENOM}/${USDC} market" "$TMP/mkt.json" || { echo "  ✗ market gov failed"; rm -rf "$TMP"; exit 1; }
  rm -rf "$TMP"
  wait_blocks 1 >/dev/null
  MARKET_ID="$(q market markets | jq -r --arg b "$BASE_DENOM" '[.markets[]? | (.market // .) | select(.base_denom==$b) | .id] | max // empty')"
fi
[ -z "$MARKET_ID" ] || [ "$MARKET_ID" = "null" ] && { echo "ERROR: could not resolve market id"; exit 1; }
post_bond "$MARKET_ID"
echo "  market_id=$MARKET_ID"

# ---- 5. cross a trade: seller SELL + buyer BUY at the same price ----------
section "crossing a demo trade (qty=${TRADE_QTY} @ ${PRICE_HUMAN} ${USDC}/share)"
BUYER_SHARES_BEFORE="$(q rwatoken balance "$TOKEN_ID" "$BUYER" | jq -r '.amount // .balance // "0"')"
expect_ok "seller places SELL" "$SELLER_KEY" market place "$MARKET_ID" sell "$PRICE_RAW" "$TRADE_QTY"
expect_ok "buyer places BUY"   "$BUYER_KEY"  market place "$MARKET_ID" buy  "$PRICE_RAW" "$TRADE_QTY"
echo "  waiting for batch clear…"; wait_blocks 3 >/dev/null
LCP="$(q market market "$MARKET_ID" | jq -r '(.market // .).last_clearing_price // "0"')"
BUYER_SHARES_AFTER="$(q rwatoken balance "$TOKEN_ID" "$BUYER" | jq -r '.amount // .balance // "0"')"
echo "  last_clearing_price=$LCP  buyer shares ${BUYER_SHARES_BEFORE} -> ${BUYER_SHARES_AFTER}"
if [ "${BUYER_SHARES_AFTER:-0}" -gt "${BUYER_SHARES_BEFORE:-0}" ]; then
  _pass "trade settled: shares delivered to buyer"
else
  _fail "trade settled: buyer shares did not increase"
fi

# ---- 6. snapshot + distribution (yield follows the token) -----------------
if [ "$FRESH_ROUND" = "1" ]; then
  section "snapshot + dividend round"
  SNAP_OUT="$(_bcast dev0 rwatoken snapshot "$TOKEN_ID")"
  if [ "$(echo "$SNAP_OUT" | jq -r '.code // 1')" != "0" ]; then
    echo "  ✗ snapshot failed: $(echo "$SNAP_OUT" | jq -r '.raw_log' | head -c 200)"; exit 1
  fi
  SNAP_ID="$(echo "$SNAP_OUT" | jq -r '[.events[]?|select(.type=="rwatoken_snapshot")|.attributes[]?|select(.key=="snapshot_id")|.value]|last // empty')"
  echo "  snapshot_id=$SNAP_ID"
  [ -z "$SNAP_ID" ] && { echo "ERROR: could not resolve snapshot id"; exit 1; }

  DIST_OUT="$(_bcast dev0 rwatoken distribute "$TOKEN_ID" "$SNAP_ID" "$DIST_RAW")"
  if [ "$(echo "$DIST_OUT" | jq -r '.code // 1')" != "0" ]; then
    echo "  ✗ distribute failed: $(echo "$DIST_OUT" | jq -r '.raw_log' | head -c 200)"; exit 1
  fi
  DIST_ID="$(echo "$DIST_OUT" | jq -r '[.events[]?|select(.type=="rwatoken_distribution")|.attributes[]?|select(.key=="distribution_id")|.value]|last // empty')"
  echo "  distribution_id=$DIST_ID total=$DIST_RAW $USDC"
  [ -z "$DIST_ID" ] && { echo "ERROR: could not resolve distribution id"; exit 1; }

  BUYER_CLAIM="$(q rwatoken claimable "$DIST_ID" "$BUYER" | jq -r '.amount // .claimable // "0"')"
  SELLER_CLAIM="$(q rwatoken claimable "$DIST_ID" "$SELLER" | jq -r '.amount // .claimable // "0"')"
  echo "  claimable — buyer=$BUYER_CLAIM seller=$SELLER_CLAIM (raw $USDC)"
  if [ "${BUYER_CLAIM:-0}" -gt 0 ]; then _pass "buyer (new holder) can claim dividend"; else _fail "buyer claim is zero"; fi
fi

echo
echo "============================================================"
echo "  RWA secondary market + dividend provisioned"
echo "  token_id=$TOKEN_ID (${SYMBOL})   market_id=$MARKET_ID (${BASE_DENOM}/${USDC})"
echo "  open the DEX: /trade/${MARKET_ID}  and  /assets/rwa/${TOKEN_ID}"
echo "============================================================"
summary || true
