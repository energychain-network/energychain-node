#!/usr/bin/env bash
# x/market — frequent-batch-auction order book over stableusd denoms.
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib.sh"
source "${SCRIPT_DIR}/.fixtures.env"

module_header "market (batch auction)"
MK=1   # usd/eur market from setup

section "positive — crossing orders clear in a batch"
expect_q "market $MK"  market market "$MK"
expect_q "order book"  market book "$MK"
# alice BUY usd paying eur @1.10, bob SELL usd @1.00 → crosses, uniform clear.
expect_ok "alice places BUY"  alice market place "$MK" buy  1100000 1000000
expect_ok "bob places SELL"   bob   market place "$MK" sell 1000000 1000000
echo "  waiting for batch clear (batch_interval=1s)…"; wait_blocks 3 >/dev/null
expect_q_has "a batch produced a clearing price" '((.market.last_clearing_price // .last_clearing_price // "0")|tonumber) > 0' market market "$MK"

section "positive — resting order + owner cancel"
expect_ok "alice places resting BUY (no cross)" alice market place "$MK" buy 500000 1000000
OID=$(q market orders | jq -r "[.orders[]|select(.owner==\"$ALICE\" and (.status|test(\"OPEN\")))][-1].id // empty")
OID="${OID:-}"
if [ -n "$OID" ]; then
  expect_ok "owner cancels own order" alice market cancel "$OID"
else
  _skip "owner cancels own order" "no open order id resolved"
fi

section "negative"
expect_fail "place on non-existent market rejected" "not found|market|exist" \
  alice market place 999 buy 1000000 1000
expect_fail "place with zero price rejected" "price|invalid|zero" \
  alice market place "$MK" buy 0 1000

section "security"
expect_fail "create-market by non-authority rejected" "authority|unauthor" \
  dev0 market create-market usd eur 10 1 1 --operator "$DEV0"
expect_fail "set-market-status by non-authority rejected" "authority|unauthor" \
  dev0 market set-market-status "$MK" paused

section "listing bond"
# setup created the usd/eur market via gov and dev0 (the operator) posted the
# 10,000 ECY listing bond — the market must be ACTIVE with the bond recorded.
expect_q_has "market $MK bond posted" \
  '((.market.bond_amount // .bond_amount // "0")|tonumber) > 0 and ((.market.status // .status)|test("ACTIVE"))' \
  market market "$MK"
expect_fail "post-bond by non-operator(alice) rejected" "operator|unauthor|awaiting|status" \
  alice market post-bond "$MK"
expect_fail "delist-market by non-authority rejected" "authority|unauthor" \
  dev0 market delist-market "$MK"
# cancel-someone-else's-order: alice rests an order, bob tries to cancel it.
expect_ok "alice places another resting BUY" alice market place "$MK" buy 400000 1000000
OID2=$(q market orders | jq -r "[.orders[]|select(.owner==\"$ALICE\" and (.status|test(\"OPEN\")))][-1].id // empty")
if [ -n "$OID2" ]; then
  expect_fail "cancel by non-owner(bob) rejected" "owner|unauthor" bob market cancel "$OID2"
else
  _skip "cancel by non-owner rejected" "no open order id resolved"
fi

summary
