#!/usr/bin/env bash
# ===========================================================================
# EnergyChain — deploy the DEX and Explorer Docker stacks (run ON the host).
#
# Expects the two repos already synced to:
#   ~/energychain/dex        (energychain-dex)
#   ~/energychain/explorer   (energychain-explorer)
#
# Both indexers point at the chain over the host LAN IP (the node binds
# 0.0.0.0). The DEX Cosmos layer indexes x/market / x/stableusd / x/rwatoken /
# x/mincast; the Explorer indexes consensus + EVM + the custom modules.
#
# The DEX EVM layer (Uniswap-V2 pools, /pools and /charts, OHLCV candles built
# from Swap logs) only works once the contracts from energychain-contracts are
# on chain. Point DEX_DEPLOYMENT_JSON at the dex-deployment.json that Hardhat
# writes and this script enables EVM indexing and wires the addresses through;
# without it the EVM layer stays off and only the Cosmos pages have data.
#
# Ports (host):
#   explorer web 3000   explorer api 8080
#   dex web      3001   dex api      8081   dex nginx 8090 (remapped off 8080)
# ===========================================================================
set -euo pipefail

CHAIN_HOST="${CHAIN_HOST:-$(hostname -I | awk '{print $1}')}"
# Browser-facing host for NEXT_PUBLIC_*. Must differ from CHAIN_HOST on any
# cloud VM: the indexers dial the chain from inside the VPC (private address —
# AWS does not hairpin an instance's own public IP), while the bundled JS runs
# in the visitor's browser and needs the routable public address.
PUBLIC_HOST="${PUBLIC_HOST:-$CHAIN_HOST}"
# The explorer indexer image is built here; from outside China the default
# module proxy is far faster than the goproxy.cn the repo Dockerfile assumes.
GOPROXY_ENV="${GOPROXY_ENV:-https://proxy.golang.org,direct}"
DEX_DIR="${DEX_DIR:-$HOME/energychain/dex}"
EXP_DIR="${EXP_DIR:-$HOME/energychain/explorer}"
CHAIN_DIR="${CHAIN_DIR:-$HOME/energychain/chain}"
DEX_DEPLOYMENT_JSON="${DEX_DEPLOYMENT_JSON:-}"
WHICH="${1:-all}"   # all | dex | explorer

# Both repos keep their compose under deploy/, so the default project name
# collides ("deploy"). Pin explicit project names so the two stacks stay
# isolated (separate networks, volumes, container names).
EXP_PROJECT="ec-explorer"
DEX_PROJECT="ec-dex"

echo "CHAIN_HOST=$CHAIN_HOST  PUBLIC_HOST=$PUBLIC_HOST  dex=$DEX_DIR  explorer=$EXP_DIR  target=$WHICH"

# The explorer indexer's go.mod has a local replace `energychain => ../../
# energychain-node/chain`, which resolves to /energychain-node/chain inside the
# build container. The repo Dockerfile never copies the chain, so we build the
# image from a custom context that includes the chain source. The compose
# `up` (without --build) then reuses this pre-built image.
build_explorer_indexer() {
  echo "--- building explorer-indexer (custom context with chain source) ---"
  local ctx="$HOME/.ec_explorer_indexer_ctx"
  rm -rf "$ctx"; mkdir -p "$ctx"
  cp -al "$CHAIN_DIR" "$ctx/chain" 2>/dev/null || cp -a "$CHAIN_DIR" "$ctx/chain"
  cp -al "$EXP_DIR/indexer" "$ctx/indexer" 2>/dev/null || cp -a "$EXP_DIR/indexer" "$ctx/indexer"
  printf '%s\n' "chain/energychaind" "chain/build" "**/.git" "**/node_modules" > "$ctx/.dockerignore"
  cat > "$ctx/Dockerfile" <<DOCKER
FROM golang:1.25-alpine AS build
ENV GOPROXY=${GOPROXY_ENV}
ENV GOSUMDB=off
COPY chain /energychain-node/chain
WORKDIR /src
COPY indexer/go.mod indexer/go.sum* ./
RUN go mod download
COPY indexer/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/indexer ./cmd/indexer

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/indexer /usr/local/bin/indexer
COPY indexer/migrations /opt/explorer/migrations
ENV INDEXER_MIGRATIONS_DIR=/opt/explorer/migrations
EXPOSE 7070
ENTRYPOINT ["/usr/local/bin/indexer"]
DOCKER
  docker build -t energychain/explorer-indexer:dev "$ctx"
}

