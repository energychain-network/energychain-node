#!/bin/bash
# Step 1 (run by COORDINATOR ONLY).
#
# Produces the "pre-genesis" genesis.json: chain-id + finalized parameters,
# but no validator gentx and no initial accounts. Coordinator publishes the
# resulting file to all validators who then locally `add-genesis-account`
# their own validator address and produce a gentx (step 02).
#
# Output:
#   $CEREMONY_DIR/pre-genesis/genesis.json   <- distribute to all validators

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/00_params.sh"

command -v jq >/dev/null 2>&1 || { echo "ERROR: jq required"; exit 1; }
command -v "$BINARY" >/dev/null 2>&1 || { echo "ERROR: $BINARY not on PATH"; exit 1; }

WORK="$CEREMONY_DIR/pre-genesis"
mkdir -p "$WORK" "$GENTX_DIR" "$GENESIS_DIR"

# Use a throwaway home for ceremony bootstrapping. The coordinator does NOT
# end up as a validator from this directory; their real validator key lives
# elsewhere and they participate via step 02 like everyone else.
COORD_HOME="$WORK/.energychaind"
rm -rf "$COORD_HOME"

echo "==> Initializing pre-genesis (chain-id=$CHAIN_ID denom=$DENOM)"
"$BINARY" init "ceremony-coordinator" --chain-id "$CHAIN_ID" --home "$COORD_HOME" >/dev/null 2>&1

GENESIS="$COORD_HOME/config/genesis.json"
TMP="$COORD_HOME/config/tmp.json"

apply_jq() {
  jq "$1" "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"
}

echo "==> Setting bond denom + supply parameters"
apply_jq ".app_state.staking.params.bond_denom = \"$DENOM\""
apply_jq ".app_state.staking.params.unbonding_time = \"$UNBONDING_PERIOD\""
apply_jq ".app_state.staking.params.max_validators = $MAX_VALIDATORS"
apply_jq ".app_state.staking.params.historical_entries = $HISTORICAL_ENTRIES"
apply_jq ".app_state.crisis.constant_fee = {\"denom\":\"$CRISIS_FEE_DENOM\",\"amount\":\"$CRISIS_FEE_AMOUNT\"}"

echo "==> Configuring governance parameters"
apply_jq ".app_state.gov.params.min_deposit[0] = {\"denom\":\"$DENOM\",\"amount\":\"$MIN_DEPOSIT\"}"
apply_jq ".app_state.gov.params.expedited_min_deposit[0] = {\"denom\":\"$DENOM\",\"amount\":\"$EXPEDITED_MIN_DEPOSIT\"}"
apply_jq ".app_state.gov.params.voting_period = \"$VOTING_PERIOD\""
apply_jq ".app_state.gov.params.expedited_voting_period = \"$EXPEDITED_VOTING_PERIOD\""
apply_jq ".app_state.gov.params.max_deposit_period = \"$DEPOSIT_PERIOD\""
apply_jq ".app_state.gov.params.quorum = \"$QUORUM\""
apply_jq ".app_state.gov.params.threshold = \"$THRESHOLD\""
apply_jq ".app_state.gov.params.expedited_threshold = \"$EXPEDITED_THRESHOLD\""
apply_jq ".app_state.gov.params.veto_threshold = \"$VETO_THRESHOLD\""

echo "==> Configuring slashing"
apply_jq ".app_state.slashing.params.signed_blocks_window = \"$SIGNED_BLOCKS_WINDOW\""
apply_jq ".app_state.slashing.params.min_signed_per_window = \"$MIN_SIGNED_PER_WINDOW\""
apply_jq ".app_state.slashing.params.downtime_jail_duration = \"$DOWNTIME_JAIL_DURATION\""
apply_jq ".app_state.slashing.params.slash_fraction_double_sign = \"$SLASH_DOUBLESIGN\""
apply_jq ".app_state.slashing.params.slash_fraction_downtime = \"$SLASH_DOWNTIME\""

echo "==> Configuring mint"
apply_jq ".app_state.mint.params.mint_denom = \"$DENOM\""
apply_jq ".app_state.mint.params.inflation_rate_change = \"0.13\""
apply_jq ".app_state.mint.params.inflation_max = \"$INFLATION_MAX\""
apply_jq ".app_state.mint.params.inflation_min = \"$INFLATION_MIN\""
apply_jq ".app_state.mint.params.goal_bonded = \"$GOAL_BONDED\""
apply_jq ".app_state.mint.params.blocks_per_year = \"10512000\""
apply_jq ".app_state.mint.minter.inflation = \"$INFLATION_RATE\""

echo "==> Configuring EVM + feemarket"
apply_jq ".app_state.evm.params.evm_denom = \"$DENOM\""
apply_jq ".app_state.feemarket.params.min_gas_price = \"$MIN_GAS_PRICE_DEC\""
apply_jq ".app_state.feemarket.params.base_fee = \"$MIN_GAS_PRICE_DEC\""

echo "==> Configuring bank metadata"
apply_jq ".app_state.bank.denom_metadata = [{
  \"description\":\"Energy Chain native token\",
  \"denom_units\":[
    {\"denom\":\"$DENOM\",\"exponent\":0,\"aliases\":[\"micro$DISPLAY_DENOM\"]},
    {\"denom\":\"$DISPLAY_DENOM\",\"exponent\":$DECIMALS,\"aliases\":[]}
  ],
  \"base\":\"$DENOM\",
  \"display\":\"$DISPLAY_DENOM\",
  \"name\":\"Energy Chain Yield\",
  \"symbol\":\"ECY\",
  \"uri\":\"\",
  \"uri_hash\":\"\"
}]"

echo "==> Configuring consensus block params"
apply_jq ".consensus.params.block.max_gas = \"$MAX_BLOCK_GAS\""

echo "==> Validating pre-genesis"
"$BINARY" genesis validate-genesis --home "$COORD_HOME"

cp "$GENESIS" "$WORK/genesis.json"

# Pre-genesis hash is informational; the FINAL ceremony hash is computed by
# step 03 after all gentxes are merged. Validators should still verify the
# pre-genesis they receive matches the coordinator's published value before
# producing their gentx.
sha256sum "$WORK/genesis.json" > "$WORK/genesis.sha256"
echo ""
echo "============================================================"
echo "PRE-GENESIS PRODUCED"
echo "  file:  $WORK/genesis.json"
echo "  sha256:  $(awk '{print $1}' "$WORK/genesis.sha256")"
echo "============================================================"
echo ""
echo "Distribute the file (and hash) to all validators."
echo "Each validator then runs 02_validator_gentx.sh."
