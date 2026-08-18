#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — one-shot PRODUCTION deploy (run ON the Ubuntu host).
#
#   build -> init -> systemd(ec-node) -> firewall -> wait healthy ->
#   seed (9 modules) -> systemd(ec-loadgen) -> status
#
# Idempotent-ish: re-running rebuilds the binary and restarts services; it does
# NOT wipe chain data unless FRESH=1.
#
# sudo: systemd + ufw steps need root. If the invoking user lacks passwordless
# sudo, pass the password once via SUDO_PASS=... (kept only in this process).
# ===========================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

BIN="${BIN:-$CHAIN_DIR/energychaind}"
CHAINID="${CHAINID:-energychain_9001-1}"
DENOM="${DENOM:-uecy}"
CHAINDIR="${CHAINDIR:-$HOME/.energychaind}"
RUN_USER="${RUN_USER:-$(id -un)}"
RUN_HOME="$HOME"
DO_LOADGEN="${DO_LOADGEN:-1}"
DO_FIREWALL="${DO_FIREWALL:-1}"

# Key storage backend — required, no default (see init_node.sh for the
# rationale; test keyring additionally needs ALLOW_TEST_KEYRING=1).
KEYRING="${KEYRING:?ERROR: set KEYRING explicitly (file or os for production; test only with ALLOW_TEST_KEYRING=1)}"
export KEYRING

# ---- network exposure (fail-closed) ----------------------------------------
# Only p2p listens publicly by default. RPC / gRPC / EVM JSON-RPC bind to
# loopback: put them behind a reverse proxy + firewall allowlist before
# exposing. EXPOSE_PUBLIC=1 restores the old bind-everything demo behaviour.
EXPOSE_PUBLIC="${EXPOSE_PUBLIC:-0}"
if [ "$EXPOSE_PUBLIC" = "1" ]; then
  BIND_HOST="0.0.0.0"
else
  BIND_HOST="127.0.0.1"
fi
RPC_LADDR="${RPC_LADDR:-tcp://${BIND_HOST}:26657}"
P2P_LADDR="${P2P_LADDR:-tcp://0.0.0.0:26656}"
GRPC_ADDR="${GRPC_ADDR:-${BIND_HOST}:9090}"
JSONRPC_ADDR="${JSONRPC_ADDR:-${BIND_HOST}:8545}"
JSONRPC_WS_ADDR="${JSONRPC_WS_ADDR:-${BIND_HOST}:8546}"

# Pruning: "nothing" turns the validator into an ever-growing archive node;
# default to the standard pruning profile unless explicitly overridden.
PRUNING="${PRUNING:-default}"

run_sudo() {
  if [ "$(id -u)" = "0" ]; then "$@";
  elif [ -n "${SUDO_PASS:-}" ]; then echo "$SUDO_PASS" | sudo -S -p '' "$@";
  else sudo "$@"; fi
}

echo "============================================================"
echo "  EnergyChain production deploy"
echo "  user=$RUN_USER home=$RUN_HOME bin=$BIN chaindir=$CHAINDIR"
echo "============================================================"

# ---- 1. build + init ------------------------------------------------------
echo "[1/6] build + init node"
BUILD="${BUILD:-1}" FRESH="${FRESH:-0}" bash "${SCRIPT_DIR}/init_node.sh"

# ---- 2. systemd: ec-node --------------------------------------------------
echo "[2/6] installing systemd unit ec-node.service"
NODE_UNIT="/etc/systemd/system/ec-node.service"
run_sudo tee "$NODE_UNIT" >/dev/null <<UNIT
[Unit]
Description=EnergyChain validator node (${CHAINID})
After=network-online.target
Wants=network-online.target