deploy_explorer() {
  echo "=== explorer ==="
  [ -d "$EXP_DIR/deploy" ] || { echo "missing $EXP_DIR/deploy"; return 1; }
  cat > "$EXP_DIR/.env" <<EOF
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_DB=energychain_explorer
POSTGRES_USER=explorer
POSTGRES_PASSWORD=explorer
DATABASE_URL=postgres://explorer:explorer@postgres:5432/energychain_explorer?sslmode=disable
DATABASE_URL_READONLY=postgres://explorer:explorer@postgres:5432/energychain_explorer?sslmode=disable
REDIS_ADDR=redis:6379
REDIS_URL=redis://redis:6379/0
REDIS_DB=0
COSMOS_RPC_URL=http://${CHAIN_HOST}:26657
COSMOS_REST_URL=http://${CHAIN_HOST}:1317
COSMOS_GRPC_URL=${CHAIN_HOST}:9090
COSMOS_GRPC_TLS=false
EVM_HTTP_URL=http://${CHAIN_HOST}:8545
EVM_WS_URL=ws://${CHAIN_HOST}:8546
CHAIN_ID=energychain_9001-1
EVM_CHAIN_ID=9001
BECH32_PREFIX=energy
NATIVE_DENOM=uecy
NATIVE_DECIMALS=18
DISPLAY_DENOM=ECY
INDEXER_START_HEIGHT=1
INDEXER_BATCH_SIZE=20
INDEXER_REORG_DEPTH=100
INDEXER_TRACE_ENABLED=true
INDEXER_METRICS_ADDR=:7070
API_HTTP_ADDR=:8080
API_WS_PATH=/ws
API_RATE_LIMIT_RPS=50
API_CACHE_TTL_SECONDS=2
API_CORS_ALLOWED_ORIGINS=*
API_METRICS_ADDR=:7071
EOF
  # web NEXT_PUBLIC_* point the browser at the public host (override the
  # localhost defaults baked into the base compose). postgres/redis host ports
  # are remapped off 5432/6379 because the host already runs those services.
  cat > "$EXP_DIR/deploy/docker-compose.override.yml" <<EOF
services:
  postgres:
    ports: !override
      - "55433:5432"
  redis:
    ports: !override
      - "56380:6379"
  web:
    environment:
      NEXT_PUBLIC_API_URL: http://${PUBLIC_HOST}:8080
      NEXT_PUBLIC_WS_URL: ws://${PUBLIC_HOST}:8080/ws
      NEXT_PUBLIC_CHAIN_NAME: EnergyChain
      NEXT_PUBLIC_DISPLAY_DENOM: ECY
      NEXT_PUBLIC_NATIVE_DECIMALS: "18"
      NEXT_PUBLIC_BECH32_PREFIX: energy
      NEXT_PUBLIC_EVM_CHAIN_ID: "9001"
      NEXT_PUBLIC_EXPLORER_URL: http://${PUBLIC_HOST}:3000
EOF
  build_explorer_indexer
  # api + web build cleanly from their own contexts (no local chain replace).
  ( cd "$EXP_DIR/deploy" && docker compose -p "$EXP_PROJECT" build api web )
  # The indexer expects the schema to exist; it is applied by golang-migrate.
  # Bring up the DB first, migrate, then start the rest of the stack with the
  # pre-built images (no --build, so the indexer image is not rebuilt).
  ( cd "$EXP_DIR/deploy" \
      && docker compose -p "$EXP_PROJECT" up -d postgres redis \
      && sleep 10 \
      && docker run --rm --network "${EXP_PROJECT}_default" \
           -v "$EXP_DIR/indexer/migrations:/migrations" migrate/migrate:v4.17.1 \
           -path=/migrations \
           -database "postgres://explorer:explorer@postgres:5432/energychain_explorer?sslmode=disable" up \
      && docker compose -p "$EXP_PROJECT" up -d )
  echo "explorer up: web http://${PUBLIC_HOST}:3000  api http://${PUBLIC_HOST}:8080"
}

