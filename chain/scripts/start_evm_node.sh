#!/bin/bash
set -e

# EVM RPC 独立节点
# 与 deploy_production.sh 的多节点网络共存，提供稳定的 EVM JSON-RPC 服务
# 使用独立数据目录和非冲突端口

BINARY="energychaind"
CHAINID="energychain_9001-1"
MONIKER="evm-rpc-node"
KEYRING="test"
KEYALGO="eth_secp256k1"
DENOM="uecy"
CHAINDIR="$HOME/.energychain-evmnode"
JSON_RPC_API="${JSON_RPC_API:-eth,txpool,net,web3}"

# 端口配置 (不与生产节点和默认端口冲突)
P2P_PORT=26756
RPC_PORT=26757
EVM_HTTP=8545
EVM_WS=8546
API_PORT=1317
GRPC_PORT=9090
PPROF_PORT=6070

command -v $BINARY >/dev/null 2>&1 || { echo "ERROR: $BINARY not found in PATH"; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "ERROR: jq not found"; exit 1; }

# Cross-platform sed
sedi() {
  if [[ "$OSTYPE" == "darwin"* ]]; then
    sed -i '' "$@"
  else
    sed -i "$@"
  fi
}

VAL_MNEMONIC="${VAL_MNEMONIC:-gesture inject test cycle original hollow east ridge hen combine junk child bacon zero hope comfort vacuum milk pitch cage oppose unhappy lunar seat}"
DEV_MNEMONIC="${DEV_MNEMONIC:-copper push brief egg scan entry inform record adjust fossil boss egg comic alien upon aspect dry avoid interest fury window hint race symptom}"

overwrite=""
while [[ $# -gt 0 ]]; do
  case $1 in
    -y) overwrite="y"; shift ;;
    -n) overwrite="n"; shift ;;
    *) shift ;;
  esac
done

if [[ -z "$overwrite" && -d "$CHAINDIR" ]]; then
  echo "Found existing EVM node data at $CHAINDIR"
  echo "Overwrite? [y/n]"
  read -r overwrite
fi
[[ -z "$overwrite" ]] && overwrite="y"

