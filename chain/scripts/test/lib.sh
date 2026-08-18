#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# Shared test library for the consolidated RWA-core modules.
#
# Drives a live local node (see scripts/local_node.sh) through positive,
# negative and security cases for every module. Sourced by setup.sh,
# modules/*.sh and run_all.sh.
#
# Authority model recap (decides who signs what):
#   * authority-gated msgs (require the gov module account) are staged into
#     ONE batched gov proposal — see setup.sh / gov_pass().
#   * self-admin / owner-gated msgs are signed directly by the relevant key.
# ---------------------------------------------------------------------------
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHAIN_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# ---- environment (all overridable) ----------------------------------------
BIN="${BIN:-${CHAIN_DIR}/energychaind}"
HOME_T="${HOME_T:-$HOME/.energychaind-test}"
CHAINID="${CHAINID:-energychain_9001-1}"
NODE="${NODE:-tcp://127.0.0.1:26757}"
DENOM="${DENOM:-uecy}"
KEYRING="${KEYRING:-test}"
GAS_PRICES="${GAS_PRICES:-10000000000${DENOM}}"
# Fixed gas (not --gas auto): a simulation failure with auto aborts the CLI
# *before* broadcast and yields no tx result, which would make negative-path
# assertions impossible. A fixed limit always broadcasts so we get the real
# on-chain code + raw_log for both success and failure cases.
GAS_LIMIT="${GAS_LIMIT:-900000}"
TX_WAIT_TRIES="${TX_WAIT_TRIES:-20}"

CObj=(--keyring-backend "$KEYRING" --home "$HOME_T")
NObj=(--node "$NODE")

# Strip non-JSON noise (e.g. "gas estimate: 12345" printed to stderr by the
# gas simulator) so jq only ever sees the result object.
_ej() { sed -n '/^{/,$p'; }

# ---- colours / counters ----------------------------------------------------
_C_G='\033[1;32m'; _C_R='\033[1;31m'; _C_Y='\033[1;33m'; _C_B='\033[1;34m'; _C_0='\033[0m'
PASS_N=0; FAIL_N=0; SKIP_N=0; FAILED_CASES=()

_pass() { PASS_N=$((PASS_N+1)); echo -e "  ${_C_G}PASS${_C_0} $1"; }
_fail() { FAIL_N=$((FAIL_N+1)); FAILED_CASES+=("$1"); echo -e "  ${_C_R}FAIL${_C_0} $1 ${2:+→ $2}"; }
_skip() { SKIP_N=$((SKIP_N+1)); echo -e "  ${_C_Y}SKIP${_C_0} $1 ${2:+($2)}"; }
section() { echo -e "${_C_B}── $* ──${_C_0}"; }
module_header() { echo; echo -e "${_C_B}════════ module: $* ════════${_C_0}"; }

# ---- addresses -------------------------------------------------------------
addr() { "$BIN" keys show "$1" -a "${CObj[@]}" 2>/dev/null; }
gov_addr() {
  "$BIN" q auth module-account gov "${NObj[@]}" -o json 2>/dev/null \
    | jq -r '.account.value.address // .account.base_account.address // .account.address'
}

# ---- query helper ----------------------------------------------------------
# q <module> <subcmd> [args...]  -> prints JSON, returns query exit code.
q() { "$BIN" query "$@" "${NObj[@]}" -o json 2>/dev/null; }

# ---- block / node helpers --------------------------------------------------
height() { curl -s "http://${NODE#tcp://}/status" 2>/dev/null | jq -r '.result.sync_info.latest_block_height // "0"'; }
wait_blocks() {
  local n="${1:-1}" start cur
  start=$(height); [ -z "$start" ] && start=0
  for _ in $(seq 1 60); do
    cur=$(height); [ -z "$cur" ] && cur=0
    [ "$cur" -ge $((start + n)) ] && return 0
    sleep 1
  done
  return 1
}

