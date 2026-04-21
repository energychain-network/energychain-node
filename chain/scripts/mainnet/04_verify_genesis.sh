#!/bin/bash
# Step 4 (run by EVERY VALIDATOR before starting the chain).
#
# Verifies that the genesis.json staged at $HOME/.energychaind/config/genesis.json
# matches the published canonical sha256.
#
# Usage:
#   EXPECTED_SHA256=<hash from coordinator>  ./04_verify_genesis.sh
#
# Optional:
#   GENESIS_PATH=/custom/path/to/genesis.json  (defaults to $HOME/.energychaind/config/genesis.json)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/00_params.sh"

: "${EXPECTED_SHA256:?EXPECTED_SHA256 is required (the canonical hash from the coordinator)}"

GENESIS_PATH="${GENESIS_PATH:-$HOME/.energychaind/config/genesis.json}"

if [[ ! -f "$GENESIS_PATH" ]]; then
  echo "ERROR: genesis.json not found at $GENESIS_PATH"
  exit 1
fi

ACTUAL="$(sha256sum "$GENESIS_PATH" | awk '{print $1}')"
echo "  file:     $GENESIS_PATH"
echo "  expected: $EXPECTED_SHA256"
echo "  actual:   $ACTUAL"

if [[ "$ACTUAL" != "$EXPECTED_SHA256" ]]; then
  echo ""
  echo "FATAL: genesis hash mismatch. Do NOT start the node with this file."
  echo "Re-fetch genesis.json from the coordinator and try again."
  exit 1
fi

# Sanity-validate the genesis is parsable by our binary.
"$BINARY" genesis validate-genesis --home "$(dirname "$(dirname "$GENESIS_PATH")")" >/dev/null

# Sanity check: chain-id matches.
CID="$(jq -r '.chain_id' "$GENESIS_PATH")"
if [[ "$CID" != "$CHAIN_ID" ]]; then
  echo "FATAL: chain-id mismatch. genesis says '$CID', expected '$CHAIN_ID'."
  exit 1
fi

echo ""
echo "OK: genesis verified. Safe to start node."
