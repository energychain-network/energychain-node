#!/bin/bash
set -e

echo "=============================================="
echo "  EnergyChain 生产节点一键部署脚本"
echo "  4 验证者 + Seed + 2 Sentry + FullNode"
echo "=============================================="
echo ""

# Cross-platform sed -i wrapper (BSD vs GNU)
sedi() {
  if [[ "$OSTYPE" == "darwin"* ]]; then
    sed -i '' "$@"
  else
    sed -i "$@"
  fi
}

# 停掉所有节点
if pgrep -f "energychaind start" > /dev/null 2>&1; then
  echo "Stopping existing nodes..."
  pkill -f "energychaind start" || true
  sleep 3
fi

# 变量
BINARY="energychaind"
BASE_HOME="$HOME/.energychain-production"
CHAIN_ID="energychain_9001-1"
DENOM="uecy"
KEYRING="test"
KEYALGO="eth_secp256k1"
IP="127.0.0.1"
NUM_VALIDATORS=4
TOTAL_NODES=8

NODES=("seed" "sentry-0" "sentry-1" "fullnode" "validator-0" "validator-1" "validator-2" "validator-3")
FULLNODE_RPC="tcp://${IP}:26687"
VALIDATOR_JSON_RPC_API="eth,net,web3"
FULLNODE_JSON_RPC_API="eth,txpool,net,web3"

command -v $BINARY >/dev/null 2>&1 || { echo "ERROR: $BINARY not found in PATH"; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "ERROR: jq not found. Install: brew install jq (Mac) / sudo apt install jq (Linux)"; exit 1; }

# 完全删除旧数据
echo "[1/9] Cleaning old data..."
rm -rf "$BASE_HOME"

# ====== 第 1 步: 初始化所有节点 ======
echo "[2/9] Initializing ${TOTAL_NODES} nodes..."
for node in "${NODES[@]}"; do
  $BINARY init "${node}" --chain-id "$CHAIN_ID" --home "${BASE_HOME}/${node}" > /dev/null 2>&1
  $BINARY config set client chain-id "$CHAIN_ID" --home "${BASE_HOME}/${node}"
  $BINARY config set client keyring-backend "$KEYRING" --home "${BASE_HOME}/${node}"
  echo "  -> ${node}"
done

# ====== 第 2 步: 创建验证者密钥 ======
echo ""
echo "[3/9] Creating ${NUM_VALIDATORS} validator keys..."
declare -a VAL_ADDRS
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
  $BINARY keys add "validator${i}" \
    --keyring-backend "$KEYRING" \
    --algo "$KEYALGO" \
    --home "${BASE_HOME}/validator-${i}" > /dev/null 2>&1

  ADDR=$($BINARY keys show "validator${i}" --keyring-backend "$KEYRING" --home "${BASE_HOME}/validator-${i}" --address)
  VAL_ADDRS+=("$ADDR")
  echo "  validator-${i}: ${ADDR}"
done

# 创建 dev0 测试账户
$BINARY keys add dev0 \
  --keyring-backend "$KEYRING" \
  --algo "$KEYALGO" \
  --home "${BASE_HOME}/validator-0" > /dev/null 2>&1
DEV0_ADDR=$($BINARY keys show dev0 --keyring-backend "$KEYRING" --home "${BASE_HOME}/validator-0" --address)
echo "  dev0: ${DEV0_ADDR}"

# ====== 第 3 步: 配置创世参数 ======
echo ""
echo "[4/9] Configuring genesis parameters..."
NODE0_HOME="${BASE_HOME}/validator-0"
GENESIS="${NODE0_HOME}/config/genesis.json"
TMP="${NODE0_HOME}/config/tmp_genesis.json"

jq --arg d "$DENOM" '.app_state.staking.params.bond_denom=$d' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.gov.params.min_deposit[0].denom=$d' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.gov.params.expedited_min_deposit[0].denom=$d' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.evm.params.evm_denom=$d' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.mint.params.mint_denom=$d' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"

