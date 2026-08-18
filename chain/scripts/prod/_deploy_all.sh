#!/usr/bin/env bash
# ===========================================================================
# Primcast devnet — one-shot full-stack bringup, meant to run DETACHED.
#
#   build binary -> mnemonics -> chain (systemd) -> seed 9 modules ->
#   loadgen -> explorer stack -> dex stack -> verify
#
# Launch it as:
#   nohup setsid bash _deploy_all.sh > ~/primcast-deploy.log 2>&1 &
#
# so a dropped SSH control connection can never kill a 10-minute Go build.
# Progress is appended to the log and the current phase is mirrored into
# ~/primcast-deploy.phase for cheap polling over a flaky link.
#
# DEVNET ONLY: KEYRING=test keeps signing keys in PLAINTEXT and UNSAFE_CORS
# opens the RPC to every origin. Both are required for unattended seeding and
# browser access; neither is acceptable past this stage.
# ===========================================================================
set -uo pipefail

EC_HOME="$HOME/energychain"
CHAIN_SRC="$EC_HOME/chain"
BIN="$CHAIN_SRC/energychaind"
SECRETS="$HOME/.primcast_secrets"
PHASE_FILE="$HOME/primcast-deploy.phase"

PUBLIC_HOST="${PUBLIC_HOST:-54.90.254.198}"
CHAIN_HOST="$(hostname -I | awk '{print $1}')"
CHAINID="${CHAINID:-energychain_9001-1}"

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

phase() {
  echo "$1" > "$PHASE_FILE"
  echo ""
  echo "############################################################"
  echo "# [$(date -u '+%H:%M:%S')] $1"
  echo "############################################################"
}

die() { echo "FATAL: $*"; echo "FAILED: $*" > "$PHASE_FILE"; exit 1; }

phase "0/7 preflight"
command -v go >/dev/null || die "go missing"
command -v jq >/dev/null || die "jq missing"
docker ps >/dev/null 2>&1 || die "docker not usable by $USER (group not applied — reconnect ssh)"
[ -d "$CHAIN_SRC" ] || die "$CHAIN_SRC missing"
[ -d "$EC_HOME/explorer/deploy" ] || die "explorer repo missing"
[ -d "$EC_HOME/dex/deploy" ] || die "dex repo missing"
echo "  go=$(go version | awk '{print $3}') docker=$(docker --version | awk '{print $3}' | tr -d ,)"
echo "  chain_host=$CHAIN_HOST public_host=$PUBLIC_HOST chain_id=$CHAINID"

phase "1/7 build energychaind"
( cd "$CHAIN_SRC" && go build -o "$BIN" ./cmd/energychaind ) || die "go build failed"
echo "  built: $("$BIN" version 2>&1 | head -1) ($(du -h "$BIN" | cut -f1))"

phase "2/7 mnemonics"
# Generated on the host and never transmitted. Reused across re-runs so the
# validator and treasury addresses stay stable.
if [ ! -f "$SECRETS" ]; then
  umask 077
  {
    echo "VAL_MNEMONIC='$("$BIN" keys mnemonic 2>/dev/null)'"
    echo "DEV_MNEMONIC='$("$BIN" keys mnemonic 2>/dev/null)'"
  } > "$SECRETS"
  chmod 600 "$SECRETS"
  echo "  generated fresh mnemonics -> $SECRETS (0600)"
else
  echo "  reusing existing $SECRETS"
fi
# shellcheck disable=SC1090
source "$SECRETS"
[ -n "${VAL_MNEMONIC:-}" ] || die "VAL_MNEMONIC empty"
[ -n "${DEV_MNEMONIC:-}" ] || die "DEV_MNEMONIC empty"
echo "  validator words=$(echo "$VAL_MNEMONIC" | wc -w) treasury words=$(echo "$DEV_MNEMONIC" | wc -w)"

phase "3/7 chain init + systemd + seed + loadgen"
# DO_FIREWALL=0: ufw stays off on purpose. deploy.sh's allowlist does not
# include the alternate ssh port, so enabling it here could lock us out; the
# AWS security group is the only gate we want for a devnet.
cd "$CHAIN_SRC"
FRESH="${FRESH:-1}" \
BUILD=0 \
KEYRING=test \
ALLOW_TEST_KEYRING=1 \
VAL_MNEMONIC="$VAL_MNEMONIC" \
DEV_MNEMONIC="$DEV_MNEMONIC" \
CHAINID="$CHAINID" \
EXPOSE_PUBLIC=1 \
API_ADDR="tcp://0.0.0.0:1317" \
UNSAFE_CORS=1 \
PRUNING=default \
DO_LOADGEN=1 \
DO_FIREWALL=0 \
RESEED="${RESEED:-0}" \
bash scripts/prod/deploy.sh || die "deploy.sh failed"

phase "4/7 chain health"
for _ in $(seq 1 30); do
  h=$(curl -s "http://127.0.0.1:26657/status" | jq -r '.result.sync_info.latest_block_height // "0"')
  [ "${h:-0}" -ge 3 ] 2>/dev/null && { echo "  height=$h"; break; }
  sleep 2
done
echo "  rest  : $(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:1317/cosmos/base/tendermint/v1beta1/node_info)"
echo "  evm   : $(curl -s -X POST -H 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
  http://127.0.0.1:8545 | jq -r '.result // "ERR"')"

phase "5/7 explorer stack"
CHAIN_HOST="$CHAIN_HOST" PUBLIC_HOST="$PUBLIC_HOST" \
  bash "$CHAIN_SRC/scripts/prod/apps.sh" explorer || die "explorer deploy failed"

phase "6/7 dex stack"
CHAIN_HOST="$CHAIN_HOST" PUBLIC_HOST="$PUBLIC_HOST" \
  bash "$CHAIN_SRC/scripts/prod/apps.sh" dex || die "dex deploy failed"

phase "7/7 verify"
echo "--- systemd ---"
systemctl is-active ec-node ec-loadgen 2>/dev/null
echo "--- containers ---"
docker ps --format '{{.Names}}\t{{.Status}}\t{{.Ports}}'
echo "--- chain height ---"
curl -s http://127.0.0.1:26657/status | jq -r '.result.sync_info.latest_block_height'
echo "--- endpoint probes ---"
for u in "explorer-web http://127.0.0.1:3000" "explorer-api http://127.0.0.1:8080/health" \
         "dex-web http://127.0.0.1:3001" "dex-api http://127.0.0.1:8081/health"; do
  n="${u%% *}"; a="${u##* }"
  printf "  %-13s %s\n" "$n" "$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 "$a")"
done

echo "DONE" > "$PHASE_FILE"
echo ""
echo "============================================================"
echo "  PRIMCAST DEVNET UP"
echo "  explorer  http://${PUBLIC_HOST}:3000"
echo "  dex       http://${PUBLIC_HOST}:3001"
echo "  rpc       http://${PUBLIC_HOST}:26657"
echo "  evm       http://${PUBLIC_HOST}:8545"
echo "============================================================"
