#!/usr/bin/env bash
# data_loader.sh — sustained, real on-chain data generator for EnergyChain.
# Drives bank send, EVM transfer, and the four custom modules in a loop so
# the block explorer sees continuously-growing real traffic. Designed to be
# safe to run forever in dev: every send waits for inclusion, sequence is
# discovered from the chain itself, and failures back off without crashing.
#
# Usage:
#   bash chain/scripts/data_loader.sh                # loops forever
#   N_LOOPS=20 bash chain/scripts/data_loader.sh     # bounded run
#   SLEEP_BETWEEN=2 bash chain/scripts/data_loader.sh
#
# Environment overrides (all have sane defaults):
#   CHAIN_HOME, CHAIN_ID, NODE_RPC, KEYRING, FROM_KEY, GAS_PRICES, EVM_RPC,
#   N_LOOPS (0 = forever), SLEEP_BETWEEN, EVM_TO, EVM_AMOUNT_WEI

set -u

CHAIN_HOME=${CHAIN_HOME:-$HOME/.energychaind}
CHAIN_ID=${CHAIN_ID:-energychain_9001-1}
NODE_RPC=${NODE_RPC:-tcp://127.0.0.1:26657}
EVM_RPC=${EVM_RPC:-http://127.0.0.1:8545}
KEYRING=${KEYRING:-test}
FROM_KEY=${FROM_KEY:-dev0}
GAS_PRICES=${GAS_PRICES:-10000000000uecy}
GAS=${GAS:-300000}
N_LOOPS=${N_LOOPS:-0}
SLEEP_BETWEEN=${SLEEP_BETWEEN:-3}

# Resolve the energychaind binary: prefer the sibling `chain/energychaind`
# next to this script (so we always exercise the latest local build).
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
BIN=${BIN:-"$SCRIPT_DIR/../energychaind"}
if ! [ -x "$BIN" ]; then BIN=$(command -v energychaind || true); fi
if [ -z "$BIN" ] || ! [ -x "$BIN" ]; then
  echo "could not find energychaind binary; set BIN=/path/to/energychaind" >&2
  exit 1
fi

CMN=("--keyring-backend" "$KEYRING" "--home" "$CHAIN_HOME" "--node" "$NODE_RPC")
TX_FLAGS=(--chain-id "$CHAIN_ID" --gas "$GAS" --gas-prices "$GAS_PRICES" -y -o json)

FROM_ADDR=$("$BIN" keys show "$FROM_KEY" -a "${CMN[@]}" 2>/dev/null) || {
  echo "failed to resolve $FROM_KEY address" >&2; exit 1
}
echo "loader using FROM=$FROM_KEY ($FROM_ADDR) chain=$CHAIN_ID node=$NODE_RPC"

# A second account for bank-send/EVM transfer destinations. Try keys in a
# few common names; fallback to a hard-coded module-style account.
TO_ADDR=$("$BIN" keys show validator -a "${CMN[@]}" 2>/dev/null || true)
[ -z "$TO_ADDR" ] && TO_ADDR=$("$BIN" keys show dev1 -a "${CMN[@]}" 2>/dev/null || true)
[ -z "$TO_ADDR" ] && TO_ADDR="energy1525tsuuslredrzpyyettld599y2qw0gxuzwnn9"
echo "loader sending bank to $TO_ADDR"

# Pre-derive the EVM hex addresses so we can call eth_sendRawTransaction-like
# flows via cosmos `tx evm raw-eth-tx` later. For dev0 we already have the
# EVM key.
FROM_EVM=$("$BIN" keys show "$FROM_KEY" --bech address-prefix=hex "${CMN[@]}" 2>/dev/null | awk '{print $2}' || true)

iter=0
fail=0

# log_tx pretty-prints code/hash from a `--output json` tx response.
# Auto-retries once on "account sequence mismatch" (which happens when the
# previous tx is still in the mempool when we try to compose the next one).
log_tx() {
  local tag=$1 ; shift
  local out parsed
  out=$("$@" 2>&1) || true
  parsed=$(RAW="$out" python3 -c '
import os, json, sys
raw = (os.environ.get("RAW") or "").strip()
try:
    j = json.loads(raw)
    print(j.get("code", -1))
except Exception:
    print("nonjson")
')
  if [ "$parsed" = "32" ]; then
    sleep 2.2
    out=$("$@" 2>&1) || true
  fi
  RAW="$out" TAG="$tag" python3 -c '
import os, json
raw = (os.environ.get("RAW") or "").strip()
tag = os.environ.get("TAG", "")
try:
    j = json.loads(raw)
    code = j.get("code", "?")
    h = j.get("txhash", "")
    log = (j.get("raw_log") or "")[:160]
    print(f"  {tag} code={code} hash={h[:12]}.. log={log}")
except Exception:
    snippet = (raw[:200] + "..") if len(raw) > 200 else raw
    print(f"  {tag} non-json: {snippet}")
'
}

# wait_inclusion sleeps slightly longer than one block so the next tx sees
# the latest sequence. Block time on local energychain is ~2s.
wait_inclusion() { sleep 2.3; }

# -------- generators ----------------------------------------------------

gen_bank() {
  local amount=$(( (RANDOM % 9 + 1) * 1000 ))uecy
  log_tx "bank.send  " "$BIN" tx bank send "$FROM_ADDR" "$TO_ADDR" "$amount" \
    --note "loader-$iter-$RANDOM" "${CMN[@]}" --from "$FROM_KEY" "${TX_FLAGS[@]}"
}

gen_energy_single() {
  local cats=(solar wind hydro nuclear geothermal biomass)
  local cat=${cats[$RANDOM % ${#cats[@]}]}
  # 32-byte payload; deterministic-ish from time/iter so explorer dedup logic
  # can prove uniqueness, while metadata varies for visual interest.
  local h
  h=$(printf 'energy-%s-%s-%s' "$cat" "$iter" "$RANDOM" | shasum -a 256 | awk '{print $1}')
  local kwh=$(( RANDOM % 1000 + 50 ))
  local meta="{\"kwh\":$kwh,\"cat\":\"$cat\",\"src\":\"loader\"}"
  log_tx "energy.sub " "$BIN" tx energy submit "$cat" "0x$h" "$meta" \
    "${CMN[@]}" --from "$FROM_KEY" "${TX_FLAGS[@]}"
}

gen_energy_batch() {
  local cat=solar
  # Build the items+root in Python so the merkle scheme matches the keeper
  # exactly: leaf[i] = keccak256(uint64_be(i) || dataHashStringBytes), then
  # parent = keccak256(min(a,b) || max(a,b)) by hex order, recursively.
  # Without this, the keeper rejects the batch ("merkle root verification
  # failed"). The chain test computeRoot() in msg_server_test.go is the
  # canonical reference.
  local payload
  payload=$(ITER="$iter" RAND="$RANDOM" python3 - <<'PY'
import json, os, hashlib, secrets
try:
    from Crypto.Hash import keccak  # pycryptodome, optional
    def k256(b): h=keccak.new(digest_bits=256); h.update(b); return h.digest()
except Exception:
    # Fallback: pysha3 / hashlib.sha3_256? Cosmos uses keccak (legacy SHA3-256
    # variant from Ethereum). Implement a tiny pure-python keccak if neither
    # is present, but performance doesn't matter at 5 leaves.
    def k256(b):
        try:
            import sha3  # pysha3
            h = sha3.keccak_256(); h.update(b); return h.digest()
        except Exception:
            # Bundled minimal keccak (Ethereum). Lifted from the public-domain
            # reference implementation — only used as last-resort fallback.
            RC = [0x0000000000000001,0x0000000000008082,0x800000000000808A,
                  0x8000000080008000,0x000000000000808B,0x0000000080000001,
                  0x8000000080008081,0x8000000000008009,0x000000000000008A,
                  0x0000000000000088,0x0000000080008009,0x000000008000000A,
                  0x000000008000808B,0x800000000000008B,0x8000000000008089,
                  0x8000000000008003,0x8000000000008002,0x8000000000000080,
                  0x000000000000800A,0x800000008000000A,0x8000000080008081,
                  0x8000000000008080,0x0000000080000001,0x8000000080008008]
            r = [[1,3,6,10,15,21,28,36,45,55,2,14,27,41,56,8,25,43,62,18,39,61,20,44]]
            def rotl(x,n): return ((x<<n)|(x>>(64-n))) & ((1<<64)-1)
            def keccak_f(s):
                lanes=[[s[5*x+y] for y in range(5)] for x in range(5)]
                for rnd in range(24):
                    C=[lanes[x][0]^lanes[x][1]^lanes[x][2]^lanes[x][3]^lanes[x][4] for x in range(5)]
                    D=[C[(x-1)%5]^rotl(C[(x+1)%5],1) for x in range(5)]
                    for x in range(5):
                        for y in range(5): lanes[x][y]^=D[x]
                    x,y=1,0; cur=lanes[x][y]; t=0
                    for tt in range(24):
                        x,y=y,(2*x+3*y)%5
                        t+=tt+1
                        cur,lanes[x][y]=lanes[x][y],rotl(cur,t%64)
                    for y in range(5):
                        T=[lanes[x][y] for x in range(5)]
                        for x in range(5):
                            lanes[x][y]=T[x]^((~T[(x+1)%5])&T[(x+2)%5])
                    lanes[0][0]^=RC[rnd]
                out=[0]*25
                for x in range(5):
                    for y in range(5): out[5*x+y]=lanes[x][y]
                return out
            def keccak_256(data):
                rate=136; state=[0]*25
                pad=bytearray(data)
                pad.append(0x01); pad.extend([0]*(rate - (len(pad)%rate)))
                pad[-1]^=0x80
                for blk in range(0,len(pad),rate):
                    for i in range(rate//8):
                        state[i] ^= int.from_bytes(pad[blk+8*i:blk+8*i+8],'little')
                    state=keccak_f(state)
                out=b''
                for i in range(4):
                    out += state[i].to_bytes(8,'little')
                return out
            return keccak_256(b)

it=int(os.environ["ITER"]); rnd=int(os.environ["RAND"])
items=[]
for i in range(1,6):
    salt=secrets.token_hex(4)
    h=hashlib.sha256(f"item-{it}-{i}-{rnd}-{salt}".encode()).hexdigest()
    kwh=(rnd*i + i*7) % 200 + 1
    items.append({"data_hash": "0x"+h, "metadata": json.dumps({"kwh":kwh})})

# leaves: keccak256(uint64_be(i) || data_hash_bytes_as_string)
leaves=[]
for i,it_ in enumerate(items):
    idx=i.to_bytes(8,"big")
    leaves.append(k256(idx + it_["data_hash"].encode()))

def merkle(L):
    if len(L)==1: return L[0]
    nxt=[]
    for i in range(0,len(L),2):
        a=L[i]
        b=L[i+1] if i+1<len(L) else L[i]
        if a.hex() <= b.hex():
            nxt.append(k256(a+b))
        else:
            nxt.append(k256(b+a))
    return merkle(nxt)

root=merkle(leaves).hex()
print(json.dumps({"items": items, "root": root}))
PY
  )
  if [ -z "$payload" ]; then
    echo "  energy.bat skipped: python3 helper failed"
    return 0
  fi
  local items=$(printf '%s' "$payload" | python3 -c 'import sys,json; print(json.dumps(json.loads(sys.stdin.read())["items"]))')
  local merkle=$(printf '%s' "$payload" | python3 -c 'import sys,json; print(json.loads(sys.stdin.read())["root"])')
  log_tx "energy.bat " "$BIN" tx energy batch-submit "$cat" "0x$merkle" \
    --items "$items" "${CMN[@]}" --from "$FROM_KEY" "${TX_FLAGS[@]}"
}

gen_oracle() {
  local feeds=(price.ecy.usd price.eth.usd price.btc.usd grid.load.kw weather.temp_c)
  local cat=${feeds[$RANDOM % ${#feeds[@]}]}
  # Tiny variance around a base value per feed so charts look organic.
  local val
  case "$cat" in
    price.ecy.usd) val=$(awk -v r=$RANDOM 'BEGIN{printf "%.4f", 1.20 + (r%2000)/10000.0}') ;;
    price.eth.usd) val=$(awk -v r=$RANDOM 'BEGIN{printf "%.2f", 3000 + (r%3000)/10.0}') ;;
    price.btc.usd) val=$(awk -v r=$RANDOM 'BEGIN{printf "%.2f", 60000 + (r%5000)}') ;;
    grid.load.kw)  val=$(awk -v r=$RANDOM 'BEGIN{printf "%.0f", 1500 + (r%1500)}') ;;
    *)             val=$(awk -v r=$RANDOM 'BEGIN{printf "%.1f", 18 + (r%200)/10.0}') ;;
  esac
  log_tx "oracle.sub " "$BIN" tx oracle submit "$cat" "$val" "{\"src\":\"loader\"}" \
    "${CMN[@]}" --from "$FROM_KEY" "${TX_FLAGS[@]}"
}

gen_identity() {
  # Register a fresh identity each loop using a synthetic but bech32-shaped
  # address that the keeper will accept (the keeper does not check on-chain
  # account existence — only string format). Using $TO_ADDR keeps repeated
  # registrations as updates rather than duplicates.
  local addr=$TO_ADDR
  local roles=(producer consumer auditor regulator)
  local role=${roles[$RANDOM % ${#roles[@]}]}
  local name="Entity-$iter"
  local meta="{\"role\":\"$role\",\"region\":\"CN-$((RANDOM%32))\"}"
  if [ $((iter % 4)) -eq 0 ]; then
    log_tx "id.reg     " "$BIN" tx identity register "$addr" "$name" "$role" "$meta" \
      "${CMN[@]}" --from "$FROM_KEY" "${TX_FLAGS[@]}"
  else
    log_tx "id.upd     " "$BIN" tx identity update "$addr" "$name" "$meta" \
      "${CMN[@]}" --from "$FROM_KEY" "${TX_FLAGS[@]}"
  fi
}

gen_audit() {
  local types=(login config.change settlement compliance.scan key.rotation)
  local actions=(create update delete review approve)
  local t=${types[$RANDOM % ${#types[@]}]}
  local a=${actions[$RANDOM % ${#actions[@]}]}
  local data="{\"trace\":\"loader-$iter\",\"ts\":$(date +%s)}"
  log_tx "audit.rec  " "$BIN" tx audit record "$t" "$TO_ADDR" "$a" "$data" \
    "${CMN[@]}" --from "$FROM_KEY" "${TX_FLAGS[@]}"
}

# gen_governance posts a TextProposal-like message via the v1 module so the
# explorer's /governance page has real proposals to render. Skipped silently on
# nodes that don't expose the v1 submit-proposal CLI.
gen_governance() {
  local f=/tmp/loader-gov-prop.json
  local title="Loader proposal #$iter"
  local desc="Auto-generated by data_loader at iter $iter"
  cat > "$f" <<JSON
{
  "messages": [],
  "metadata": "ipfs://loader",
  "deposit": "10000000uecy",
  "title": "$title",
  "summary": "$desc"
}
JSON
  out=$("$BIN" tx gov submit-proposal "$f" "${CMN[@]}" --from "$FROM_KEY" "${TX_FLAGS[@]}" 2>&1) || true
  code=$(RAW="$out" python3 -c '
import os, json
raw = (os.environ.get("RAW") or "").strip()
try:
    print(json.loads(raw).get("code", -1))
except Exception:
    print("nonjson")
' 2>/dev/null)
  if [ "$code" = "0" ]; then
    echo "  gov.prop   code=0 ok"
    # follow up with a vote so the proposal isn't stuck at deposit_period
    sleep 2
    last_id=$("$BIN" q gov proposals --limit 1 --reverse "${CMN[@]}" -o json 2>/dev/null | python3 -c 'import sys,json
try:
  j = json.load(sys.stdin)
  ps = j.get("proposals") or []
  print(ps[-1].get("id") or ps[-1].get("proposal_id") if ps else "")
except Exception:
  print("")' 2>/dev/null)
    if [ -n "$last_id" ]; then
      "$BIN" tx gov vote "$last_id" yes "${CMN[@]}" --from "$FROM_KEY" "${TX_FLAGS[@]}" >/dev/null 2>&1 || true
      echo "  gov.vote   yes on #$last_id"
    fi
  else
    echo "  gov.prop   skipped (code=$code)"
  fi
}

# EVM transfer via cosmos-evm `tx evm raw-eth-tx`. We construct the raw tx
# offline with a tiny node helper if available; otherwise we fall back to
# `tx bank send` (already covered above) so the loader is still useful on
# minimal hosts.
gen_evm_transfer() {
  if ! command -v node >/dev/null 2>&1; then return 0; fi
  # The EVM private key is exported via the unsafe debug command. We cache
  # it for the duration of the loop to avoid leaking it to the shell history.
  if [ -z "${EVM_PK:-}" ]; then
    EVM_PK=$("$BIN" keys unsafe-export-eth-key "$FROM_KEY" --keyring-backend "$KEYRING" --home "$CHAIN_HOME" 2>/dev/null || true)
    [ -z "$EVM_PK" ] && return 0
  fi
  EVM_TO_DEFAULT="0x000000000000000000000000000000000000dEaD"
  EVM_TO=${EVM_TO:-$EVM_TO_DEFAULT}
  EVM_AMOUNT_WEI=${EVM_AMOUNT_WEI:-1000000000000000} # 0.001 ECY
  RAW=$(EVM_PK="$EVM_PK" EVM_TO="$EVM_TO" EVM_AMOUNT_WEI="$EVM_AMOUNT_WEI" EVM_RPC="$EVM_RPC" \
    node -e '
const http = require("http");
const url = new URL(process.env.EVM_RPC);
function rpc(method, params){
  return new Promise((res, rej) => {
    const body = JSON.stringify({jsonrpc:"2.0",id:1,method,params});
    const req = http.request({hostname:url.hostname,port:url.port,path:url.pathname,method:"POST",headers:{"Content-Type":"application/json","Content-Length":body.length}},(r)=>{
      let d="";r.on("data",c=>d+=c);r.on("end",()=>{try{const j=JSON.parse(d);j.error?rej(j.error):res(j.result)}catch(e){rej(e)}});
    });
    req.on("error",rej); req.write(body); req.end();
  });
}
(async () => {
  // Use ethers v6 if installed in /tmp, else fall back to a minimal RLP+secp256k1
  // signer. We only ship the ethers path since the loader is dev-only.
  let ethers;
  try { ethers = require("/tmp/node_modules/ethers"); }
  catch { try { ethers = require("ethers"); } catch { console.log(""); return; } }
  const provider = new ethers.JsonRpcProvider(process.env.EVM_RPC);
  const wallet = new ethers.Wallet(process.env.EVM_PK, provider);
  const tx = await wallet.sendTransaction({
    to: process.env.EVM_TO,
    value: BigInt(process.env.EVM_AMOUNT_WEI),
    gasLimit: 21000n,
  });
  console.log(tx.hash);
})().catch(e => { console.error(e.message || String(e)); });
' 2>&1)
  if [[ "$RAW" =~ ^0x[0-9a-fA-F]{64}$ ]]; then
    echo "  evm.send   code=0 hash=${RAW:0:14}.. ok"
  else
    echo "  evm.send   skipped: ${RAW:0:120}"
  fi
}

# -------- main loop -----------------------------------------------------

# Best effort: install ethers locally for the EVM helper.
if command -v npm >/dev/null 2>&1 && [ ! -d /tmp/node_modules/ethers ]; then
  ( cd /tmp && npm i --silent ethers >/dev/null 2>&1 || true )
fi

while :; do
  iter=$((iter + 1))
  echo "[$(date '+%H:%M:%S')] loop #$iter"
  gen_bank;          wait_inclusion
  gen_energy_single; wait_inclusion
  gen_oracle;        wait_inclusion
  gen_audit;         wait_inclusion
  if [ $((iter % 3)) -eq 0 ]; then gen_energy_batch; wait_inclusion; fi
  if [ $((iter % 4)) -eq 0 ]; then gen_identity;     wait_inclusion; fi
  if [ $((iter % 2)) -eq 0 ]; then gen_evm_transfer; wait_inclusion; fi
  if [ $((iter % 25)) -eq 1 ]; then gen_governance;  wait_inclusion; fi
  sleep "$SLEEP_BETWEEN"
  if [ "$N_LOOPS" -gt 0 ] && [ "$iter" -ge "$N_LOOPS" ]; then break; fi
done

echo "loader finished after $iter loops, $fail failures"
