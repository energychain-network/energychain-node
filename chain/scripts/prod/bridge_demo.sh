#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — mint/burn bridge demo provisioning.
#
# Makes the DEX /bridge page actually mint: registers a dedicated, fully
# reserve-backed stablecoin (usdc) as a bridgeable asset, binds it to a PoR
# reserve topic fed a large attested value (so reserve coverage always holds),
# optionally tightens mint rate limits, then performs ONE inbound deposit
# (attest + release) to prove 1:1 inbound minting end to end.
#
# Why a dedicated `usdc` (not the existing `usd`): the live `usd` supply is
# millions while its `reserve.usd` topic is fed only ~100 (and reset every
# load cycle), so reserve-gated minting against `usd` would always fail
# coverage. `usdc`/`reserve.usdc` is untouched by the load generator.
#
# Authority model (mirrors scripts/test/setup.sh):
#   * authority-gated msgs (create-topic, create-denom, register-asset,
#     update-params) -> ONE batched gov proposal (gov_pass, ~8s voting).
#   * admin/operator msgs (add-minter, bind-reserve, submit-value, attest,
#     release) -> signed directly by dev0 (denom admin + chain attestor).
#
# Idempotent: re-running after the usdc denom exists skips the gov create and
# just re-feeds the oracle + performs a fresh inbound (unique nonce).
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

TOPIC="${TOPIC:-reserve.usdc}"
UDENOM="${UDENOM:-usdc}"
# 1e8 usdc (6dp) attested reserve — a ceiling far above any demo issuance.
RESERVE_VALUE="${RESERVE_VALUE:-100000000000000}"
# 1000 usdc (6dp) inbound demo deposit.
MINT_AMOUNT="${MINT_AMOUNT:-1000000000}"

DEV0="$(addr dev0)"; ALICE="$(addr alice)"
RECIP="${RECIP:-$ALICE}"

section "node liveness"
H="$(height)"; [ -z "$H" ] && H=0
if [ "$H" = "0" ]; then echo "ERROR: node not reachable at $NODE"; exit 1; fi
echo "  height=$H gov=$(gov_addr) dev0=$DEV0 recipient=$RECIP"

# ---- 1. authority-gated create (topic + denom + bridge asset) --------------
if q stableusd denom "$UDENOM" >/dev/null 2>&1; then
  echo "  usdc denom already present — skipping gov create"
else
  section "gov proposal: reserve topic + usdc denom + bridge asset"
  TMP="$(mktemp -d)"
  gen_msg "$TMP/01_topic.json" assethub create-topic  --id "$TOPIC" --description "USDC bridge reserve PoR" --min-sources 1
  gen_msg "$TMP/02_denom.json" stableusd create-denom  --id "$UDENOM" --symbol scnUSDC --decimals 6 --peg-currency USD --admin "$DEV0"
  gen_msg "$TMP/03_asset.json" bridge register-asset   "$UDENOM"
  for f in "$TMP"/*.json; do
    jq -e '."@type"' "$f" >/dev/null 2>&1 || { echo "  ✗ invalid msg $(basename "$f")"; cat "$f"; rm -rf "$TMP"; exit 1; }
  done
  gov_pass "bridge demo: usdc topic+denom+asset" \
    "$TMP/01_topic.json" "$TMP/02_denom.json" "$TMP/03_asset.json" \
    || { echo "  ✗ gov create failed"; rm -rf "$TMP"; exit 1; }
  rm -rf "$TMP"
fi

# ---- 2. admin: minter + reserve bind + oracle feed -------------------------
section "dev0 admin: minter + reserve bind + oracle feed"
expect_ok "add-minter usdc (dev0)" dev0 stableusd add-minter --denom-id "$UDENOM" --minter "$DEV0" || true
expect_ok "bind-reserve usdc -> $TOPIC (100%)" dev0 stableusd bind-reserve \
  --denom-id "$UDENOM" --reserve-topic "$TOPIC" --required-ratio-bps 10000 --max-staleness-seconds 0
expect_ok "submit-value $TOPIC = $RESERVE_VALUE" dev0 assethub submit-value "$TOPIC" "$RESERVE_VALUE"
wait_blocks 1 >/dev/null

# ---- 3. mint rate limits (full-params gov update; preserves other fields) ---
section "bridge mint rate limits (gov update-params)"
CUR="$(q bridge params | jq '.params // empty')"
if [ -n "$CUR" ]; then
  TMP2="$(mktemp -d)"
  jq -n --arg gov "$(gov_addr)" \
        --argjson p "$(echo "$CUR" | jq '
          .max_mint_per_tx = "100000000000"        # 100k usdc per release
          | .mint_window_seconds = 3600            # rolling 1h window
          | .max_mint_per_window = "1000000000000" # 1M usdc / chain / hour
        ')" \
    '{"@type":"/energychain.bridge.v1.MsgUpdateParams", authority:$gov, params:$p}' \
    > "$TMP2/params.json"
  if jq -e '.params' "$TMP2/params.json" >/dev/null 2>&1; then
    gov_pass "bridge mint limits" "$TMP2/params.json" \
      || echo "  WARN: limit proposal did not pass (reserve gate still enforced)"
  else
    echo "  SKIP: could not build params msg (reserve gate still enforced)"
  fi
  rm -rf "$TMP2"
else
  echo "  SKIP: could not read current bridge params"
fi

# ---- 4. demo inbound deposit (attest + release -> 1:1 mint) ----------------
section "demo inbound deposit (attest + release)"
ASSET_ID="$(q bridge assets | jq -r '[.assets[]|select(.denom=="'"$UDENOM"'")|.id]|min // empty')"
CHAIN_ID="$(q bridge chains | jq -r '[.chains[]|select(.name=="ethereum")|.id]|min // 1')"
[ -z "$ASSET_ID" ] && { echo "  ✗ usdc asset not found"; exit 1; }
NONCE="${NONCE:-$(date +%s)}"
echo "  usdc asset_id=$ASSET_ID chain_id=$CHAIN_ID nonce=$NONCE amount=$MINT_AMOUNT recipient=$RECIP"
expect_ok "attest inbound (dev0, quorum=1)" dev0 bridge attest "$CHAIN_ID" "$NONCE" "$RECIP" "$ASSET_ID" "$MINT_AMOUNT"
IB="$(q bridge inbounds | jq -r '[.inbounds[]|select(.src_nonce=="'"$NONCE"'")|.id]|max // empty')"
[ -z "$IB" ] && IB="$(q bridge inbounds | jq -r '.inbounds[-1].id // empty')"
echo "  inbound id=$IB"
expect_ok "release inbound -> mint usdc 1:1" dev0 bridge release "$IB"
wait_blocks 1 >/dev/null

# ---- 5. verify -------------------------------------------------------------
section "verify"
echo "  usdc supply        : $(q stableusd supply  "$UDENOM"        | jq -rc '.supply // .amount // .')"
echo "  recipient balance  : $(q stableusd balance "$UDENOM" "$RECIP" | jq -rc '.balance // .amount // .')"
echo "  net-bridged usdc   : $(q bridge net-bridged "$UDENOM"       | jq -rc '.balance // .')"
echo "  bridge params      : $(q bridge params | jq -rc '.params|{max_mint_per_tx,mint_window_seconds,max_mint_per_window}')"
summary || true
echo "BRIDGE_DEMO_DONE"