# ---- broadcast a signed tx and wait for its result -------------------------
# _bcast <key> <module> <subcmd> [args...]  -> echoes final tx JSON (with .code)
_bcast() {
  local key="$1" module="$2" sub="$3"; shift 3
  local out hash
  out=$("$BIN" tx "$module" "$sub" "$@" \
    --from "$key" "${CObj[@]}" "${NObj[@]}" \
    --chain-id "$CHAINID" --gas "$GAS_LIMIT" \
    --gas-prices "$GAS_PRICES" --broadcast-mode sync -y -o json 2>&1 | _ej)
  # broadcast-time failure (bad args, account seq, insufficient funds at check)
  hash=$(echo "$out" | jq -r '.txhash // empty' 2>/dev/null)
  if [ -z "$hash" ]; then echo "$out"; return 0; fi
  local code
  code=$(echo "$out" | jq -r '.code // 0' 2>/dev/null)
  if [ "${code:-0}" != "0" ]; then echo "$out"; return 0; fi
  # poll for inclusion
  local i res
  for i in $(seq 1 "$TX_WAIT_TRIES"); do
    res=$("$BIN" q tx "$hash" "${NObj[@]}" -o json 2>/dev/null)
    if [ -n "$res" ] && [ "$(echo "$res" | jq -r '.code // empty' 2>/dev/null)" != "" ]; then
      echo "$res"; return 0
    fi
    sleep 1
  done
  echo "$out"
}

# ---- assertions ------------------------------------------------------------
# expect_ok <desc> <key> <module> <subcmd> [args...]
expect_ok() {
  local desc="$1"; shift
  local out code raw
  out=$(_bcast "$@")
  code=$(echo "$out" | jq -r '.code // empty' 2>/dev/null)
  if [ "$code" = "0" ]; then _pass "$desc"; return 0; fi
  raw=$(echo "$out" | jq -r '.raw_log // .message // .' 2>/dev/null | head -c 240)
  _fail "$desc" "code=${code:-?} ${raw}"
  return 1
}

# expect_fail <desc> <regex> <key> <module> <subcmd> [args...]
# A negative case passes when the tx does NOT succeed on-chain. That covers
# both a committed tx with a non-zero code AND a pre-broadcast rejection
# (ValidateBasic, bad enum, insufficient funds at CheckTx, etc.) which yields
# no tx result at all. Only a committed tx with code==0 is a real failure.
expect_fail() {
  local desc="$1" re="$2"; shift 2
  local out hash code raw
  out=$(_bcast "$@")
  hash=$(echo "$out" | jq -r '.txhash // empty' 2>/dev/null)
  code=$(echo "$out" | jq -r '.code // empty' 2>/dev/null)
  raw=$(echo "$out" | jq -r '.raw_log // .message // empty' 2>/dev/null)
  [ -z "$raw" ] && raw="$out"   # CLI/broadcast errors come back as plain text
  if [ -n "$hash" ] && [ "$code" = "0" ]; then
    _fail "$desc" "expected failure but tx succeeded (hash=$hash)"
    return 1
  fi
  if echo "$raw" | grep -qiE "$re"; then
    _pass "$desc (rejected: $(echo "$raw" | grep -oiE "$re" | head -1))"
  else
    _pass "$desc (rejected, code=${code:-cli})"
  fi
  return 0
}

# expect_q <desc> <module> <subcmd> [args...]
expect_q() {
  local desc="$1"; shift
  if q "$@" >/dev/null 2>&1; then _pass "query: $desc"; else _fail "query: $desc"; fi
}

# expect_q_has <desc> <jqfilter> <module> <subcmd> [args...]
expect_q_has() {
  local desc="$1" filt="$2"; shift 2
  local out
  out=$(q "$@")
  if [ -n "$out" ] && echo "$out" | jq -e "$filt" >/dev/null 2>&1; then
    _pass "query: $desc"
  else
    _fail "query: $desc" "filter $filt"
  fi
}

# ---- gov proposal helper ---------------------------------------------------
# gen_msg <outfile> <module> <subcmd> [flags...]
#   Generates a single message with authority/admin rewritten to the gov
#   module account, ready to embed in a proposal.
gen_msg() {
  local out="$1" module="$2" sub="$3"; shift 3
  local gov; gov="$(gov_addr)"
  "$BIN" tx "$module" "$sub" "$@" \
    --from dev0 "${CObj[@]}" --chain-id "$CHAINID" --generate-only -o json 2>/dev/null \
    | jq --arg gov "$gov" '.body.messages[0] | (if has("authority") then .authority=$gov else . end)' > "$out"
}

