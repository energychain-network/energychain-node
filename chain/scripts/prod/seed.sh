#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — production load SEED.
#
#   1. runs the standard fixture bootstrap (scripts/test/setup.sh): denoms
#      usd/eur, market usd/eur, bridge ethereum+usd, assethub topic, identity
#      registrar+sanction, rwatoken SOLAR1, mincast mcx1, a base offering.
#   2. provisions a pool of `load0..loadN-1` accounts. Account i is dedicated
#      to module (i % 9) and is given exactly the role/balance it needs:
#        0 stableusd   mint usd
#        1 rwatoken    mint units
#        2 mincast     mint usd + self-mint units
#        3 market      mint usd + eur
#        4 assethub    self register-provider (oracle)
#        5 identity    granted registrar (one batched gov proposal)
#        6 offering    mint usd (subscribes to the big load offering)
#        7 automation  gas only (creates schedules)
#        8 bridge      mint usd (locks out)
#   3. creates a long, huge-cap "load" offering so subscriptions never cap out.
#   4. writes the manifest ~/.energychaind/loadpool.env for loadgen.sh.
#
# Idempotent: re-running with the manifest present is a no-op unless RESEED=1.
# ===========================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Point the shared test library at the production node + home.
export BIN="${BIN:-$CHAIN_DIR/energychaind}"
export HOME_T="${HOME_T:-$HOME/.energychaind}"
export CHAINID="${CHAINID:-energychain_9001-1}"
export NODE="${NODE:-tcp://127.0.0.1:26657}"
export DENOM="${DENOM:-uecy}"
export KEYRING="${KEYRING:-test}"
export GAS_PRICES="${GAS_PRICES:-10000000000${DENOM}}"

# shellcheck source=../test/lib.sh
source "${CHAIN_DIR}/scripts/test/lib.sh"

POOL_SIZE="${POOL_SIZE:-72}"          # 8 accounts per module -> ~50 tx/block after block-wait overhead
RESEED="${RESEED:-0}"
MANIFEST="${HOME_T}/loadpool.env"
GAS_EACH="2000000000000000000000"     # 2000 ECY gas per pool account
USD="usd"; EUR="eur"
MINT_USD="100000000000"               # 100k scnUSD (6dp) per account that needs it
MINT_EUR="100000000000"
MINT_RWA="100000000"                  # rwatoken units
MELT_DUMMY=0

MODNAMES=(stableusd rwatoken mincast market assethub identity offering automation bridge)

if [ -f "$MANIFEST" ] && [ "$RESEED" != "1" ]; then
  echo "[seed] manifest $MANIFEST already present (RESEED=1 to force). Skipping."
  exit 0
fi

section "node liveness"
H="$(height)"; [ -z "$H" ] && H=0
if [ "$H" = "0" ]; then echo "ERROR: node not reachable at $NODE"; exit 1; fi
echo "  node at height $H, gov=$(gov_addr)"

# ---- 1. base fixtures ------------------------------------------------------
section "base fixtures (scripts/test/setup.sh)"
bash "${CHAIN_DIR}/scripts/test/setup.sh"
# shellcheck disable=SC1090
[ -f "${CHAIN_DIR}/scripts/test/.fixtures.env" ] && source "${CHAIN_DIR}/scripts/test/.fixtures.env"
DEV0="$(addr dev0)"
RWA_ID="${RWA_ID:-1}"; MCX_ID="${MCX_ID:-1}"
echo "  dev0=$DEV0 rwa_id=$RWA_ID mcx_id=$MCX_ID"

# ---- 2. create pool keys ---------------------------------------------------
section "creating ${POOL_SIZE} load keys"
POOL_ADDRS=()
for i in $(seq 0 $((POOL_SIZE-1))); do
  k="load${i}"
  if ! "$BIN" keys show "$k" "${CObj[@]}" >/dev/null 2>&1; then
    "$BIN" keys add "$k" --algo eth_secp256k1 "${CObj[@]}" >/dev/null 2>&1
  fi
  POOL_ADDRS+=("$(addr "$k")")
done
echo "  created/loaded ${#POOL_ADDRS[@]} keys"

# ---- 3. fund gas to every account in ONE multi-send ------------------------
section "funding gas (bank multi-send)"
"$BIN" tx bank multi-send dev0 "${POOL_ADDRS[@]}" "${GAS_EACH}${DENOM}" \
  "${CObj[@]}" "${NObj[@]}" --chain-id "$CHAINID" \
  --gas auto --gas-adjustment 1.4 --gas-prices "$GAS_PRICES" \
  --broadcast-mode sync -y -o json >/dev/null 2>&1
wait_blocks 2 >/dev/null
echo "  funded ${POOL_SIZE} accounts with ${GAS_EACH}${DENOM} gas each"