deploy_dex() {
  echo "=== dex ==="
  [ -d "$DEX_DIR/deploy" ] || { echo "missing $DEX_DIR/deploy"; return 1; }
  cp "$DEX_DIR/deploy/.env.example" "$DEX_DIR/deploy/.env"
  # Server-side keys get the in-VPC chain address; NEXT_PUBLIC_* keys get the
  # browser-routable one (see CHAIN_HOST / PUBLIC_HOST at the top).
  python3 - "$DEX_DIR/deploy/.env" "$CHAIN_HOST" "$PUBLIC_HOST" "$DEX_DEPLOYMENT_JSON" <<'PY'
import sys,re,json,os
path,ch,pub,depjson=sys.argv[1],sys.argv[2],sys.argv[3],sys.argv[4]

# Uniswap-V2 addresses, if the contracts have been deployed. The indexer only
# builds OHLCV candles from EVM Swap logs, so without these the /charts and
# /pools pages stay empty no matter how much Cosmos activity there is.
evm={}
if depjson and os.path.exists(depjson):
    c=json.load(open(depjson)).get("contracts",{})
    need=("UniswapV2Factory","UniswapV2Router02","WECY")
    if all(c.get(k) for k in need):
        evm={
         "DEX_EVM_ENABLED":"true",
         "DEX_FACTORY":c["UniswapV2Factory"],
         "DEX_ROUTER":c["UniswapV2Router02"],
         "DEX_WECY":c["WECY"],
         "NEXT_PUBLIC_DEX_FACTORY":c["UniswapV2Factory"],
         "NEXT_PUBLIC_DEX_ROUTER":c["UniswapV2Router02"],
         "NEXT_PUBLIC_DEX_WECY":c["WECY"],
        }
        if c.get("Multicall3"): evm["DEX_MULTICALL"]=c["Multicall3"]
        # USDT anchors the USD price of every WECY pair; without it the UI can
        # quote ECY only in token terms.
        if c.get("TestUSDT"):
            evm["DEX_USDT"]=c["TestUSDT"]
            evm["DEX_STABLE_TOKENS"]=c["TestUSDT"]
        print(f"dex EVM layer ON — factory={c['UniswapV2Factory']}")
if not evm:
    evm={"DEX_EVM_ENABLED":"false"}
    print("dex EVM layer OFF — no dex-deployment.json (Cosmos pages only)")

ov={
 "DEX_EVM_RPC":f"http://{ch}:8545",
 "DEX_EVM_WS":f"ws://{ch}:8546",
 "DEX_COSMOS_ENABLED":"true",
 "DEX_COSMOS_RPC":f"http://{ch}:26657",
 "DEX_COSMOS_REST":f"http://{ch}:1317",
 "NEXT_PUBLIC_DEX_API_BASE":f"http://{pub}:8081",
 "NEXT_PUBLIC_DEX_WS":f"ws://{pub}:8081/ws",
 "NEXT_PUBLIC_DEX_RPC":f"http://{pub}:8545",
 "NEXT_PUBLIC_DEX_COSMOS_RPC":f"http://{pub}:26657",
 "NEXT_PUBLIC_DEX_COSMOS_REST":f"http://{pub}:1317",
 "NEXT_PUBLIC_EXPLORER_BASE":f"http://{pub}:3000",
 # blank: .env.example ships an inline comment that leaks into the value and
 # makes AppKit/WalletConnect SSR-init crash (web 500). Empty disables WC.
 "NEXT_PUBLIC_WC_PROJECT_ID":"",
}
# Last, so the freshly deployed addresses win over .env.example's stale ones.
ov.update(evm)
lines=open(path).read().splitlines()
seen=set()
out=[]
for ln in lines:
    m=re.match(r'^([A-Z0-9_]+)=',ln)
    if m and m.group(1) in ov:
        out.append(f"{m.group(1)}={ov[m.group(1)]}"); seen.add(m.group(1))
    else:
        out.append(ln)
for k,v in ov.items():
    if k not in seen: out.append(f"{k}={v}")
open(path,"w").write("\n".join(out)+"\n")
print("dex .env written")
PY
  # remap nginx off 8080 (used by the explorer api)
  cat > "$DEX_DIR/deploy/docker-compose.override.yml" <<EOF
services:
  nginx:
    ports: !override
      - "8090:80"
EOF
  # apply DB migrations (the indexer expects the schema to already exist) then
  # bring the stack up.
  ( cd "$DEX_DIR/deploy" \
      && docker compose -p "$DEX_PROJECT" --env-file .env up -d postgres redis \
      && docker compose -p "$DEX_PROJECT" --env-file .env --profile migrate run --rm migrate \
      && docker compose -p "$DEX_PROJECT" --env-file .env up -d --build )
  echo "dex up: web http://${PUBLIC_HOST}:3001  api http://${PUBLIC_HOST}:8081"
}

open_firewall() {
  command -v ufw >/dev/null 2>&1 || return 0
  for p in 3000 3001 3002 8080 8081 8090; do sudo ufw allow "${p}/tcp" >/dev/null 2>&1 || true; done
  echo "firewall: opened app ports 3000 3001 3002 8080 8081 8090"
}

case "$WHICH" in
  explorer) deploy_explorer ;;
  dex)      deploy_dex ;;
  all)      deploy_explorer; deploy_dex ;;
  *) echo "usage: apps.sh [all|dex|explorer]"; exit 1 ;;
esac
open_firewall
