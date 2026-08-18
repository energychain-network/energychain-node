#!/usr/bin/env bash
# x/rwatoken — compliant security token with dividend snapshots + redemptions.
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib.sh"
source "${SCRIPT_DIR}/.fixtures.env"

module_header "rwatoken (RWA security token)"
T="${RWA_ID:-1}"

section "positive"
expect_q "token $T"                 rwatoken token "$T"
expect_q_has "alice holds units" '((.balance//.amount//"0")|tonumber) > 0' rwatoken balance "$T" "$ALICE"
expect_ok "transfer units alice->bob"   alice rwatoken transfer "$T" "$BOB" 1000
expect_ok "admin takes snapshot"        dev0  rwatoken snapshot "$T"
SNAP=$(q rwatoken token "$T" | jq -r '.token.last_snapshot_id // .last_snapshot_id // 1' 2>/dev/null); SNAP="${SNAP:-1}"
expect_ok "admin creates distribution"  dev0  rwatoken distribute "$T" "$SNAP" 2000000
DIST=$(q rwatoken distribution 1 >/dev/null 2>&1 && echo 1 || echo 1)
expect_q "distribution $DIST"           rwatoken distribution "$DIST"
expect_ok "alice claims dividend"       alice rwatoken claim "$DIST"
expect_ok "admin funds redemption pool" dev0  rwatoken fund-pool "$T" 5000000
expect_ok "alice requests redemption"   alice rwatoken redeem "$T" 1000

section "negative"
expect_fail "create-token by non-issuer(alice) rejected" "issuer|unauthor" \
  alice rwatoken create-token \
  --symbol NOISS --name "Not Whitelisted" --asset-class revenue_right --decimals 0 \
  --settlement-denom usd --require-kyc=false --redemption-price 0 \
  --redemption-delay-seconds 0 --per-holder-cap 0 --metadata-uri ""
expect_q_has "dev0 is whitelisted issuer" '.is_issuer == true' rwatoken is-issuer "$DEV0"
expect_fail "mint by non-admin(alice) rejected" "admin|unauthor" \
  alice rwatoken mint "$T" "$ALICE" 1
expect_fail "transfer over balance rejected" "insufficient|exceed|balance" \
  bob rwatoken transfer "$T" "$ALICE" 999999999999

section "security"
expect_fail "distribute by non-admin(alice) rejected" "admin|unauthor" \
  alice rwatoken distribute "$T" "$SNAP" 1000
expect_fail "freeze by non-admin(alice) rejected" "admin|unauthor" \
  alice rwatoken freeze --token-id "$T" --holder "$BOB"
expect_fail "set-token-status by non-admin(alice) rejected" "admin|unauthor" \
  alice rwatoken set-token-status --token-id "$T" --status paused

summary
