#!/usr/bin/env bash
# x/mincast — Origin-Mincast bonding curve (dynamic floor price).
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib.sh"
source "${SCRIPT_DIR}/.fixtures.env"

module_header "mincast (bonding curve / floor price)"
M="${MCX_ID:-1}"

section "positive"
expect_q "market $M"            mincast market "$M"
expect_q "quote mint"           mincast quote "$M" true 1000000
# Bonding-curve units are whole and pricey (~1 unit per ~1-3 settlement units),
# so mint generously and then operate on a fraction of the realised balance.
expect_ok "alice mints mincast" alice mincast mint "$M" 30000000 0
expect_q_has "alice holds mincast" '((.balance//.amount//"0")|tonumber) > 0' mincast balance "$M" "$ALICE"
MB=$(q mincast balance "$M" "$ALICE" | jq -r '.balance//.amount//"0"'); MB="${MB:-0}"
PART=$(( MB / 5 )); [ "$PART" -lt 1 ] && PART=1
echo "  alice mincast balance=$MB (operating on $PART per step)"
expect_ok "alice melts some"     alice mincast melt "$M" "$PART" 0
expect_ok "alice transfers units" alice mincast transfer "$M" "$BOB" "$PART"
expect_ok "dev0 injects treasury (lifts floor)" dev0 mincast inject "$M" 1000000
expect_ok "dev0 funds reward pool" dev0 mincast fund-reward "$M" 1000000
expect_ok "alice opens invest"   alice mincast open-invest "$M" "$PART" 60 1000

section "negative"
expect_fail "mint with impossible min-units-out rejected" "slippage|min_units|min units|insufficient" \
  alice mincast mint "$M" 1000000 999999999999
expect_fail "melt over balance rejected" "insufficient|exceed|balance" \
  bob mincast melt "$M" 999999999999 0

section "security"
expect_fail "update-market by non-admin(alice) rejected" "admin|unauthor" \
  alice mincast update-market --market-id "$M" --name hax --mint-fee-bps 0 --melt-fee-bps 0 --require-kyc=false
expect_fail "set-market-status by non-admin(alice) rejected" "admin|unauthor" \
  alice mincast set-market-status --market-id "$M" --status paused

summary
