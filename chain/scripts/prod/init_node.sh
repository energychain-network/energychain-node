#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — production single-validator node INIT (9-module RWA core).
#
# Builds the binary, initializes the node home, patches genesis and writes
# config for PUBLIC endpoints. It deliberately does NOT start the node —
# systemd owns the long-running process (see deploy.sh / energychaind.service).
#
# Genesis logic mirrors the proven scripts/local_node.sh bringup. The nine
# consolidated modules (identity / assethub / stableusd / rwatoken / mincast /
# offering / market / automation / bridge) all boot from a valid DefaultGenesis;
# runtime entities are seeded later by scripts/prod/seed.sh.
#
# Override anything via env: FRESH=1 wipes existing data, BUILD=0 skips build.
# ===========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

BINARY="${BINARY:-$CHAIN_DIR/energychaind}"
CHAINID="${CHAINID:-energychain_9001-1}"
MONIKER="${MONIKER:-energychain-prod}"
KEYALGO="eth_secp256k1"
DENOM="${DENOM:-uecy}"
CHAINDIR="${CHAINDIR:-$HOME/.energychaind}"
FRESH="${FRESH:-0}"
BUILD="${BUILD:-1}"

# ---- key security (fail-closed) --------------------------------------------
# KEYRING must be chosen explicitly. `file`/`os` store keys encrypted; `test`
# stores them in PLAINTEXT and is only acceptable for demo/load environments,
# so it additionally requires ALLOW_TEST_KEYRING=1 as an explicit ack.
KEYRING="${KEYRING:?ERROR: set KEYRING explicitly (file or os for production; test only with ALLOW_TEST_KEYRING=1)}"
if [ "$KEYRING" = "test" ] && [ "${ALLOW_TEST_KEYRING:-0}" != "1" ]; then
  echo "ERROR: KEYRING=test stores private keys in PLAINTEXT."
  echo "       Use KEYRING=file (with KEYRING_PASSWORD) or KEYRING=os for production."
  echo "       For a throwaway demo net, re-run with ALLOW_TEST_KEYRING=1."
  exit 1
fi
if [ "$KEYRING" = "file" ] && [ -z "${KEYRING_PASSWORD:-}" ]; then
  echo "ERROR: KEYRING=file requires KEYRING_PASSWORD (>= 8 chars) for non-interactive init."
  exit 1
fi

# Validator/treasury mnemonics MUST be injected via env — there are no
# defaults. A mnemonic committed to a repo is a public key giveaway: anyone
# holding it controls the validator and (single-validator) governance.
VAL_MNEMONIC="${VAL_MNEMONIC:?ERROR: export VAL_MNEMONIC (validator key mnemonic; generate offline, never commit it)}"
DEV_MNEMONIC="${DEV_MNEMONIC:?ERROR: export DEV_MNEMONIC (dev0 treasury key mnemonic; generate offline, never commit it)}"

# keyring_in <extra-lines...> emits the stdin a keyring-touching command
# expects: the payload lines first, then the passphrase twice for the `file`
# backend (create/unlock prompts). `test`/`os` backends ignore the extras.
keyring_in() {
  if [ "$#" -gt 0 ]; then printf '%s\n' "$@"; fi
  if [ "$KEYRING" = "file" ]; then
    printf '%s\n%s\n' "$KEYRING_PASSWORD" "$KEYRING_PASSWORD"
  fi
}

# Public listen addresses are applied as systemd start flags (see deploy.sh);
# the REST API bind lives in app.toml and defaults to loopback — front it
# with a reverse proxy / firewall before exposing it.
API_ADDR="${API_ADDR:-tcp://127.0.0.1:1317}"

# CORS: default DENY. Set CORS_ORIGINS="https://app.example.com" to allow
# known frontends, or UNSAFE_CORS=1 to reproduce the old wide-open demo mode.
CORS_ORIGINS="${CORS_ORIGINS:-}"
UNSAFE_CORS="${UNSAFE_CORS:-0}"