# ---- 4. per-module role / balance seeding ---------------------------------
mint_usd() { expect_ok "mint usd -> $1" dev0 stableusd mint "$USD" "$1" "$MINT_USD" >/dev/null; }
mint_eur() { expect_ok "mint eur -> $1" dev0 stableusd mint "$EUR" "$1" "$MINT_EUR" >/dev/null; }

section "per-module seeding"
IDENTITY_REGISTRARS=()
for i in $(seq 0 $((POOL_SIZE-1))); do
  k="load${i}"; a="${POOL_ADDRS[$i]}"; m=$((i % 9))
  case "$m" in
    0) mint_usd "$a" ;;                                   # stableusd
    1) expect_ok "rwa mint -> $k" dev0 rwatoken mint "$RWA_ID" "$a" "$MINT_RWA" >/dev/null ;;
    2) mint_usd "$a" ;;                                   # mincast (self-mints below)
    3) mint_usd "$a"; mint_eur "$a" ;;                    # market
    4) expect_ok "$k register-provider" "$k" assethub register-provider --role oracle --display-name "load-oracle-$i" --bond 10000000 >/dev/null ;;
    5) IDENTITY_REGISTRARS+=("$a") ;;                     # identity (gov below)
    6) mint_usd "$a" ;;                                   # offering
    7) : ;;                                               # automation (gas only)
    8) mint_usd "$a" ;;                                   # bridge
  esac
done
echo "  per-module balances/roles applied"

# identity: grant registrar to the identity group via ONE batched proposal
if [ "${#IDENTITY_REGISTRARS[@]}" -gt 0 ]; then
  section "granting registrar to identity group (gov)"
  TMP="$(mktemp -d)"; files=()
  idx=0
  for a in "${IDENTITY_REGISTRARS[@]}"; do
    f="$TMP/reg_${idx}.json"
    gen_msg "$f" identity add-registrar --registrar "$a" --display-name "load-registrar-$idx"
    files+=("$f"); idx=$((idx+1))
  done
  gov_pass "load registrars" "${files[@]}" || echo "  WARN: registrar proposal did not pass"
  rm -rf "$TMP"
fi

# mincast group: self-mint units so they can transfer them in the load loop
section "mincast group self-mint"
for i in $(seq 0 $((POOL_SIZE-1))); do
  [ $((i % 9)) -eq 2 ] || continue
  expect_ok "load${i} mincast mint" "load${i}" mincast mint "$MCX_ID" 20000000 0 >/dev/null
done

# ---- 5. long huge-cap load offering ---------------------------------------
section "load offering (huge cap, long window)"
NOW=$(date +%s); END=$((NOW + 315360000))   # +10 years
expect_ok "create load offering" dev0 offering create-offering \
  --token-id "$RWA_ID" --unit-price 1000000 \
  --soft-cap 1000000 --hard-cap 1000000000000000 \
  --start-time "$NOW" --end-time "$END" \
  --total-tranches 1 --required-injection 0 --injection-interval 0 >/dev/null
OFFER_ID=$(q offering offerings | jq -r '[.offerings[].id // .offerings[].offering.id] | max // 1')
OFFER_ID="${OFFER_ID:-1}"
echo "  load offering id=$OFFER_ID"

MARKET_PAIR=$(q market markets 2>/dev/null | jq -r '[.markets[].id // .markets[].market.id] | min // 1' 2>/dev/null)
[ -z "$MARKET_PAIR" ] || [ "$MARKET_PAIR" = "null" ] && MARKET_PAIR=1

# bridge numeric ids — `lock` takes asset_id (uint64) + dest_chain_id (uint64)
BRIDGE_ASSET_ID=$(q bridge assets 2>/dev/null | jq -r '[.assets[] | select(.denom=="usd") | .id] | min // 1' 2>/dev/null)
[ -z "$BRIDGE_ASSET_ID" ] || [ "$BRIDGE_ASSET_ID" = "null" ] && BRIDGE_ASSET_ID=1
BRIDGE_CHAIN_ID=$(q bridge chains 2>/dev/null | jq -r '[.chains[] | select(.name=="ethereum") | .id] | min // 1' 2>/dev/null)
[ -z "$BRIDGE_CHAIN_ID" ] || [ "$BRIDGE_CHAIN_ID" = "null" ] && BRIDGE_CHAIN_ID=1

# ---- 6. manifest -----------------------------------------------------------
cat > "$MANIFEST" <<EOF
# generated by scripts/prod/seed.sh
POOL_SIZE=$POOL_SIZE
RWA_ID=$RWA_ID
MCX_ID=$MCX_ID
OFFER_ID=$OFFER_ID
MARKET_PAIR=$MARKET_PAIR
USD=$USD
EUR=$EUR
DENOM=$DENOM
BRIDGE_ASSET_ID=$BRIDGE_ASSET_ID
BRIDGE_CHAIN_ID=$BRIDGE_CHAIN_ID
EOF
echo ""
echo "[seed] DONE. manifest -> $MANIFEST"
cat "$MANIFEST"