jq '.app_state.bank.denom_metadata=[{
  "description":"Energy Chain native token",
  "denom_units":[
    {"denom":"uecy","exponent":0,"aliases":["microecy"]},
    {"denom":"ecy","exponent":18,"aliases":[]}
  ],
  "base":"uecy","display":"ecy",
  "name":"Energy Chain Yield","symbol":"ECY","uri":"","uri_hash":""
}]' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"

jq '.app_state.evm.params.active_static_precompiles=[
  "0x0000000000000000000000000000000000000100",
  "0x0000000000000000000000000000000000000400",
  "0x0000000000000000000000000000000000000800",
  "0x0000000000000000000000000000000000000801",
  "0x0000000000000000000000000000000000000802",
  "0x0000000000000000000000000000000000000803",
  "0x0000000000000000000000000000000000000804",
  "0x0000000000000000000000000000000000000805",
  "0x0000000000000000000000000000000000000806",
  "0x0000000000000000000000000000000000000807"
]' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"

jq '.app_state.erc20.native_precompiles=["0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"]' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"
jq --arg d "$DENOM" '.app_state.erc20.token_pairs=[{contract_owner:1,erc20_address:"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",denom:$d,enabled:true}]' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"

jq '.consensus.params.block.max_gas="60000000"' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"
jq '.app_state.feemarket.params.min_gas_price="10000000000.000000000000000000"' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"
jq '.app_state.feemarket.params.base_fee="10000000000.000000000000000000"' "$GENESIS" > "$TMP" && mv "$TMP" "$GENESIS"

echo "  Genesis params configured."

# ====== 第 4 步: 添加创世账户 ======
echo ""
echo "[5/9] Adding genesis accounts..."
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
  $BINARY genesis add-genesis-account "${VAL_ADDRS[$i]}" "100000000000000000000000000${DENOM}" \
    --home "$NODE0_HOME" --keyring-backend "$KEYRING"
  echo "  validator-${i}: 100M ECY"
done

$BINARY genesis add-genesis-account "$DEV0_ADDR" "100000000000000000000000${DENOM}" \
  --home "$NODE0_HOME" --keyring-backend "$KEYRING"
echo "  dev0: 100,000 ECY"

# ====== 第 5 步: 生成 gentx (带 gas-prices) ======
echo ""
echo "[6/9] Creating genesis transactions..."
STAKE="1000000000000000000000000${DENOM}"

for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
  NODE_HOME="${BASE_HOME}/validator-${i}"
  if [ "$i" -ne 0 ]; then
    cp "${NODE0_HOME}/config/genesis.json" "${NODE_HOME}/config/genesis.json"
  fi

  $BINARY genesis gentx "validator${i}" "$STAKE" \
    --chain-id "$CHAIN_ID" \
    --keyring-backend "$KEYRING" \
    --home "$NODE_HOME" \
    --moniker "validator-${i}" \
    --gas-prices "10000000000${DENOM}" \
    --commission-rate "0.10" \
    --commission-max-rate "0.20" \
    --commission-max-change-rate "0.01" \
    --min-self-delegation "1" > /dev/null 2>&1
  echo "  gentx: validator-${i}"
done

# 收集 gentx
for i in $(seq 1 $((NUM_VALIDATORS - 1))); do
  cp "${BASE_HOME}/validator-${i}/config/gentx/"*.json "${NODE0_HOME}/config/gentx/" 2>/dev/null || true
done

$BINARY genesis collect-gentxs --home "$NODE0_HOME" > /dev/null 2>&1
$BINARY genesis validate-genesis --home "$NODE0_HOME"
echo "  Genesis validated!"

# ====== 第 6 步: 分发创世文件 ======
for node in "${NODES[@]}"; do
  if [ "$node" != "validator-0" ]; then
    cp "${NODE0_HOME}/config/genesis.json" "${BASE_HOME}/${node}/config/genesis.json"
  fi
