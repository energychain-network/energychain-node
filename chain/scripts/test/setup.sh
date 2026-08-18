#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# One-time fixture setup for the module test suite.
#
#   1. create + fund test keys (alice, bob, mallory)
#   2. ONE batched gov proposal for authority-gated entities:
#        identity registrar(dev0) + sanction(mallory),
#        assethub reserve topic, stableusd denoms (usd, eur),
#        market usd/eur book, bridge chain + asset
#   3. dev0 self-admin seeding (minters, supply, rwatoken, mincast, offering,
#      provider, oracle value)
#
# Idempotency: safe to re-run; "already exists" style failures are tolerated.
# ---------------------------------------------------------------------------
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

KEYALGO="eth_secp256k1"

ensure_key() {
  local name="$1"
  if ! "$BIN" keys show "$name" "${CObj[@]}" >/dev/null 2>&1; then
    "$BIN" keys add "$name" --algo "$KEYALGO" "${CObj[@]}" >/dev/null 2>&1
  fi
}

# fund <addr> <uecy-amount> — sends from dev0 and WAITS for the tx to commit.
# (Three rapid sync-broadcasts in one block reuse the same account sequence and
# all but the first silently fail; _bcast polls for inclusion so the sequence
# advances before the next send.)
fund() {
  local out code
  out=$(_bcast dev0 bank send "$DEV0" "$1" "${2}${DENOM}")
  code=$(echo "$out" | jq -r '.code // empty' 2>/dev/null)
  if [ "$code" != "0" ]; then
    echo "  WARN: funding $1 failed (code=${code:-cli})"
  fi
}

section "node liveness"
if [ "$(height)" = "0" ] || [ -z "$(height)" ]; then
  echo "ERROR: node not reachable at $NODE — start it with scripts/local_node.sh"; exit 1
fi
echo "  node at height $(height), gov=$(gov_addr)"

# Idempotency: if the core fixtures already exist, don't re-run the gov
# proposal (it would fail on duplicate denoms/registrar/sanction).
if q stableusd denom usd >/dev/null 2>&1 && [ -f "${SCRIPT_DIR}/.fixtures.env" ]; then
  echo "  fixtures already present — skipping setup (delete chain data for a clean run)"
  exit 0
fi

section "test keys"
for k in alice bob mallory; do ensure_key "$k"; done
DEV0=$(addr dev0); ALICE=$(addr alice); BOB=$(addr bob); MALLORY=$(addr mallory)
echo "  dev0=$DEV0"; echo "  alice=$ALICE"; echo "  bob=$BOB"; echo "  mallory=$MALLORY (sanctioned)"

section "funding test accounts (gas)"
fund "$ALICE" 1000000000000000000
fund "$BOB"   1000000000000000000
fund "$MALLORY" 1000000000000000000
wait_blocks 1 >/dev/null
echo "  funded alice/bob/mallory with 1e18 ${DENOM}"

# ---- batched governance proposal ------------------------------------------
section "staging authority-gated setup proposal"
TMP="$(mktemp -d)"
gen_msg "$TMP/01_registrar.json" identity add-registrar --registrar "$DEV0" --display-name "Dev Registrar"
gen_msg "$TMP/02_sanction.json"  identity add-sanction  --address "$MALLORY" --list-source "OFAC-TEST" --reason "test sanction"
gen_msg "$TMP/03_topic.json"     assethub create-topic  --id "reserve.usd" --description "USD reserve PoR" --min-sources 1
gen_msg "$TMP/04_usd.json"       stableusd create-denom --id usd --symbol scnUSD --decimals 6 --peg-currency USD --admin "$DEV0"
gen_msg "$TMP/05_eur.json"       stableusd create-denom --id eur --symbol scnEUR --decimals 6 --peg-currency EUR --admin "$DEV0"
# market/bridge create commands take positional args (see autocli); flags would
# be silently dropped and the message omitted from the proposal.
gen_msg "$TMP/06_market.json"    market create-market usd eur 10 1 1 --operator "$DEV0"
gen_msg "$TMP/07_chain.json"     bridge register-chain ethereum "eip155:1" 1 --attestors "$DEV0"
gen_msg "$TMP/08_asset.json"     bridge register-asset usd
gen_msg "$TMP/09_issuer.json"    rwatoken add-issuer "$DEV0" --display-name "Dev Issuer"

