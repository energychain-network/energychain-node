#!/usr/bin/env bash
# x/offering — Initial RWA Offering (IRO) subscription + allocation.
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib.sh"
source "${SCRIPT_DIR}/.fixtures.env"

module_header "offering (IRO)"
T="${RWA_ID:-1}"

section "positive — full lifecycle on a short offering"
NOW=$(date +%s); END=$((NOW + 6))
expect_ok "issuer creates short offering" dev0 offering create-offering \
  --token-id "$T" --unit-price 1000000 --soft-cap 2000000 --hard-cap 10000000 \
  --start-time "$NOW" --end-time "$END" --total-tranches 1 --required-injection 0 --injection-interval 0
OID=$(q offering offerings | jq -r '.offerings[-1].id // .offerings[-1].offering.id // empty'); OID="${OID:-1}"
echo "  offering id=$OID (ends in 6s)"
expect_q "offering $OID"                offering offering "$OID"
expect_ok "alice subscribes (reaches soft cap)" alice offering subscribe "$OID" 2000000
expect_q_has "raised >= soft cap" '((.offering.raised // .raised // "0")|tonumber) >= 2000000' offering offering "$OID"
echo "  waiting for offering window to end (EndBlocker auto-closes)…"; sleep 7; wait_blocks 2 >/dev/null
# The offering EndBlocker auto-closes a past-window raise; assert it reached
# SUCCEEDED on its own (no manual close needed).
expect_q_has "offering auto-closed SUCCEEDED" '((.offering.status // .status // "")|tostring|test("SUCCEEDED|^2$"))' offering offering "$OID"
expect_ok "alice claims allocation"      alice offering claim-allocation "$OID"
expect_q_has "alice now holds RWA units" '((.balance//.amount//"0")|tonumber) > 0' rwatoken balance "$T" "$ALICE"

section "negative"
NOW=$(date +%s); END=$((NOW + 3600))
expect_ok "issuer creates offering2" dev0 offering create-offering \
  --token-id "$T" --unit-price 1000000 --soft-cap 2000000 --hard-cap 10000000 \
  --start-time "$NOW" --end-time "$END" --total-tranches 1 --required-injection 0 --injection-interval 0
OID2=$(q offering offerings | jq -r '.offerings[-1].id // empty'); OID2="${OID2:-2}"
expect_fail "subscribe non-multiple of unit_price rejected" "multiple|invalid|unit" \
  alice offering subscribe "$OID2" 1500000
expect_fail "subscribe by sanctioned mallory rejected" "sanction|compliance|insufficient|frozen" \
  mallory offering subscribe "$OID2" 2000000

section "security"
expect_fail "cancel-offering by non-issuer(alice) rejected" "issuer|unauthor" \
  alice offering cancel-offering --offering-id "$OID2" --reason hax
expect_fail "inject by non-issuer(alice) rejected" "issuer|unauthor" \
  alice offering inject "$OID2" 1000000

summary
