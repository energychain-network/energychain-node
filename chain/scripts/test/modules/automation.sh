#!/usr/bin/env bash
# x/automation — cron schedules + streaming settlement payments.
set -uo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib.sh"
source "${SCRIPT_DIR}/.fixtures.env"

module_header "automation (scheduler + streampay)"
T="${RWA_ID:-1}"; M="${MCX_ID:-1}"

section "positive — schedules"
expect_ok "create rwa-snapshot schedule" dev0 automation create-schedule rwa-snapshot "$T" 0 60 0 0 0
SID=$(q automation schedules | jq -r '.schedules[-1].id // empty'); SID="${SID:-1}"
expect_q "schedule $SID"        automation schedule "$SID"
expect_ok "pause schedule"      dev0 automation pause-schedule "$SID"
expect_ok "resume schedule"     dev0 automation resume-schedule "$SID"

section "positive — streams (usd settlement)"
NOW=$(date +%s)
expect_ok "dev0 opens stream -> alice" dev0 automation create-stream "$ALICE" usd 10000000 100000 "$NOW"
STID=$(q automation streams | jq -r '.streams[-1].id // empty'); STID="${STID:-1}"
expect_q "stream $STID"            automation stream "$STID"
echo "  letting stream vest…"; wait_blocks 2 >/dev/null
expect_q "withdrawable balance"    automation withdrawable "$STID"
expect_ok "receiver withdraws vested" alice automation withdraw-stream "$STID"

section "negative"
expect_fail "schedule with unknown target rejected" "not found|token" \
  dev0 automation create-schedule rwa-snapshot 999 0 60 0 0 0
expect_fail "schedule below min interval rejected" "interval|min|invalid" \
  dev0 automation create-schedule rwa-snapshot "$T" 0 1 0 0 0

section "security"
expect_fail "cancel-schedule by non-creator(alice) rejected" "creator|unauthor" \
  alice automation cancel-schedule "$SID"
expect_fail "cancel-stream by non-sender(alice) rejected" "sender|unauthor" \
  alice automation cancel-stream "$STID"

summary
