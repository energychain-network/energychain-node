#!/usr/bin/env bash
# ===========================================================================
# Primcast devnet — full bring-up from GitHub. Runs ON the host, driven over
# SSM (see _ecssm.sh); nothing here needs an inbound ssh session.
#
#   repos -> toolchain -> chain(+seed+loadgen) -> dex contracts -> apps -> swapbot
#
# Each phase writes ~/devnet.phase so progress survives a dropped control
# channel, and each is individually idempotent: re-running resumes rather than
# rebuilding from scratch. PHASES=chain,apps runs a subset.
#
# The chain is a throwaway devnet: keys live in the plaintext test keyring so
# seed.sh / loadgen.sh can sign non-interactively.
# ===========================================================================
set -uo pipefail

BRANCH="${BRANCH:-devnet/primcast-deploy}"
ORG="${ORG:-https://github.com/energychain-network}"
SRC="${SRC:-$HOME/src}"
PUBLIC_HOST="${PUBLIC_HOST:?set PUBLIC_HOST (browser-facing address)}"
CHAIN_HOST="${CHAIN_HOST:-$(hostname -I | awk '{print $1}')}"
FRESH="${FRESH:-0}"
PHASES="${PHASES:-repos,toolchain,chain,contracts,apps,swapbot}"

CHAIN_REPO="$SRC/energychain-node"
DEX_REPO="$SRC/energychain-dex"
EXP_REPO="$SRC/energychain-explorer"
CON_REPO="$SRC/energychain-contracts"
PROD="$CHAIN_REPO/chain/scripts/prod"
SECRETS="$HOME/.primcast/mnemonics.env"

# The feemarket floor is 10 gwei (init_node.sh patches genesis); anything below
# it is accepted into the mempool but never mined.
export GAS_PRICE_WEI="${GAS_PRICE_WEI:-20000000000}"

phase() { echo "$1" > "$HOME/devnet.phase"; printf '\n=========== %s ===========\n' "$1"; }
die()   { echo "FATAL: $*"; echo "FAILED: $*" > "$HOME/devnet.phase"; exit 1; }
want()  { [[ ",$PHASES," == *",$1,"* ]]; }

# --------------------------------------------------------------------------
want repos && {
phase "repos ($BRANCH)"
mkdir -p "$SRC"
for r in energychain-node energychain-dex energychain-explorer energychain-contracts; do
  if [ -d "$SRC/$r/.git" ]; then
    echo "--- $r: fetch ---"
    git -C "$SRC/$r" fetch --depth=1 origin "$BRANCH" -q \
      && git -C "$SRC/$r" checkout -q -B "$BRANCH" FETCH_HEAD \
      || die "git fetch $r"
  else
    echo "--- $r: clone ---"
    rm -rf "$SRC/$r"
    git clone --depth=1 -b "$BRANCH" -q "$ORG/$r.git" "$SRC/$r" || die "git clone $r"
  fi
  echo "    $(git -C "$SRC/$r" log --oneline -1)"
done
}

# --------------------------------------------------------------------------
want toolchain && {
phase "toolchain"
export DEBIAN_FRONTEND=noninteractive
need_apt=()
command -v jq   >/dev/null || need_apt+=(jq)
command -v curl >/dev/null || need_apt+=(curl)
command -v git  >/dev/null || need_apt+=(git)
command -v gcc  >/dev/null || need_apt+=(build-essential)
if [ "${#need_apt[@]}" -gt 0 ]; then
  sudo apt-get update -qq && sudo apt-get install -y -qq "${need_apt[@]}" || die "apt install"
fi

command -v go >/dev/null || export PATH="$PATH:/usr/local/go/bin"
command -v go >/dev/null || die "go missing — run _bootstrap_host.sh first"
echo "go:     $(go version)"

# Hardhat needs Node >= 18. Prefer the distro package; fall back to NodeSource
# on releases that ship something older.
if ! command -v node >/dev/null || [ "$(node -p 'process.versions.node.split(".")[0]')" -lt 18 ]; then
  sudo apt-get install -y -qq nodejs npm >/dev/null 2>&1 || true
  if ! command -v node >/dev/null || [ "$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)" -lt 18 ]; then
    curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash - >/dev/null 2>&1 \
      && sudo apt-get install -y -qq nodejs || die "node install"
  fi
fi
echo "node:   $(node -v)   npm: $(npm -v)"
docker ps >/dev/null 2>&1 || sudo usermod -aG docker "$(id -un)" || true
echo "docker: $(docker --version)"
}