done
echo "  Genesis distributed to all ${TOTAL_NODES} nodes."

# ====== 第 7 步: 获取节点 ID 并配置网络 ======
echo ""
echo "[7/9] Configuring P2P network..."

# Get node IDs (compatible with Bash 3.x — no associative arrays)
ID_SEED=$($BINARY comet show-node-id --home "${BASE_HOME}/seed")
ID_SENTRY0=$($BINARY comet show-node-id --home "${BASE_HOME}/sentry-0")
ID_SENTRY1=$($BINARY comet show-node-id --home "${BASE_HOME}/sentry-1")
ID_FULLNODE=$($BINARY comet show-node-id --home "${BASE_HOME}/fullnode")
ID_VAL0=$($BINARY comet show-node-id --home "${BASE_HOME}/validator-0")
ID_VAL1=$($BINARY comet show-node-id --home "${BASE_HOME}/validator-1")
ID_VAL2=$($BINARY comet show-node-id --home "${BASE_HOME}/validator-2")
ID_VAL3=$($BINARY comet show-node-id --home "${BASE_HOME}/validator-3")

SEED_PEER="${ID_SEED}@${IP}:26656"

# --- Seed Node: P2P=26656 (seed only, no API/gRPC/EVM) ---
SEED_CFG="${BASE_HOME}/seed/config/config.toml"
SEED_APP="${BASE_HOME}/seed/config/app.toml"
sedi 's|seed_mode = false|seed_mode = true|' "$SEED_CFG"
sedi 's|allow_duplicate_ip = false|allow_duplicate_ip = true|' "$SEED_CFG"
sedi 's|addr_book_strict = true|addr_book_strict = false|' "$SEED_CFG"
sedi '/^\[mempool\]$/,/^\[/ s|^type = .*|type = "app"|' "$SEED_CFG"
sedi '/^\[mempool\]$/,/^\[/ s|^size = .*|size = 20000|' "$SEED_CFG"
sedi '/^\[evm.mempool\]$/,/^\[/ s|^operate-exclusively = .*|operate-exclusively = true|' "$SEED_APP"
sedi '/^\[grpc\]$/,/^\[/ s|enable = true|enable = false|' "$SEED_APP"
sedi '/^\[api\]$/,/^\[/ s|enable = false|enable = false|' "$SEED_APP"
echo "  seed: P2P=26656 (seed_mode)"

# --- Sentry-0: P2P=26666, 保护 validator-0 和 validator-1 ---
S0_CFG="${BASE_HOME}/sentry-0/config/config.toml"
S0_APP="${BASE_HOME}/sentry-0/config/app.toml"
sedi "s|laddr = \"tcp://0.0.0.0:26656\"|laddr = \"tcp://0.0.0.0:26666\"|" "$S0_CFG"
sedi "s|laddr = \"tcp://127.0.0.1:26657\"|laddr = \"tcp://0.0.0.0:26667\"|" "$S0_CFG"
sedi "s|pprof_laddr = \"localhost:6060\"|pprof_laddr = \"localhost:6061\"|" "$S0_CFG"
sedi "s|^seeds = .*|seeds = \"${SEED_PEER}\"|" "$S0_CFG"
sedi "s|^persistent_peers = .*|persistent_peers = \"${ID_VAL0}@${IP}:26696,${ID_VAL1}@${IP}:26706\"|" "$S0_CFG"
sedi "s|^private_peer_ids = .*|private_peer_ids = \"${ID_VAL0},${ID_VAL1}\"|" "$S0_CFG"
sedi 's|allow_duplicate_ip = false|allow_duplicate_ip = true|' "$S0_CFG"
sedi 's|addr_book_strict = true|addr_book_strict = false|' "$S0_CFG"
sedi '/^\[mempool\]$/,/^\[/ s|^type = .*|type = "app"|' "$S0_CFG"
sedi '/^\[mempool\]$/,/^\[/ s|^size = .*|size = 20000|' "$S0_CFG"
sedi '/^\[evm.mempool\]$/,/^\[/ s|^operate-exclusively = .*|operate-exclusively = true|' "$S0_APP"
sedi "s|address = \"tcp://localhost:1317\"|address = \"tcp://0.0.0.0:1318\"|" "$S0_APP"
sedi "s|address = \"localhost:9090\"|address = \"0.0.0.0:9190\"|" "$S0_APP"
sedi "s|address = \"127.0.0.1:8545\"|address = \"0.0.0.0:8555\"|" "$S0_APP"
sedi "s|ws-address = \"127.0.0.1:8546\"|ws-address = \"0.0.0.0:8556\"|" "$S0_APP"
sedi '/^\[api\]$/,/^\[/ s|enable = false|enable = true|' "$S0_APP"
sedi '/^\[json-rpc\]$/,/^\[/ s|enable = false|enable = true|' "$S0_APP"
echo "  sentry-0: P2P=26666 (protects val-0, val-1)"

