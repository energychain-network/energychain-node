#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — RWA issuer whitelist (governance).
#
# Creating an RWA share token (x/rwatoken MsgCreateToken) is gated: the admin
# must be a governance-whitelisted issuer (or the gov authority itself). This
# script submits ONE gov proposal carrying a MsgAddIssuer per target address,
# deposits the minimum, votes YES with the validator key, and waits for it to
# pass under fast governance (voting_period=12s).
#
# Run this ON THE UBUNTU HOST (server-side signing — no browser wallet, so it
# is unaffected by the frontend gas/RPC issues).
#
# Usage:
#   ./whitelist_issuers.sh                 # whitelist w1 (default)
#   ./whitelist_issuers.sh w1 w2 w3        # several keyring key names
#   ./whitelist_issuers.sh energy1abc...   # or raw bech32 addresses
#
# Env overrides (defaults target the local prod deployment):
#   BIN, HOME_T (keyring dir), NODE, CHAINID, DENOM, GAS_PRICES, GOV_DEPOSIT
# ===========================================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Defaults match the running prod-topology test deployment.
export BIN="${BIN:-/data/energychain/bin/energychaind}"
export HOME_T="${HOME_T:-/data/energychain/seedctl}"          # keyring holding dev0 + validator
export CHAINID="${CHAINID:-energychain_9001-1}"
export NODE="${NODE:-tcp://127.0.0.1:26957}"                  # full node RPC
export DENOM="${DENOM:-uecy}"
export KEYRING="${KEYRING:-test}"
export GAS_PRICES="${GAS_PRICES:-25000000000${DENOM}}"        # >= node minimum-gas-prices
export GOV_DEPOSIT="${GOV_DEPOSIT:-10000000000000000000000}" # >= gov min_deposit (10000 ECY)

# shellcheck source=../test/lib.sh
source "${CHAIN_DIR}/scripts/test/lib.sh"

ISSUERS=("$@"); [ ${#ISSUERS[@]} -eq 0 ] && ISSUERS=(w1)

GOV="$(gov_addr)"
[ -z "$GOV" ] && { echo "✗ could not resolve gov module address (is NODE=$NODE reachable?)"; exit 1; }
echo "gov authority : $GOV"
echo "proposer/voter: dev0 (deposit) + validator (vote)  keyring=$HOME_T"

TMP="$(mktemp -d)"; MSGS=()
for it in "${ISSUERS[@]}"; do
  if [[ "$it" == energy1* ]]; then a="$it"; name="$it"; else a="$(addr "$it")"; name="$it"; fi
  if [ -z "$a" ]; then echo "✗ cannot resolve issuer '$it' (not a bech32 addr and not a keyring key in $HOME_T)"; rm -rf "$TMP"; exit 1; fi
  f="${TMP}/issuer_${name//[^a-zA-Z0-9]/_}.json"
  jq -n --arg gov "$GOV" --arg iss "$a" --arg dn "$name" \
    '{"@type":"/energychain.rwatoken.v1.MsgAddIssuer", authority:$gov, issuer:$iss, display_name:$dn}' > "$f"
  MSGS+=("$f")
  echo "  + whitelist issuer: $name -> $a"
done

echo "submitting gov proposal (deposit=${GOV_DEPOSIT}${DENOM}, fast voting)…"
if gov_pass "rwa: whitelist issuer(s)" "${MSGS[@]}"; then
  echo "✅ done — issuer(s) whitelisted. They can now create RWA tokens on the Issue page."
  rc=0
else
  echo "❌ proposal did not pass."
  echo "   NOTE: MsgAddIssuer fails with 'already exists' if an address is ALREADY"
  echo "   whitelisted — drop that address from the args and retry."
  rc=1
fi
rm -rf "$TMP"
exit $rc
