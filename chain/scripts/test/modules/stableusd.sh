#!/usr/bin/env bash
# x/stableusd — WeUSD-style omni-settlement stablecoin.
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib.sh"
source "${SCRIPT_DIR}/.fixtures.env"

module_header "stableusd (settlement)"

section "positive"
expect_q "denom usd"             stableusd denom usd
expect_q "supply usd"            stableusd supply usd
expect_q_has "alice usd balance>0" '((.balance//.amount//"0")|tonumber) > 0' stableusd balance usd "$ALICE"
expect_ok "transfer usd alice->bob"      alice stableusd transfer usd "$BOB" 1000000
expect_ok "approve usd alice->bob"       alice stableusd approve  usd "$BOB" 500000
expect_q_has "allowance recorded" '((.allowance//.amount//"0")|tonumber) >= 500000' stableusd allowance usd "$ALICE" "$BOB"
expect_ok "burn usd (dev0)"              dev0  stableusd burn     usd 100000
expect_ok "redeem usd (alice)"           alice stableusd redeem   usd 1000000 "fiat-out"

section "negative"
expect_fail "mint by non-minter(alice) rejected" "minter|unauthor" \
  alice stableusd mint usd "$ALICE" 1
expect_fail "transfer over balance rejected" "insufficient|exceed|balance" \
  bob stableusd transfer usd "$ALICE" 999999999999
expect_fail "transfer to sanctioned mallory rejected" "sanction|compliance|frozen|blacklist" \
  alice stableusd transfer usd "$MALLORY" 1000

section "security"
expect_fail "create-denom by non-authority rejected" "authority|unauthor" \
  dev0 stableusd create-denom --id hax --symbol HAX --decimals 6 --peg-currency USD --admin "$DEV0"
expect_fail "add-minter by non-admin(alice) rejected" "admin|unauthor" \
  alice stableusd add-minter --denom-id usd --minter "$ALICE"
expect_fail "freeze by non-admin(alice) rejected" "admin|unauthor" \
  alice stableusd freeze --denom-id usd --account "$BOB"
# blacklist gate: dev0(admin) blacklists bob, transfers to bob must fail, then lift.
expect_ok   "admin blacklists bob"        dev0  stableusd blacklist   --denom-id usd --account "$BOB"
expect_fail "transfer to blacklisted bob rejected" "blacklist|compliance|frozen" \
  alice stableusd transfer usd "$BOB" 1000
expect_ok   "admin un-blacklists bob"     dev0  stableusd unblacklist --denom-id usd --account "$BOB"
expect_ok   "transfer to bob now allowed" alice stableusd transfer usd "$BOB" 1000

summary