# --- Sentry-1: P2P=26676, 保护 validator-2 和 validator-3 ---
S1_CFG="${BASE_HOME}/sentry-1/config/config.toml"
S1_APP="${BASE_HOME}/sentry-1/config/app.toml"
sedi "s|laddr = \"tcp://0.0.0.0:26656\"|laddr = \"tcp://0.0.0.0:26676\"|" "$S1_CFG"
sedi "s|laddr = \"tcp://127.0.0.1:26657\"|laddr = \"tcp://0.0.0.0:26677\"|" "$S1_CFG"
sedi "s|pprof_laddr = \"localhost:6060\"|pprof_laddr = \"localhost:6062\"|" "$S1_CFG"
sedi "s|^seeds = .*|seeds = \"${SEED_PEER}\"|" "$S1_CFG"
sedi "s|^persistent_peers = .*|persistent_peers = \"${ID_VAL2}@${IP}:26716,${ID_VAL3}@${IP}:26726\"|" "$S1_CFG"
sedi "s|^private_peer_ids = .*|private_peer_ids = \"${ID_VAL2},${ID_VAL3}\"|" "$S1_CFG"
sedi 's|allow_duplicate_ip = false|allow_duplicate_ip = true|' "$S1_CFG"
sedi 's|addr_book_strict = true|addr_book_strict = false|' "$S1_CFG"
sedi '/^\[mempool\]$/,/^\[/ s|^type = .*|type = "app"|' "$S1_CFG"
sedi '/^\[mempool\]$/,/^\[/ s|^size = .*|size = 20000|' "$S1_CFG"
sedi '/^\[evm.mempool\]$/,/^\[/ s|^operate-exclusively = .*|operate-exclusively = true|' "$S1_APP"
sedi "s|address = \"tcp://localhost:1317\"|address = \"tcp://0.0.0.0:1319\"|" "$S1_APP"
sedi "s|address = \"localhost:9090\"|address = \"0.0.0.0:9290\"|" "$S1_APP"
sedi "s|address = \"127.0.0.1:8545\"|address = \"0.0.0.0:8565\"|" "$S1_APP"
sedi "s|ws-address = \"127.0.0.1:8546\"|ws-address = \"0.0.0.0:8566\"|" "$S1_APP"
sedi '/^\[api\]$/,/^\[/ s|enable = false|enable = true|' "$S1_APP"
sedi '/^\[json-rpc\]$/,/^\[/ s|enable = false|enable = true|' "$S1_APP"
echo "  sentry-1: P2P=26676 (protects val-2, val-3)"