# Guard against silently-dropped messages (a bad flag yields an empty file).
for f in "$TMP"/*.json; do
  if ! jq -e '."@type"' "$f" >/dev/null 2>&1; then
    echo "  ✗ generated message $(basename "$f") is invalid/empty — aborting"; rm -rf "$TMP"; exit 1
  fi
done

if gov_pass "rwa-core test bootstrap" \
    "$TMP/01_registrar.json" "$TMP/02_sanction.json" "$TMP/03_topic.json" \
    "$TMP/04_usd.json" "$TMP/05_eur.json" "$TMP/06_market.json" \
    "$TMP/07_chain.json" "$TMP/08_asset.json" "$TMP/09_issuer.json"; then
  echo "  ✓ governance setup applied"
else
  echo "  ✗ governance setup proposal failed — aborting"; rm -rf "$TMP"; exit 1
fi
rm -rf "$TMP"

# ---- listing bond -----------------------------------------------------------
# Markets created by governance start PENDING_BOND when params.listing_bond>0:
# the designated operator (dev0) must escrow the bond to open trading.
section "market: post listing bond (operator dev0)"
MKT_ID=$(q market markets | jq -r '.markets[-1].id // empty')
MKT_ID="${MKT_ID:-1}"
MKT_STATUS=$(q market market "$MKT_ID" | jq -r '.market.status // empty')
if [ "$MKT_STATUS" = "MARKET_STATUS_PENDING_BOND" ]; then
  expect_ok "post-bond market $MKT_ID" dev0 market post-bond "$MKT_ID"
else
  echo "  market $MKT_ID status=$MKT_STATUS — no bond required"
fi

# ---- dev0 self-admin seeding ----------------------------------------------
section "stableusd: minters + supply"
expect_ok "add-minter dev0 (usd)" dev0 stableusd add-minter --denom-id usd --minter "$DEV0"
expect_ok "add-minter dev0 (eur)" dev0 stableusd add-minter --denom-id eur --minter "$DEV0"
expect_ok "mint usd -> dev0"  dev0 stableusd mint usd "$DEV0"  100000000
expect_ok "mint usd -> alice" dev0 stableusd mint usd "$ALICE" 50000000
expect_ok "mint usd -> bob"   dev0 stableusd mint usd "$BOB"   50000000
expect_ok "mint eur -> alice" dev0 stableusd mint eur "$ALICE" 50000000
expect_ok "mint eur -> bob"   dev0 stableusd mint eur "$BOB"   50000000

section "assethub: provider + oracle value (PoR)"
expect_ok "register-provider dev0 (oracle)" dev0 assethub register-provider --role oracle --display-name "Dev Oracle" --bond 10000000
expect_ok "submit-value reserve.usd" dev0 assethub submit-value reserve.usd 100000000

section "rwatoken: token + supply"
expect_ok "create-token (admin dev0)" dev0 rwatoken create-token \
  --symbol SOLAR1 --name "Solar Revenue 1" --asset-class revenue_right --decimals 6 \
  --settlement-denom usd --require-kyc=false --redemption-price 1000000 \
  --redemption-delay-seconds 5 --per-holder-cap 0 --metadata-uri "ipfs://solar1"
RWA_ID=$(q rwatoken tokens | jq -r '.tokens[-1].id // .tokens[-1].token.id // empty')
RWA_ID="${RWA_ID:-1}"
echo "  rwatoken id=$RWA_ID"
expect_ok "rwatoken mint -> alice" dev0 rwatoken mint "$RWA_ID" "$ALICE" 1000000
expect_ok "rwatoken mint -> bob"   dev0 rwatoken mint "$RWA_ID" "$BOB"   1000000

section "mincast: market"
expect_ok "create-market (admin dev0)" dev0 mincast create-market \
  --denom mcx1 --name "Mincast Solar 1" --settlement-denom usd \
  --initial-price 1000000 --mint-fee-bps 50 --melt-fee-bps 50 --require-kyc=false
MCX_ID=$(q mincast markets | jq -r '.markets[-1].id // .markets[-1].market.id // empty')
MCX_ID="${MCX_ID:-1}"
echo "  mincast market id=$MCX_ID"

section "offering: IRO"
# Caps must be whole multiples of unit_price (price per RWA unit).
NOW=$(date +%s); END=$((NOW + 3600))
expect_ok "create-offering (issuer dev0)" dev0 offering create-offering \
  --token-id "$RWA_ID" --unit-price 1000000 --soft-cap 10000000 --hard-cap 100000000 \
  --start-time "$NOW" --end-time "$END" --total-tranches 1 --required-injection 0 --injection-interval 0

echo
echo "setup complete."
summary || true
# Persist resolved ids for module scripts.
cat > "${SCRIPT_DIR}/.fixtures.env" <<EOF
export DEV0=$DEV0
export ALICE=$ALICE
export BOB=$BOB
export MALLORY=$MALLORY
export RWA_ID=$RWA_ID
export MCX_ID=$MCX_ID
EOF