# --------------------------------------------------------------------------
want chain && {
phase "chain"
# Mnemonics are generated once and kept out of the repo. They are the validator
# and treasury keys; regenerating them would orphan any existing chain state.
# Validate rather than just test for the file: `keys mnemonic` failing inside a
# command substitution still leaves an empty but present secrets file behind,
# and the resulting error surfaces much later inside init_node.sh.
mnemonics_ok() {
  [ -f "$SECRETS" ] || return 1
  # shellcheck disable=SC1090
  . "$SECRETS" 2>/dev/null || return 1
  [ "$(echo "${VAL_MNEMONIC:-}" | wc -w)" -ge 12 ] && [ "$(echo "${DEV_MNEMONIC:-}" | wc -w)" -ge 12 ]
}
if ! mnemonics_ok; then
  echo "--- generating validator/treasury mnemonics ---"
  rm -f "$SECRETS"
  mkdir -p "$(dirname "$SECRETS")"; chmod 700 "$(dirname "$SECRETS")"
  ( cd "$CHAIN_REPO/chain" && go build -o energychaind ./cmd/energychaind ) || die "go build (for keygen)"
  val="$("$CHAIN_REPO/chain/energychaind" keys mnemonic)" || die "keys mnemonic"
  dev="$("$CHAIN_REPO/chain/energychaind" keys mnemonic)" || die "keys mnemonic"
  printf "VAL_MNEMONIC='%s'\nDEV_MNEMONIC='%s'\n" "$val" "$dev" > "$SECRETS"
  chmod 600 "$SECRETS"
  mnemonics_ok || die "generated mnemonics look wrong (is the binary healthy? try: energychaind keys mnemonic)"
fi

sudo systemctl stop ec-loadgen ec-swapbot 2>/dev/null
sudo systemctl stop ec-node 2>/dev/null
pkill -f "energychaind start" 2>/dev/null; sleep 2

KEYRING=test ALLOW_TEST_KEYRING=1 \
VAL_MNEMONIC="$VAL_MNEMONIC" DEV_MNEMONIC="$DEV_MNEMONIC" \
EXPOSE_PUBLIC=1 API_ADDR=tcp://0.0.0.0:1317 UNSAFE_CORS=1 \
DO_FIREWALL=0 FRESH="$FRESH" \
  bash "$PROD/deploy.sh" || die "deploy.sh"

curl -s http://127.0.0.1:26657/status | jq -r '"height=\(.result.sync_info.latest_block_height) chain=\(.result.node_info.network)"'
}

# --------------------------------------------------------------------------
want contracts && {
phase "contracts"
BIN="$CHAIN_REPO/chain/energychaind"
CHAINDIR="$HOME/.energychaind"
# dev0 is an eth_secp256k1 key, so the same account works as an EVM signer.
DEV_KEY="$(yes | "$BIN" keys unsafe-export-eth-key dev0 \
             --keyring-backend test --home "$CHAINDIR" 2>/dev/null | tr -d '[:space:]')"
[ ${#DEV_KEY} -eq 64 ] || die "could not export dev0 eth key (got ${#DEV_KEY} chars)"

cd "$CON_REPO" || die "no contracts repo"
[ -d node_modules ] || npm ci --no-audit --no-fund >/dev/null 2>&1 || npm install --no-audit --no-fund || die "npm install"
export RPC_URL="http://127.0.0.1:8545"
export PRIVATE_KEY="0x$DEV_KEY"

npx hardhat compile 2>&1 | tail -3
if [ -f dex-deployment.json ] && [ "$FRESH" != "1" ]; then
  echo "--- dex-deployment.json exists; skipping deploy (FRESH=1 to redeploy) ---"
else
  rm -f dex-deployment.json
  npx hardhat run scripts/deploy_dex.ts --network energychain_local 2>&1 | tail -20 || die "deploy_dex"
fi
[ -f dex-deployment.json ] || die "deploy_dex produced no dex-deployment.json"
jq -r '.contracts | to_entries[] | "  \(.key)=\(.value)"' dex-deployment.json

# Pools + an initial candle history. Without at least one Swap the ohlcv table
# stays empty and the chart pages render blank.
npx hardhat run scripts/seed_dex.ts --network energychain_local 2>&1 | tail -20 || die "seed_dex"
}

# --------------------------------------------------------------------------
want apps && {
phase "apps"
CHAIN_HOST="$CHAIN_HOST" PUBLIC_HOST="$PUBLIC_HOST" \
CHAIN_DIR="$CHAIN_REPO/chain" DEX_DIR="$DEX_REPO" EXP_DIR="$EXP_REPO" \
DEX_DEPLOYMENT_JSON="$CON_REPO/dex-deployment.json" \
  bash "$PROD/apps.sh" all || die "apps.sh"
}

# --------------------------------------------------------------------------
want swapbot && {
phase "swapbot"
# Candles only advance while swaps keep arriving, so the bot is a service
# rather than a one-shot seeding step.
BIN="$CHAIN_REPO/chain/energychaind"
DEV_KEY="$(yes | "$BIN" keys unsafe-export-eth-key dev0 \
             --keyring-backend test --home "$HOME/.energychaind" 2>/dev/null | tr -d '[:space:]')"
sudo tee /etc/systemd/system/ec-swapbot.service >/dev/null <<UNIT
[Unit]
Description=Primcast DEX swap bot (feeds OHLCV candles)
After=ec-node.service
Requires=ec-node.service

[Service]
User=$(id -un)
Type=simple
WorkingDirectory=${CON_REPO}
Environment=HOME=${HOME}
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin
Environment=RPC_URL=http://127.0.0.1:8545
Environment=PRIVATE_KEY=0x${DEV_KEY}
Environment=GAS_PRICE_WEI=${GAS_PRICE_WEI}
Environment=BOT_INTERVAL_MS=${BOT_INTERVAL_MS:-4000}
ExecStart=/usr/bin/npx hardhat run scripts/swap_bot.ts --network energychain_local
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
UNIT
sudo systemctl daemon-reload
sudo systemctl enable --now ec-swapbot.service || die "ec-swapbot"
sleep 5
systemctl is-active ec-swapbot
}

phase "DONE"
echo ""
echo "chain   rpc=http://${PUBLIC_HOST}:26657  rest=http://${PUBLIC_HOST}:1317  evm=http://${PUBLIC_HOST}:8545"
echo "explorer http://${PUBLIC_HOST}:3000   api :8080"
echo "dex      http://${PUBLIC_HOST}:3001   api :8081"
