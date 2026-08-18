#!/usr/bin/env bash
# Swap energychaind to energychaind.new with health-gated auto-rollback.
# Usage: PW=<sudo-pass> bash swap_chain.sh
set -uo pipefail
CHAIN_DIR="/home/oem/energychain/chain"
RPC="http://127.0.0.1:26657"
cd "$CHAIN_DIR"

S() { echo "${PW}" | sudo -S -p "" "$@"; }
height() { curl -s "$RPC/status" 2>/dev/null | jq -r '.result.sync_info.latest_block_height // 0' 2>/dev/null; }

[ -f energychaind.new ] || { echo "energychaind.new missing"; exit 1; }
TS="$(date +%Y%m%d-%H%M%S)"
cp -p energychaind "energychaind.bak.$TS"
echo "backup -> energychaind.bak.$TS  (pre-swap height=$(height))"

echo "stopping ec-loadgen + ec-node ..."
S systemctl stop ec-loadgen
S systemctl stop ec-node
sleep 2

cp energychaind.new energychaind
echo "binary replaced; starting ec-node ..."
S systemctl start ec-node

ok=0
h0="$(height)"
for i in $(seq 1 40); do
  sleep 3
  h="$(height)"
  if [ -n "$h" ] && [ "$h" != "0" ] && [ "$h" -gt "${h0:-0}" 2>/dev/null ]; then
    ok=1; break
  fi
  h0="${h:-$h0}"
done

if [ "$ok" != "1" ]; then
  echo "!!! new binary did not advance height -> ROLLBACK"
  S systemctl stop ec-node
  cp "energychaind.bak.$TS" energychaind
  S systemctl start ec-node
  sleep 5
  S systemctl start ec-loadgen
  echo "ROLLBACK_DONE height=$(height)"
  echo "--- last ec-node journal ---"
  S journalctl -u ec-node -n 40 --no-pager
  exit 1
fi

echo "new binary healthy at height=$h; starting ec-loadgen ..."
S systemctl start ec-loadgen
sleep 2
echo "SWAP_OK height=$(height)"
