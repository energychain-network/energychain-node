#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# End-to-end module test runner for the 9 consolidated RWA-core modules.
#
# Prerequisite: a FRESH local node must be running, e.g.
#   CHAINDIR=$HOME/.energychaind-test RPC_LADDR=tcp://127.0.0.1:26757 \
#   P2P_LADDR=tcp://0.0.0.0:26756 GRPC_ADDR=127.0.0.1:9390 \
#   API_ADDR=tcp://127.0.0.1:1417 JSONRPC_ADDR=127.0.0.1:8645 \
#   JSONRPC_WS_ADDR=127.0.0.1:8646 PROM_PORT=26760 \
#   bash scripts/local_node.sh -y
#
# Then:  NODE=tcp://127.0.0.1:26757 HOME_T=$HOME/.energychaind-test \
#        bash scripts/test/run_all.sh
# ---------------------------------------------------------------------------
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${HERE}/lib.sh"

MODULES=(identity assethub stableusd rwatoken mincast offering market automation bridge)

echo -e "${_C_B}################ RWA-core module test suite ################${_C_0}"
echo "node=$NODE home=$HOME_T bin=$BIN"

# 1) fixtures
echo -e "\n${_C_B}>>> setup (fixtures + governance bootstrap)${_C_0}"
if ! bash "${HERE}/setup.sh"; then
  echo -e "${_C_R}setup failed — aborting suite${_C_0}"; exit 1
fi

# 2) per-module suites
TOT_P=0; TOT_F=0; TOT_S=0; FAILED_MODULES=()
for m in "${MODULES[@]}"; do
  out=$(bash "${HERE}/modules/${m}.sh" 2>&1)
  echo "$out"
  line=$(echo "$out" | grep -E '^RESULT ' | tail -1)
  p=$(echo "$line" | sed -nE 's/.*pass=([0-9]+).*/\1/p'); p="${p:-0}"
  f=$(echo "$line" | sed -nE 's/.*fail=([0-9]+).*/\1/p'); f="${f:-0}"
  s=$(echo "$line" | sed -nE 's/.*skip=([0-9]+).*/\1/p'); s="${s:-0}"
  TOT_P=$((TOT_P + p)); TOT_F=$((TOT_F + f)); TOT_S=$((TOT_S + s))
  [ "${f:-0}" -gt 0 ] && FAILED_MODULES+=("$m")
done

echo
echo -e "${_C_B}################ GRAND TOTAL ################${_C_0}"
echo -e "  ${_C_G}pass=${TOT_P}${_C_0}  ${_C_R}fail=${TOT_F}${_C_0}  ${_C_Y}skip=${TOT_S}${_C_0}"
if [ "${#FAILED_MODULES[@]}" -gt 0 ]; then
  echo -e "  ${_C_R}modules with failures:${_C_0} ${FAILED_MODULES[*]}"
  exit 1
fi
echo -e "  ${_C_G}ALL MODULES PASSED${_C_0}"
