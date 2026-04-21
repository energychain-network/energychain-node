#!/bin/bash
set -e

# 停止已有的 energychaind 进程
if pgrep -x energychaind > /dev/null 2>&1; then
  echo "Stopping existing energychaind process..."
  pkill -x energychaind || true
  sleep 2
fi

BINARY="energychaind"
CHAINID="energychain_9001-1"
MONIKER="energychain-local"
KEYRING="test"
KEYALGO="eth_secp256k1"
DENOM="uecy"
LOGLEVEL="info"
CHAINDIR="$HOME/.energychaind"

CONFIG_TOML=$CHAINDIR/config/config.toml
APP_TOML=$CHAINDIR/config/app.toml
GENESIS=$CHAINDIR/config/genesis.json
TMP_GENESIS=$CHAINDIR/config/tmp_genesis.json

command -v jq >/dev/null 2>&1 || { echo "jq is required. Install it first."; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN_DIR="$(dirname "$SCRIPT_DIR")"

# Build binary if not found
if ! command -v $BINARY >/dev/null 2>&1; then
  if [ -f "$CHAIN_DIR/$BINARY" ]; then
    BINARY="$CHAIN_DIR/$BINARY"
  else
    echo "Building $BINARY..."
    cd "$CHAIN_DIR" && go build -o $BINARY ./cmd/energychaind
    BINARY="$CHAIN_DIR/$BINARY"
  fi
fi

overwrite=""
while [[ $# -gt 0 ]]; do
  case $1 in
    -y) overwrite="y"; shift ;;
    -n) overwrite="n"; shift ;;
    *) shift ;;
  esac
done

if [[ -z "$overwrite" && -d "$CHAINDIR" ]]; then
  echo "Found existing chain data at $CHAINDIR"
  echo "Overwrite? [y/n]"
  read -r overwrite
fi
[[ -z "$overwrite" ]] && overwrite="y"

# Mnemonics: read from env or use defaults for LOCAL DEV ONLY
# For production, ALWAYS set these via environment variables
VAL_MNEMONIC="${VAL_MNEMONIC:-gesture inject test cycle original hollow east ridge hen combine junk child bacon zero hope comfort vacuum milk pitch cage oppose unhappy lunar seat}"

DEV_MNEMONIC="${DEV_MNEMONIC:-copper push brief egg scan entry inform record adjust fossil boss egg comic alien upon aspect dry avoid interest fury window hint race symptom}"

