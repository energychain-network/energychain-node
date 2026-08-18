#!/usr/bin/env bash
# x/assethub — data-trust layer: bonded providers, devices, oracle topics.
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib.sh"
source "${SCRIPT_DIR}/.fixtures.env"

module_header "assethub (data trust)"

section "positive — oracle provider (dev0)"
expect_q "provider dev0"            assethub provider "$DEV0"
expect_q "providers"               assethub providers
expect_q "topic reserve.usd"       assethub topic reserve.usd
expect_q "submission reserve.usd"  assethub submission reserve.usd "$DEV0"
expect_ok "dev0 updates oracle value" dev0 assethub submit-value reserve.usd 100500000
expect_ok "dev0 increases bond"       dev0 assethub increase-bond 1000000

section "positive — device provider (alice) + device lifecycle"
expect_ok "alice registers as device provider" alice assethub register-provider \
  --role device --display-name "Alice Devices" --bond 10000000
expect_ok "alice registers device"   alice assethub register-device \
  --id dev1 --device-type solar_inverter --pubkey 0xpub01 --jurisdiction US
expect_q "device dev1"               assethub device dev1
expect_ok "alice attests device"     alice assethub attest-device \
  --id dev1 --attestation-hash 0xhash01 --firmware v1.0.0

section "negative"
expect_fail "submit-value to unknown topic rejected" "not found|topic|exist" \
  dev0 assethub submit-value no.such.topic 1
expect_fail "submit-value by non-provider(bob) rejected" "provider|unauthor|not " \
  bob assethub submit-value reserve.usd 1

section "security"
expect_fail "create-topic by non-authority rejected" "authority|unauthor" \
  dev0 assethub create-topic --id hax.topic --description hax --min-sources 1
expect_fail "slash-provider by non-authority rejected" "authority|unauthor" \
  dev0 assethub slash-provider --provider "$DEV0" --reason hax

summary
