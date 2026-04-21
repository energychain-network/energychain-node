#!/bin/bash
# Step 3 (run by COORDINATOR after collecting all gentxes + account snippets).
#
# Inputs:
#   $GENTX_DIR/gentx-*.json          <- collected from all validators
#   $GENTX_DIR/account-*.json        <- collected from all validators
#   $CEREMONY_DIR/extra-accounts.json (optional)  <- non-validator initial allocations
#
# Output:
#   $GENESIS_DIR/genesis.json        <- the final genesis to publish
#   $GENESIS_DIR/genesis.sha256      <- sha256 every node MUST match
#
# After finalize, every validator copies $GENESIS_DIR/genesis.json into
# $HOME/.energychaind/config/genesis.json AND verifies the sha256 with
# 04_verify_genesis.sh before starting the node.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/00_params.sh"

command -v jq >/dev/null 2>&1 || { echo "ERROR: jq required"; exit 1; }

WORK="$CEREMONY_DIR/finalize"
COORD_HOME="$WORK/.energychaind"
rm -rf "$WORK"
mkdir -p "$WORK" "$GENESIS_DIR"

cp -r "$CEREMONY_DIR/pre-genesis/.energychaind" "$COORD_HOME"

GENESIS="$COORD_HOME/config/genesis.json"
TMP="$COORD_HOME/config/tmp.json"

# 1. Merge every collected validator account.
echo "==> Merging $(ls "$GENTX_DIR"/account-*.json 2>/dev/null | wc -l | tr -d ' ') validator accounts"
for f in "$GENTX_DIR"/account-*.json; do
  [[ -e "$f" ]] || { echo "  (no account snippets found)"; break; }
  ADDR="$(jq -r '.validator_address' "$f")"
  AMT="$(jq -r '.amount' "$f")"
  echo "  + $ADDR :: ${AMT}${DENOM}"
  "$BINARY" genesis add-genesis-account "$ADDR" "${AMT}${DENOM}" --home "$COORD_HOME"
done

# 2. Merge any extra (non-validator) genesis accounts.
EXTRA="$CEREMONY_DIR/extra-accounts.json"
if [[ -f "$EXTRA" ]]; then
  echo "==> Merging extra (non-validator) accounts from $EXTRA"
  jq -c '.[]' "$EXTRA" | while read -r row; do
    ADDR="$(echo "$row" | jq -r '.address')"
    AMT="$(echo "$row" | jq -r '.amount')"
    echo "  + $ADDR :: ${AMT}${DENOM}"
    "$BINARY" genesis add-genesis-account "$ADDR" "${AMT}${DENOM}" --home "$COORD_HOME"
  done
fi

# 3. Stage gentxes.
mkdir -p "$COORD_HOME/config/gentx"
echo "==> Copying $(ls "$GENTX_DIR"/gentx-*.json 2>/dev/null | wc -l | tr -d ' ') validator gentxes"
for f in "$GENTX_DIR"/gentx-*.json; do
  [[ -e "$f" ]] || { echo "  (no gentxes found)"; exit 1; }
  cp "$f" "$COORD_HOME/config/gentx/"
done

# 4. Run collect-gentxs (this is what stitches all signed validator txs into
# the final app_state.genutil.gen_txs array).
echo "==> Running collect-gentxs"
"$BINARY" genesis collect-gentxs --home "$COORD_HOME"

# 5. Final validation.
echo "==> Validating final genesis"
"$BINARY" genesis validate-genesis --home "$COORD_HOME"

# 6. Canonicalize the genesis JSON (sorted keys, no trailing whitespace) so
# the sha256 is reproducible across operators on different platforms.
jq -S '.' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"

# 7. Publish.
cp "$GENESIS" "$GENESIS_DIR/genesis.json"
sha256sum "$GENESIS_DIR/genesis.json" | awk '{print $1}' > "$GENESIS_HASH_FILE"

echo ""
echo "============================================================"
echo "FINAL GENESIS PRODUCED"
echo "  file:    $GENESIS_DIR/genesis.json"
echo "  sha256:  $(cat "$GENESIS_HASH_FILE")"
echo "============================================================"
echo ""
echo "Publish BOTH the file and the sha256 over multiple channels (git tag,"
echo "discord pinned message, Twitter, etc). Every validator MUST run"
echo "04_verify_genesis.sh against this file BEFORE starting their node."