if [[ "$overwrite" == "y" || "$overwrite" == "Y" ]]; then
  rm -rf "$CHAINDIR"

  $BINARY config set client chain-id "$CHAINID" --home "$CHAINDIR"
  $BINARY config set client keyring-backend "$KEYRING" --home "$CHAINDIR"
  $BINARY config set client node "tcp://127.0.0.1:26657" --home "$CHAINDIR"

  echo "$VAL_MNEMONIC" | $BINARY keys add validator --recover --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$CHAINDIR"
  echo "$DEV_MNEMONIC" | $BINARY keys add dev0 --recover --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$CHAINDIR"

  echo "$VAL_MNEMONIC" | $BINARY init "$MONIKER" -o --chain-id "$CHAINID" --home "$CHAINDIR" --recover

  # Customize genesis for energy chain
  jq --arg denom "$DENOM" '.app_state["staking"]["params"]["bond_denom"]=$denom' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg denom "$DENOM" '.app_state["gov"]["params"]["min_deposit"][0]["denom"]=$denom' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg denom "$DENOM" '.app_state["gov"]["params"]["expedited_min_deposit"][0]["denom"]=$denom' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg denom "$DENOM" '.app_state["evm"]["params"]["evm_denom"]=$denom' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg denom "$DENOM" '.app_state["mint"]["params"]["mint_denom"]=$denom' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  jq '.app_state["bank"]["denom_metadata"]=[{"description":"Energy Chain native token","denom_units":[{"denom":"uecy","exponent":0,"aliases":["microecy"]},{"denom":"ecy","exponent":18,"aliases":[]}],"base":"uecy","display":"ecy","name":"Energy Chain Yield","symbol":"ECY","uri":"","uri_hash":""}]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  jq '.app_state["evm"]["params"]["active_static_precompiles"]=["0x0000000000000000000000000000000000000100","0x0000000000000000000000000000000000000400","0x0000000000000000000000000000000000000800","0x0000000000000000000000000000000000000801","0x0000000000000000000000000000000000000802","0x0000000000000000000000000000000000000803","0x0000000000000000000000000000000000000804","0x0000000000000000000000000000000000000805","0x0000000000000000000000000000000000000806","0x0000000000000000000000000000000000000807"]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  jq '.app_state.erc20.native_precompiles=["0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg denom "$DENOM" '.app_state.erc20.token_pairs=[{contract_owner:1,erc20_address:"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",denom:$denom,enabled:true}]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  jq '.consensus.params.block.max_gas="60000000"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  # Set feemarket: min_gas_price=10 Gwei, base_fee=10 Gwei
  jq '.app_state["feemarket"]["params"]["min_gas_price"]="10000000000.000000000000000000"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq '.app_state["feemarket"]["params"]["base_fee"]="10000000000.000000000000000000"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  # Speed up for dev: shorter voting periods
  sed -i.bak 's/"max_deposit_period": "172800s"/"max_deposit_period": "30s"/g' "$GENESIS"
  sed -i.bak 's/"voting_period": "172800s"/"voting_period": "30s"/g' "$GENESIS"
  sed -i.bak 's/"expedited_voting_period": "86400s"/"expedited_voting_period": "15s"/g' "$GENESIS"

  # Fund validator and dev account
  $BINARY genesis add-genesis-account validator 100000000000000000000000000${DENOM} --keyring-backend "$KEYRING" --home "$CHAINDIR"
  $BINARY genesis add-genesis-account dev0 1000000000000000000000${DENOM} --keyring-backend "$KEYRING" --home "$CHAINDIR"

  # ----------------------------------------------------------------------
  # Bootstrap custom modules so the dev account can drive real traffic
  # without needing governance proposals first.
  #
  #   * identity.params.admin_address = dev0      → dev0 can register/revoke
  #   * oracle.oracles += dev0 (any-category)     → dev0 can submit any feed
  #
  # In production these are seeded via genesis/governance from real keys.
  # ----------------------------------------------------------------------
  DEV0_ADDR=$($BINARY keys show dev0 -a --keyring-backend "$KEYRING" --home "$CHAINDIR")
  echo "Bootstrapping custom-module access for dev0=$DEV0_ADDR"

  jq --arg addr "$DEV0_ADDR" '.app_state["identity"]["params"]["admin_address"]=$addr' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  jq --arg addr "$DEV0_ADDR" '.app_state["oracle"]["oracles"]=[{
    "address": $addr,
    "name": "dev0-bootstrap-oracle",
    "active": true,
    "authorized_categories": []
  }]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  # Config tweaks
  sed -i.bak 's/timeout_propose = "3s"/timeout_propose = "2s"/g' "$CONFIG_TOML"
  sed -i.bak 's/timeout_commit = "5s"/timeout_commit = "2s"/g' "$CONFIG_TOML"
  # Larger app-side mempool for batch uploads
  sed -i.bak '/^\[mempool\]$/,/^\[/ s|^type = .*|type = "app"|' "$CONFIG_TOML"
  sed -i.bak '/^\[mempool\]$/,/^\[/ s|^size = .*|size = 20000|' "$CONFIG_TOML"
  sed -i.bak 's/prometheus = false/prometheus = true/' "$CONFIG_TOML"
  sed -i.bak 's/enabled = false/enabled = true/g' "$APP_TOML"
  sed -i.bak 's/enable = false/enable = true/g' "$APP_TOML"
  sed -i.bak 's/enable-indexer = false/enable-indexer = true/g' "$APP_TOML"
  sed -i.bak '/^\[evm.mempool\]$/,/^\[/ s|^operate-exclusively = .*|operate-exclusively = true|' "$APP_TOML"
  # CORS: allow all origins in dev; for production use specific domains
  CORS_ORIGINS="${CORS_ALLOWED_ORIGINS:-*}"
  sed -i.bak "s/enabled-unsafe-cors = false/enabled-unsafe-cors = true/g" "$APP_TOML"
  sed -i.bak "s|cors_allowed_origins = \[\]|cors_allowed_origins = [\"$CORS_ORIGINS\"]|g" "$CONFIG_TOML"

  # Create genesis tx and finalize
  $BINARY genesis gentx validator 1000000000000000000000${DENOM} --gas-prices 10000000000${DENOM} --keyring-backend "$KEYRING" --chain-id "$CHAINID" --home "$CHAINDIR"
  $BINARY genesis collect-gentxs --home "$CHAINDIR"
  $BINARY genesis validate-genesis --home "$CHAINDIR"

  echo ""
  echo "=== Energy Chain testnet initialized ==="
  echo "Chain ID:   $CHAINID"
  echo "Denom:      $DENOM"
  echo "Home:       $CHAINDIR"
  echo ""
  echo "Dev account 'dev0' created. Use 'energychaind keys export dev0 --unsafe --unarmored' to get the private key."
  echo ""
fi

echo "Starting energychaind..."
# JSON-RPC APIs: local single-node development still uses fullnode-style tx ingress.
JSON_RPC_API="${JSON_RPC_API:-eth,txpool,net,web3}"
$BINARY start \
  --pruning nothing \
  --log_level "$LOGLEVEL" \
  --minimum-gas-prices=10000000000${DENOM} \
  --evm.min-tip=0 \
  --home "$CHAINDIR" \
  --json-rpc.api "$JSON_RPC_API" \
  --chain-id "$CHAINID"