CONFIG_TOML="$CHAINDIR/config/config.toml"
APP_TOML="$CHAINDIR/config/app.toml"
GENESIS="$CHAINDIR/config/genesis.json"
TMP="$CHAINDIR/config/tmp_genesis.json"

command -v jq >/dev/null 2>&1 || { echo "ERROR: jq is required"; exit 1; }

if [ "$BUILD" = "1" ]; then
  command -v go >/dev/null 2>&1 || { echo "ERROR: go is required to build"; exit 1; }
  echo "[build] go build -> $BINARY"
  ( cd "$CHAIN_DIR" && go build -o "$BINARY" ./cmd/energychaind )
fi

if [ -f "$GENESIS" ] && [ "$FRESH" != "1" ]; then
  echo "[init] $CHAINDIR already initialized (set FRESH=1 to wipe). Skipping."
  exit 0
fi
echo "[init] (re)initializing $CHAINDIR"
rm -rf "$CHAINDIR"

$BINARY config set client chain-id "$CHAINID" --home "$CHAINDIR"
$BINARY config set client keyring-backend "$KEYRING" --home "$CHAINDIR"
$BINARY config set client node "tcp://127.0.0.1:26657" --home "$CHAINDIR"

keyring_in "$VAL_MNEMONIC" | $BINARY keys add validator --recover --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$CHAINDIR"
keyring_in "$DEV_MNEMONIC" | $BINARY keys add dev0      --recover --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$CHAINDIR"
echo "$VAL_MNEMONIC" | $BINARY init "$MONIKER" -o --chain-id "$CHAINID" --home "$CHAINDIR" --recover

# ---- genesis (denoms, evm, feemarket, fast gov) ---------------------------
jq --arg d "$DENOM" '.app_state.staking.params.bond_denom=$d'              "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.gov.params.min_deposit[0].denom=$d'         "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.gov.params.expedited_min_deposit[0].denom=$d' "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.evm.params.evm_denom=$d'                    "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.mint.params.mint_denom=$d'                  "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"

jq '.app_state.bank.denom_metadata=[{"description":"Energy Chain native token","denom_units":[{"denom":"uecy","exponent":0,"aliases":["microecy"]},{"denom":"ecy","exponent":18,"aliases":[]}],"base":"uecy","display":"ecy","name":"Energy Chain Yield","symbol":"ECY","uri":"","uri_hash":""}]' "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"

jq '.app_state.evm.params.active_static_precompiles=["0x0000000000000000000000000000000000000100","0x0000000000000000000000000000000000000400","0x0000000000000000000000000000000000000800","0x0000000000000000000000000000000000000801","0x0000000000000000000000000000000000000802","0x0000000000000000000000000000000000000803","0x0000000000000000000000000000000000000804","0x0000000000000000000000000000000000000805","0x0000000000000000000000000000000000000806","0x0000000000000000000000000000000000000807"]' "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"
jq '.app_state.erc20.native_precompiles=["0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"]' "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.erc20.token_pairs=[{contract_owner:1,erc20_address:"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",denom:$d,enabled:true}]' "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"

jq '.consensus.params.block.max_gas="120000000"' "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"
jq '.app_state.feemarket.params.min_gas_price="10000000000.000000000000000000"' "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"
jq '.app_state.feemarket.params.base_fee="10000000000.000000000000000000"'      "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"

# Fast governance so the single bootstrap proposal (seed.sh) lands in seconds.
sed -i.bak 's/"max_deposit_period": "172800s"/"max_deposit_period": "20s"/g'     "$GENESIS"
sed -i.bak 's/"voting_period": "172800s"/"voting_period": "8s"/g'                 "$GENESIS"
sed -i.bak 's/"expedited_voting_period": "86400s"/"expedited_voting_period": "6s"/g' "$GENESIS"
jq --arg d "$DENOM" '.app_state.gov.params.min_deposit[0]={"denom":$d,"amount":"1000000"}'           "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.gov.params.expedited_min_deposit[0]={"denom":$d,"amount":"2000000"}' "$GENESIS" >"$TMP" && mv "$TMP" "$GENESIS"