[Service]
User=${RUN_USER}
Type=simple
Environment=HOME=${RUN_HOME}
# Anything in the binary that still resolves a path relative to the working
# directory (the SDK upgrade keeper does, when no home is configured) then
# lands inside the chain home instead of failing against /.
WorkingDirectory=${CHAINDIR}
ExecStart=${BIN} start --home ${CHAINDIR} --chain-id ${CHAINID} --pruning ${PRUNING} --log_level info --minimum-gas-prices=10000000000${DENOM} --evm.min-tip=0 --json-rpc.api eth,txpool,net,web3 --rpc.laddr ${RPC_LADDR} --p2p.laddr ${P2P_LADDR} --grpc.address ${GRPC_ADDR} --json-rpc.address ${JSONRPC_ADDR} --json-rpc.ws-address ${JSONRPC_WS_ADDR}
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
UNIT
run_sudo systemctl daemon-reload
run_sudo systemctl enable --now ec-node.service
echo "  ec-node started"

# ---- 3. firewall ----------------------------------------------------------
if [ "$DO_FIREWALL" = "1" ] && command -v ufw >/dev/null 2>&1; then
  echo "[3/6] firewall (ufw)"
  run_sudo ufw allow 22/tcp || true     # keep SSH open BEFORE enabling
  run_sudo ufw allow 26656/tcp || true  # p2p is the only public port by default
  if [ "$EXPOSE_PUBLIC" = "1" ]; then
    for p in 80 443 3000 3001 8080 8081 8090 8545 8546 1317 9090 26657; do
      run_sudo ufw allow "${p}/tcp" || true
    done
  fi
  run_sudo ufw --force enable || true
  echo "  ufw enabled (public: 22, 26656$([ "$EXPOSE_PUBLIC" = "1" ] && echo ', rpc/rest/evm — EXPOSE_PUBLIC=1'))"
else
  echo "[3/6] firewall: skipped"
fi

# ---- 4. wait for the node to produce blocks -------------------------------
echo "[4/6] waiting for node to become healthy"
ok=0
for _ in $(seq 1 60); do
  h=$(curl -s http://127.0.0.1:26657/status 2>/dev/null | jq -r '.result.sync_info.latest_block_height // "0"')
  if [ -n "$h" ] && [ "$h" != "0" ] && [ "$h" -ge 2 ] 2>/dev/null; then ok=1; echo "  height=$h"; break; fi
  sleep 2
done
[ "$ok" = "1" ] || { echo "ERROR: node did not produce blocks; journalctl -u ec-node"; exit 1; }

# ---- 5. seed 9 modules ----------------------------------------------------
# seed.sh / loadgen.sh sign txs non-interactively and therefore only work
# with the (plaintext) test keyring — never run them on a real production
# validator whose keys live in the file/os backend.
if [ "$KEYRING" = "test" ]; then
  echo "[5/6] seeding 9 modules + load pool"
  RESEED="${RESEED:-0}" bash "${SCRIPT_DIR}/seed.sh"
else
  echo "[5/6] seed: skipped (KEYRING=$KEYRING; demo seeding requires the test keyring)"
  DO_LOADGEN=0
fi

# ---- 6. systemd: ec-loadgen ----------------------------------------------
if [ "$DO_LOADGEN" = "1" ]; then
  echo "[6/6] installing systemd unit ec-loadgen.service"
  LG_UNIT="/etc/systemd/system/ec-loadgen.service"
  run_sudo tee "$LG_UNIT" >/dev/null <<UNIT
[Unit]
Description=EnergyChain on-chain load generator (~50 tx/block, 9 modules)
After=ec-node.service
Requires=ec-node.service

[Service]
User=${RUN_USER}
Type=simple
Environment=HOME=${RUN_HOME}
WorkingDirectory=${CHAIN_DIR}
ExecStart=/usr/bin/env bash ${SCRIPT_DIR}/loadgen.sh
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT
  run_sudo systemctl daemon-reload
  run_sudo systemctl enable --now ec-loadgen.service
  echo "  ec-loadgen started"
else
  echo "[6/6] loadgen: skipped (DO_LOADGEN=0)"
fi

echo ""
echo "============================================================"
echo "  DONE"
echo "  RPC   http://$(hostname -I | awk '{print $1}'):26657"
echo "  REST  http://$(hostname -I | awk '{print $1}'):1317"
echo "  gRPC  $(hostname -I | awk '{print $1}'):9090"
echo "  EVM   http://$(hostname -I | awk '{print $1}'):8545"
echo "  logs  journalctl -u ec-node -f   |   journalctl -u ec-loadgen -f"
echo "============================================================"