if [[ "$overwrite" == "y" || "$overwrite" == "Y" ]]; then
  rm -rf "$CHAINDIR"

  $BINARY config set client chain-id "$CHAINID" --home "$CHAINDIR"
  $BINARY config set client keyring-backend "$KEYRING" --home "$CHAINDIR"
  $BINARY config set client node "tcp://127.0.0.1:${RPC_PORT}" --home "$CHAINDIR"

  echo "$VAL_MNEMONIC" | $BINARY keys add validator --recover --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$CHAINDIR"
  echo "$DEV_MNEMONIC" | $BINARY keys add dev0 --recover --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$CHAINDIR"

  echo "$VAL_MNEMONIC" | $BINARY init "$MONIKER" -o --chain-id "$CHAINID" --home "$CHAINDIR" --recover

  CONFIG_TOML=$CHAINDIR/config/config.toml
  APP_TOML=$CHAINDIR/config/app.toml
  GENESIS=$CHAINDIR/config/genesis.json
  TMP_GENESIS=$CHAINDIR/config/tmp_genesis.json

  jq --arg d "$DENOM" '.app_state.staking.params.bond_denom=$d' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg d "$DENOM" '.app_state.gov.params.min_deposit[0].denom=$d' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg d "$DENOM" '.app_state.gov.params.expedited_min_deposit[0].denom=$d' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg d "$DENOM" '.app_state.evm.params.evm_denom=$d' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg d "$DENOM" '.app_state.mint.params.mint_denom=$d' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  jq '.app_state.bank.denom_metadata=[{"description":"Energy Chain native token","denom_units":[{"denom":"uecy","exponent":0,"aliases":["microecy"]},{"denom":"ecy","exponent":18,"aliases":[]}],"base":"uecy","display":"ecy","name":"Energy Chain Yield","symbol":"ECY","uri":"","uri_hash":""}]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  jq '.app_state.evm.params.active_static_precompiles=["0x0000000000000000000000000000000000000100","0x0000000000000000000000000000000000000400","0x0000000000000000000000000000000000000800","0x0000000000000000000000000000000000000801","0x0000000000000000000000000000000000000802","0x0000000000000000000000000000000000000803","0x0000000000000000000000000000000000000804","0x0000000000000000000000000000000000000805","0x0000000000000000000000000000000000000806","0x0000000000000000000000000000000000000807"]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  jq '.app_state.erc20.native_precompiles=["0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq --arg d "$DENOM" '.app_state.erc20.token_pairs=[{contract_owner:1,erc20_address:"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",denom:$d,enabled:true}]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  jq '.consensus.params.block.max_gas="60000000"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq '.app_state.feemarket.params.min_gas_price="10000000000.000000000000000000"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  jq '.app_state.feemarket.params.base_fee="10000000000.000000000000000000"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

  sedi 's/"max_deposit_period": "172800s"/"max_deposit_period": "30s"/g' "$GENESIS"
  sedi 's/"voting_period": "172800s"/"voting_period": "30s"/g' "$GENESIS"
  sedi 's/"expedited_voting_period": "86400s"/"expedited_voting_period": "15s"/g' "$GENESIS"

  $BINARY genesis add-genesis-account validator 100000000000000000000000000${DENOM} --keyring-backend "$KEYRING" --home "$CHAINDIR"
  $BINARY genesis add-genesis-account dev0 1000000000000000000000000${DENOM} --keyring-backend "$KEYRING" --home "$CHAINDIR"

  # Configure ports
  sedi "s|laddr = \"tcp://0.0.0.0:26656\"|laddr = \"tcp://0.0.0.0:${P2P_PORT}\"|" "$CONFIG_TOML"
  sedi "s|laddr = \"tcp://127.0.0.1:26657\"|laddr = \"tcp://0.0.0.0:${RPC_PORT}\"|" "$CONFIG_TOML"
  sedi "s|pprof_laddr = \"localhost:6060\"|pprof_laddr = \"localhost:${PPROF_PORT}\"|" "$CONFIG_TOML"

  sedi 's/timeout_propose = "3s"/timeout_propose = "2s"/g' "$CONFIG_TOML"
  sedi 's/timeout_commit = "5s"/timeout_commit = "2s"/g' "$CONFIG_TOML"
  sedi '/^\[mempool\]$/,/^\[/ s|^type = .*|type = "app"|' "$CONFIG_TOML"
  sedi '/^\[mempool\]$/,/^\[/ s|^size = .*|size = 20000|' "$CONFIG_TOML"
  sedi 's/prometheus = false/prometheus = true/' "$CONFIG_TOML"
  sedi "s|cors_allowed_origins = \[\]|cors_allowed_origins = [\"*\"]|g" "$CONFIG_TOML"

  sedi "s|address = \"tcp://localhost:1317\"|address = \"tcp://0.0.0.0:${API_PORT}\"|" "$APP_TOML"
  sedi "s|address = \"0.0.0.0:9090\"|address = \"0.0.0.0:${GRPC_PORT}\"|" "$APP_TOML"
  sedi "s|address = \"127.0.0.1:8545\"|address = \"0.0.0.0:${EVM_HTTP}\"|" "$APP_TOML"
  sedi "s|ws-address = \"127.0.0.1:8546\"|ws-address = \"0.0.0.0:${EVM_WS}\"|" "$APP_TOML"

  sedi 's/enable = false/enable = true/g' "$APP_TOML"
  sedi 's/enabled-unsafe-cors = false/enabled-unsafe-cors = true/g' "$APP_TOML"
  sedi 's/enable-indexer = false/enable-indexer = true/g' "$APP_TOML"
  sedi '/^\[evm.mempool\]$/,/^\[/ s|^operate-exclusively = .*|operate-exclusively = true|' "$APP_TOML"

  $BINARY genesis gentx validator 1000000000000000000000${DENOM} --gas-prices 10000000000${DENOM} --keyring-backend "$KEYRING" --chain-id "$CHAINID" --home "$CHAINDIR"
  $BINARY genesis collect-gentxs --home "$CHAINDIR"
  $BINARY genesis validate-genesis --home "$CHAINDIR"

  echo ""
  echo "=== EVM RPC Node initialized ==="
  echo "Data:    $CHAINDIR"
  echo "EVM:     http://0.0.0.0:${EVM_HTTP}"
  echo "RPC:     http://0.0.0.0:${RPC_PORT}"
  echo "API:     http://0.0.0.0:${API_PORT}"
  echo ""
fi

echo "Starting EVM RPC node..."
nohup $BINARY start \
  --pruning nothing \
  --log_level info \
  --minimum-gas-prices=10000000000${DENOM} \
  --home "$CHAINDIR" \
  --json-rpc.api "$JSON_RPC_API" \
  --chain-id "$CHAINID" \
  > "$CHAINDIR/node.log" 2>&1 &

NODE_PID=$!
echo "  PID: $NODE_PID"
echo "  Log: $CHAINDIR/node.log"
echo ""

sleep 8

if kill -0 $NODE_PID 2>/dev/null; then
  EVM_RESULT=$(curl -s -X POST http://127.0.0.1:${EVM_HTTP} -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' 2>/dev/null || echo "FAILED")
  echo "=============================================="
  echo "  EVM RPC Node Running"
  echo "=============================================="
  echo ""
  echo "  EVM JSON-RPC : http://127.0.0.1:${EVM_HTTP}"
  echo "  EVM WebSocket: ws://127.0.0.1:${EVM_WS}"
  echo "  CometBFT RPC : http://127.0.0.1:${RPC_PORT}"
  echo "  REST API     : http://127.0.0.1:${API_PORT}"
  echo "  gRPC         : 127.0.0.1:${GRPC_PORT}"
  echo ""
  echo "  eth_blockNumber: $EVM_RESULT"
  echo ""
  echo "  Data dir: $CHAINDIR"
  echo "  Key home: $CHAINDIR (validator, dev0)"
  echo ""
  echo "  Stop: kill $NODE_PID"
  echo "  Logs: tail -f $CHAINDIR/node.log"
  echo "=============================================="
else
  echo "ERROR: Node failed to start. Check logs:"
  tail -20 "$CHAINDIR/node.log"
fi
