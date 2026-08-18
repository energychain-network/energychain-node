#!/usr/bin/env bash
# x/bridge — cross-chain settlement bridge (threshold attestation).
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib.sh"
source "${SCRIPT_DIR}/.fixtures.env"

module_header "bridge (cross-chain settlement)"
CH=1   # ethereum chain from setup
AS=1   # usd asset from setup

section "positive — outbound lock"
expect_q "chain $CH"   bridge chain "$CH"
expect_q "asset $AS"   bridge asset "$AS"
expect_ok "dev0 locks usd to bridge out" dev0 bridge lock "$AS" 5000000 "$CH" "0xRecipientOnEth"
expect_q "outbounds listed"  bridge outbounds
expect_q_has "escrow holds locked usd" '((.balance//.amount//"0")|tonumber) >= 5000000' bridge escrow usd

section "positive — inbound attest + release"
expect_ok "attestor attests inbound -> alice" dev0 bridge attest "$CH" 1 "$ALICE" "$AS" 2000000
IB=$(q bridge inbounds | jq -r '.inbounds[-1].id // empty'); IB="${IB:-1}"
expect_q "inbound $IB"   bridge inbound "$IB"
expect_ok "release attested inbound" dev0 bridge release "$IB"

section "negative"
expect_fail "lock unregistered asset rejected" "not found|asset|exist" \
  dev0 bridge lock 999 1000 "$CH" "0xabc"
expect_fail "lock to unknown chain rejected" "not found|chain|exist" \
  dev0 bridge lock "$AS" 1000 999 "0xabc"

section "security"
expect_fail "register-chain by non-authority rejected" "authority|unauthor" \
  dev0 bridge register-chain hax "eip155:99" 1 --attestors "$DEV0"
expect_fail "register-asset by non-authority rejected" "authority|unauthor" \
  dev0 bridge register-asset eur
expect_fail "attest by non-attestor(alice) rejected" "attestor|unauthor" \
  alice bridge attest "$CH" 2 "$BOB" "$AS" 1000

summary