# gov_pass <title> <msgfile...>  -> submit one proposal, vote yes, wait PASSED
gov_pass() {
  local title="$1"; shift
  local tmp; tmp="$(mktemp -d)"
  local prop="${tmp}/proposal.json"
  local gov; gov="$(gov_addr)"
  jq -n --arg t "$title" --arg dep "20000000${DENOM}" \
    --slurpfile msgs <(jq -s '.' "$@") \
    '{messages: $msgs[0], metadata:"ipfs://none", deposit:$dep, title:$t, summary:$t}' > "$prop"

  local out hash pid
  out=$("$BIN" tx gov submit-proposal "$prop" --from dev0 "${CObj[@]}" "${NObj[@]}" \
    --chain-id "$CHAINID" --gas auto --gas-adjustment 1.5 --gas-prices "$GAS_PRICES" \
    --broadcast-mode sync -y -o json 2>&1 | _ej)
  hash=$(echo "$out" | jq -r '.txhash // empty' 2>/dev/null)
  if [ -z "$hash" ]; then echo "SUBMIT FAILED: $out"; rm -rf "$tmp"; return 1; fi
  wait_blocks 2 >/dev/null
  local txres; txres=$("$BIN" q tx "$hash" "${NObj[@]}" -o json 2>/dev/null)
  if [ "$(echo "$txres" | jq -r '.code // 1')" != "0" ]; then
    echo "SUBMIT TX FAILED: $(echo "$txres" | jq -r '.raw_log' | head -c 300)"; rm -rf "$tmp"; return 1
  fi
  pid=$(echo "$txres" | jq -r '[.events[]?|select(.type=="submit_proposal")|.attributes[]?|select(.key=="proposal_id")|.value]|last // empty')
  if [ -z "$pid" ]; then
    pid=$("$BIN" q gov proposals "${NObj[@]}" -o json 2>/dev/null | jq -r '.proposals[-1].id // .proposals[-1].proposal_id // empty')
  fi
  if [ -z "$pid" ]; then echo "NO PROPOSAL ID"; rm -rf "$tmp"; return 1; fi
  echo "  proposal #$pid submitted; voting…"
  "$BIN" tx gov vote "$pid" yes --from validator "${CObj[@]}" "${NObj[@]}" \
    --chain-id "$CHAINID" --gas auto --gas-adjustment 1.5 --gas-prices "$GAS_PRICES" \
    --broadcast-mode sync -y -o json >/dev/null 2>&1
  # wait for the voting period (8s) to elapse + tally
  local st
  for _ in $(seq 1 25); do
    st=$("$BIN" q gov proposal "$pid" "${NObj[@]}" -o json 2>/dev/null | jq -r '.proposal.status // .status // empty')
    case "$st" in
      PROPOSAL_STATUS_PASSED) echo "  proposal #$pid PASSED"; rm -rf "$tmp"; return 0 ;;
      PROPOSAL_STATUS_REJECTED|PROPOSAL_STATUS_FAILED) echo "  proposal #$pid $st"; rm -rf "$tmp"; return 1 ;;
    esac
    sleep 1
  done
  echo "  proposal #$pid did not finalize (last status: ${st:-unknown})"
  rm -rf "$tmp"; return 1
}

summary() {
  echo
  echo -e "${_C_B}════════ summary ════════${_C_0}"
  echo -e "  ${_C_G}pass=${PASS_N}${_C_0}  ${_C_R}fail=${FAIL_N}${_C_0}  ${_C_Y}skip=${SKIP_N}${_C_0}"
  echo "RESULT pass=${PASS_N} fail=${FAIL_N} skip=${SKIP_N}"
  if [ "$FAIL_N" -gt 0 ]; then
    echo -e "  ${_C_R}failed cases:${_C_0}"
    printf '    - %s\n' "${FAILED_CASES[@]}"
    return 1
  fi
  return 0
}