# ---- genesis accounts (validator + dev0 treasury) -------------------------
keyring_in | $BINARY genesis add-genesis-account validator 100000000000000000000000000${DENOM} --keyring-backend "$KEYRING" --home "$CHAINDIR"
keyring_in | $BINARY genesis add-genesis-account dev0      100000000000000000000000000${DENOM} --keyring-backend "$KEYRING" --home "$CHAINDIR"

# ---- config (public + production hygiene) ---------------------------------
sed -i.bak 's/timeout_propose = "3s"/timeout_propose = "2s"/g' "$CONFIG_TOML"
sed -i.bak 's/timeout_commit = "5s"/timeout_commit = "2s"/g'   "$CONFIG_TOML"
sed -i.bak '/^\[mempool\]$/,/^\[/ s|^type = .*|type = "app"|'  "$CONFIG_TOML"
sed -i.bak '/^\[mempool\]$/,/^\[/ s|^size = .*|size = 30000|'  "$CONFIG_TOML"
sed -i.bak 's/prometheus = false/prometheus = true/'          "$CONFIG_TOML"
# CORS: deny by default; allow only explicitly whitelisted origins (or the
# old wide-open behaviour when UNSAFE_CORS=1 is explicitly acked).
if [ "$UNSAFE_CORS" = "1" ]; then
  sed -i.bak 's|cors_allowed_origins = \[\]|cors_allowed_origins = ["*"]|g' "$CONFIG_TOML"
elif [ -n "$CORS_ORIGINS" ]; then
  CORS_JSON=$(printf '%s' "$CORS_ORIGINS" | jq -R 'split(",") | map(gsub("^\\s+|\\s+$";""))' -c)
  sed -i.bak "s|cors_allowed_origins = \[\]|cors_allowed_origins = ${CORS_JSON}|g" "$CONFIG_TOML"
fi

# app.toml: enable REST + JSON-RPC + tx indexer, app-side evm mempool.
sed -i.bak 's/^enable = false/enable = true/g'           "$APP_TOML"
sed -i.bak 's/^enabled = false/enabled = true/g'         "$APP_TOML"
sed -i.bak 's/enable-indexer = false/enable-indexer = true/g' "$APP_TOML"
if [ "$UNSAFE_CORS" = "1" ]; then
  sed -i.bak 's/enabled-unsafe-cors = false/enabled-unsafe-cors = true/g' "$APP_TOML"
fi
sed -i.bak '/^\[evm.mempool\]$/,/^\[/ s|^operate-exclusively = .*|operate-exclusively = true|' "$APP_TOML"
sed -i.bak "s|address = \"tcp://localhost:1317\"|address = \"$API_ADDR\"|g" "$APP_TOML"
sed -i.bak "s|address = \"tcp://0.0.0.0:1317\"|address = \"$API_ADDR\"|g"   "$APP_TOML"

# ---- finalize -------------------------------------------------------------
keyring_in | $BINARY genesis gentx validator 1000000000000000000000000${DENOM} \
  --gas-prices 10000000000${DENOM} --keyring-backend "$KEYRING" \
  --chain-id "$CHAINID" --home "$CHAINDIR"
$BINARY genesis collect-gentxs --home "$CHAINDIR"
$BINARY genesis validate --home "$CHAINDIR"

rm -f "$CONFIG_TOML.bak" "$APP_TOML.bak" "$GENESIS.bak" 2>/dev/null || true

echo ""
echo "[init] OK — chain-id=$CHAINID home=$CHAINDIR denom=$DENOM"
echo "[init] dev0=$(keyring_in | $BINARY keys show dev0 -a --keyring-backend "$KEYRING" --home "$CHAINDIR")"
echo "[init] start it via systemd (deploy.sh) — do NOT 'start' here."