# --- Full Node: P2P=26686, 公共 RPC ---
FN_CFG="${BASE_HOME}/fullnode/config/config.toml"
FN_APP="${BASE_HOME}/fullnode/config/app.toml"
sedi "s|laddr = \"tcp://0.0.0.0:26656\"|laddr = \"tcp://0.0.0.0:26686\"|" "$FN_CFG"
sedi "s|laddr = \"tcp://127.0.0.1:26657\"|laddr = \"tcp://0.0.0.0:26687\"|" "$FN_CFG"
sedi "s|pprof_laddr = \"localhost:6060\"|pprof_laddr = \"localhost:6063\"|" "$FN_CFG"
sedi "s|^seeds = .*|seeds = \"${SEED_PEER}\"|" "$FN_CFG"
sedi "s|^persistent_peers = .*|persistent_peers = \"${ID_SENTRY0}@${IP}:26666,${ID_SENTRY1}@${IP}:26676\"|" "$FN_CFG"
sedi 's|allow_duplicate_ip = false|allow_duplicate_ip = true|' "$FN_CFG"
sedi 's|addr_book_strict = true|addr_book_strict = false|' "$FN_CFG"
sedi "s|cors_allowed_origins = \[\]|cors_allowed_origins = [\"*\"]|" "$FN_CFG"
sedi '/^\[mempool\]$/,/^\[/ s|^type = .*|type = "app"|' "$FN_CFG"
sedi '/^\[mempool\]$/,/^\[/ s|^size = .*|size = 20000|' "$FN_CFG"
sedi 's|prometheus = false|prometheus = true|' "$FN_CFG"
sedi '/^\[evm.mempool\]$/,/^\[/ s|^operate-exclusively = .*|operate-exclusively = true|' "$FN_APP"
sedi "s|address = \"tcp://localhost:1317\"|address = \"tcp://0.0.0.0:1320\"|" "$FN_APP"
sedi "s|address = \"localhost:9090\"|address = \"0.0.0.0:9390\"|" "$FN_APP"
sedi "s|address = \"127.0.0.1:8545\"|address = \"0.0.0.0:8575\"|" "$FN_APP"
sedi "s|ws-address = \"127.0.0.1:8546\"|ws-address = \"0.0.0.0:8576\"|" "$FN_APP"
sedi '/^\[api\]$/,/^\[/ s|enable = false|enable = true|' "$FN_APP"
sedi "s|enabled-unsafe-cors = false|enabled-unsafe-cors = true|" "$FN_APP"
sedi '/^\[json-rpc\]$/,/^\[/ s|enable = false|enable = true|' "$FN_APP"
sedi 's|enable-indexer = false|enable-indexer = true|' "$FN_APP"
# State Sync 快照: 每 1000 块生成一次, 保留最近 2 个, 供新节点快速同步
sedi 's|snapshot-interval = 0|snapshot-interval = 1000|' "$FN_APP"
sedi 's|snapshot-keep-recent = 2|snapshot-keep-recent = 2|' "$FN_APP"
echo "  fullnode: P2P=26686 RPC=26687 EVM=8575 (snapshot-interval=1000)"

