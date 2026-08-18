#!/usr/bin/env bash
# x/identity — DID / KYC / sanctions / transfer policy + audit.
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib.sh"
source "${SCRIPT_DIR}/.fixtures.env"

module_header "identity (compliance)"

section "positive"
expect_q "params"            identity params
expect_q_has "registrar dev0 listed" "(.registrars // []) | any(.registrar==\"$DEV0\" or .address==\"$DEV0\")" identity registrars
expect_ok "registrar sets alice KYC-cleared" dev0 identity set-account \
  --address "$ALICE" --did "did:ec:alice" --kyc-cleared=true --accredited=true --jurisdiction US --kyc-expires-at 0
expect_q_has "alice account stored" '.account != null' identity account "$ALICE"
expect_ok "owner creates policy polx" dev0 identity create-policy \
  --id polx --description "kyc gate" --require-kyc=true --require-accredited=false --deny-frozen=true
expect_q "policy polx" identity policy polx
expect_q_has "mallory is sanctioned" '.sanctioned==true' identity sanctioned "$MALLORY"

section "negative"
expect_fail "set-account by non-registrar(bob) rejected" "registrar|unauthor" \
  bob identity set-account --address "$BOB" --kyc-cleared=true --accredited=false --jurisdiction US --kyc-expires-at 0
expect_fail "create duplicate policy polx rejected" "exist|already" \
  dev0 identity create-policy --id polx --description dup --require-kyc=false --require-accredited=false --deny-frozen=false
expect_fail "delete non-existent policy rejected" "not found|exist" \
  dev0 identity delete-policy nope

section "security"
expect_fail "add-registrar by non-authority rejected" "authority|unauthor" \
  dev0 identity add-registrar --registrar "$BOB" --display-name hax
expect_fail "add-sanction by non-authority rejected" "authority|unauthor" \
  dev0 identity add-sanction --address "$BOB" --list-source x --reason x
expect_fail "remove-sanction by non-authority rejected" "authority|unauthor" \
  dev0 identity remove-sanction "$MALLORY"
expect_fail "freeze-account by non-registrar(bob) rejected" "registrar|unauthor" \
  bob identity freeze-account --address "$ALICE" --reason hax

summary