# --- 4 个 Validators ---
#   validator-0: P2P=26696  -> sentry-0
#   validator-1: P2P=26706  -> sentry-0
#   validator-2: P2P=26716  -> sentry-1
#   validator-3: P2P=26726  -> sentry-1
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
  P2P_PORT=$((26696 + i * 10))
  RPC_PORT=$((26697 + i * 10))
  PPROF=$((6064 + i))
  EVM_PORT=$((8585 + i * 10))
  EVM_WS=$((8586 + i * 10))
  API_PORT=$((1321 + i))
  GRPC_PORT=$((9490 + i * 100))

  VC="${BASE_HOME}/validator-${i}/config/config.toml"
  VA="${BASE_HOME}/validator-${i}/config/app.toml"

  sedi "s|laddr = \"tcp://0.0.0.0:26656\"|laddr = \"tcp://0.0.0.0:${P2P_PORT}\"|" "$VC"
  sedi "s|laddr = \"tcp://127.0.0.1:26657\"|laddr = \"tcp://127.0.0.1:${RPC_PORT}\"|" "$VC"
  sedi "s|pprof_laddr = \"localhost:6060\"|pprof_laddr = \"localhost:${PPROF}\"|" "$VC"

  # validator-0,1 连 sentry-0; validator-2,3 连 sentry-1
  if [ "$i" -le 1 ]; then
    SENTRY="${ID_SENTRY0}@${IP}:26666"
  else
    SENTRY="${ID_SENTRY1}@${IP}:26676"
  fi
  sedi "s|^persistent_peers = .*|persistent_peers = \"${SENTRY}\"|" "$VC"
  sedi 's|pex = true|pex = false|' "$VC"
  sedi 's|allow_duplicate_ip = false|allow_duplicate_ip = true|' "$VC"
  sedi 's|addr_book_strict = true|addr_book_strict = false|' "$VC"
  sedi 's|prometheus = false|prometheus = true|' "$VC"
  sedi '/^\[mempool\]$/,/^\[/ s|^type = .*|type = "app"|' "$VC"
  sedi '/^\[mempool\]$/,/^\[/ s|^size = .*|size = 20000|' "$VC"
  sedi '/^\[evm.mempool\]$/,/^\[/ s|^operate-exclusively = .*|operate-exclusively = true|' "$VA"

  sedi "s|address = \"tcp://localhost:1317\"|address = \"tcp://127.0.0.1:${API_PORT}\"|" "$VA"
  sedi "s|address = \"localhost:9090\"|address = \"127.0.0.1:${GRPC_PORT}\"|" "$VA"
  sedi "s|address = \"127.0.0.1:8545\"|address = \"127.0.0.1:${EVM_PORT}\"|" "$VA"
  sedi "s|ws-address = \"127.0.0.1:8546\"|ws-address = \"127.0.0.1:${EVM_WS}\"|" "$VA"
  sedi '/^\[api\]$/,/^\[/ s|enable = false|enable = true|' "$VA"
  sedi '/^\[json-rpc\]$/,/^\[/ s|enable = false|enable = true|' "$VA"

  echo "  validator-${i}: P2P=${P2P_PORT} EVM=${EVM_PORT} (pex=off)"
done

# ====== 第 8 步: 设置 min gas prices ======
echo ""
echo "[8/9] Setting minimum gas prices..."
for node in "${NODES[@]}"; do
  APP_TOML="${BASE_HOME}/${node}/config/app.toml"
  sedi "s|^minimum-gas-prices = .*|minimum-gas-prices = \"10000000000${DENOM}\"|" "$APP_TOML"
done
$BINARY config set client node "$FULLNODE_RPC" --home "${BASE_HOME}/fullnode"
for node in seed sentry-0 sentry-1 validator-0 validator-1 validator-2 validator-3; do
  $BINARY config set client node "$FULLNODE_RPC" --home "${BASE_HOME}/${node}"
done
echo "  All nodes: 10 Gwei"

# ====== 第 9 步: 启动所有节点 ======
echo ""
echo "[9/9] Starting all ${TOTAL_NODES} nodes..."
LOG_DIR="${BASE_HOME}/logs"
mkdir -p "$LOG_DIR"

# Seed
nohup $BINARY start --home "${BASE_HOME}/seed" --chain-id "$CHAIN_ID" --minimum-gas-prices="10000000000${DENOM}" > "${LOG_DIR}/seed.log" 2>&1 &
echo "  seed        PID: $!"
sleep 3

# Sentries
for i in 0 1; do
  nohup $BINARY start --home "${BASE_HOME}/sentry-${i}" --chain-id "$CHAIN_ID" --minimum-gas-prices="10000000000${DENOM}" --json-rpc.api "$VALIDATOR_JSON_RPC_API" > "${LOG_DIR}/sentry-${i}.log" 2>&1 &
  echo "  sentry-${i}    PID: $!"
done
sleep 3

# Fullnode
nohup $BINARY start --home "${BASE_HOME}/fullnode" --chain-id "$CHAIN_ID" --minimum-gas-prices="10000000000${DENOM}" --json-rpc.api "$FULLNODE_JSON_RPC_API" --pruning nothing > "${LOG_DIR}/fullnode.log" 2>&1 &
echo "  fullnode    PID: $!"
sleep 3

# 4 Validators
for i in $(seq 0 $((NUM_VALIDATORS - 1))); do
  nohup $BINARY start --home "${BASE_HOME}/validator-${i}" --chain-id "$CHAIN_ID" --minimum-gas-prices="10000000000${DENOM}" --json-rpc.api "$VALIDATOR_JSON_RPC_API" > "${LOG_DIR}/validator-${i}.log" 2>&1 &
  echo "  validator-${i} PID: $!"
  sleep 2
done

# ====== 健康检查 ======
echo ""
echo "Waiting for nodes to start..."
sleep 10

RUNNING=$(ps aux | grep "energychaind start" | grep -v grep | wc -l)

echo ""
echo "=============================================="
echo "  EnergyChain Production Network"
echo "=============================================="
echo ""
echo "  Nodes running: ${RUNNING}/${TOTAL_NODES}"
echo ""
echo "  Chain ID:      ${CHAIN_ID}"
echo "  EVM Chain ID:  262144"
echo "  Denom:         ${DENOM} (uecy)"
echo ""
echo "  Architecture:"
echo "    Seed:        P2P=26656"
echo "    Sentry-0:    P2P=26666  (protects val-0, val-1)"
echo "    Sentry-1:    P2P=26676  (protects val-2, val-3)"
echo "    Fullnode:    P2P=26686  EVM=8575  REST=1320  RPC=26687"
echo "    Validator-0: P2P=26696  EVM=8585"
echo "    Validator-1: P2P=26706  EVM=8595"
echo "    Validator-2: P2P=26716  EVM=8605"
echo "    Validator-3: P2P=26726  EVM=8615"
echo ""
echo "  Full Node endpoints (public):"
echo "    CometBFT RPC: http://${IP}:26687"
echo "    EVM RPC:      http://${IP}:8575"
echo "    EVM WS:       ws://${IP}:8576"
echo "    REST API:     http://${IP}:1320"
echo "    gRPC:         ${IP}:9390"
echo "    CLI node:     ${FULLNODE_RPC}"
echo ""
echo "  Dev account:   ${DEV0_ADDR}"
echo "  Logs:          ${LOG_DIR}/"
echo ""

if [ "$RUNNING" -eq "$TOTAL_NODES" ]; then
  echo "  All ${TOTAL_NODES} nodes are running!"
else
  echo "  WARNING: Only ${RUNNING}/${TOTAL_NODES} nodes running."
  echo "  Check logs for errors:"
  for node in "${NODES[@]}"; do
    if ! ps aux | grep "energychaind start.*${node}" | grep -v grep > /dev/null 2>&1; then
      echo ""
      echo "  --- ${node} (NOT RUNNING) ---"
      tail -5 "${LOG_DIR}/${node}.log" 2>/dev/null || echo "  (no log)"
    fi
  done
fi

echo ""
echo "  ── EVM RPC 服务 ──"
echo "  Fullnode 提供 EVM JSON-RPC (自动重启机制已内置):"
echo "    EVM JSON-RPC : http://127.0.0.1:8575"
echo "    REST API     : http://127.0.0.1:1320"
echo "    CometBFT RPC : http://127.0.0.1:26687"
echo ""
echo "  Stop all:  pkill -f 'energychaind start'"
echo "  View logs: tail -f ${LOG_DIR}/fullnode.log"
echo "=============================================="
