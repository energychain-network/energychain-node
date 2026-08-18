#!/usr/bin/env bash
# SSM command channel — replaces ssh/rsync for hosts behind a flaky or
# DPI-filtered link. Everything tunnels over the HTTPS control plane, so it
# survives the cross-border conditions that keep killing port 22.
#
# Usage:
#   _ecssm.sh run  '<shell>'          run inline, wait, print stdout/stderr
#   _ecssm.sh runf <file>             same but read the script from a file
#   _ecssm.sh bg   <tag> '<shell>'    launch detached, log to ~/<tag>.log
#   _ecssm.sh log  <tag> [lines]      tail that log
#   _ecssm.sh wait <tag> [max_min]    poll until ~/<tag>.done appears
#
# Env: SSM_INSTANCE, AWS_PROFILE, SSM_REGION, SSM_TIMEOUT (inline wait, sec)
set -uo pipefail

INSTANCE="${SSM_INSTANCE:-i-0436008701c67f4b2}"
PROFILE="${AWS_PROFILE:-primcast}"
REGION="${SSM_REGION:-ap-southeast-1}"
TIMEOUT="${SSM_TIMEOUT:-600}"
RUN_AS="${SSM_RUN_AS:-ubuntu}"

aws_ssm() { aws ssm "$@" --profile "$PROFILE" --region "$REGION"; }

# SSM runs commands as root. Nearly everything here wants to be ubuntu (chain
# home, go cache, docker group), so wrap the payload in a login shell for that
# user unless the caller opts out with SSM_RUN_AS=root.
wrap() {
  local body="$1"
  if [[ "$RUN_AS" == "root" ]]; then
    printf '%s' "$body"
  else
    printf 'sudo -H -u %s bash -lc %s' "$RUN_AS" "$(printf '%q' "$body")"
  fi
}

send() {
  local body="$1"
  local payload
  payload="$(wrap "$body")"
  # JSON-encode via python to survive quotes/newlines in the payload.
  local params
  params="$(SSM_PAYLOAD="$payload" python3 -c '
import json, os
print(json.dumps({"commands": [os.environ["SSM_PAYLOAD"]]}))')"
  aws_ssm send-command \
    --instance-ids "$INSTANCE" \
    --document-name AWS-RunShellScript \
    --parameters "$params" \
    --timeout-seconds 3600 \
    --query 'Command.CommandId' --output text
}

collect() {
  local cid="$1" waited=0
  while (( waited < TIMEOUT )); do
    local status
    status="$(aws_ssm get-command-invocation --command-id "$cid" \
      --instance-id "$INSTANCE" --query 'Status' --output text 2>/dev/null)"
    case "$status" in
      Success|Failed|Cancelled|TimedOut)
        aws_ssm get-command-invocation --command-id "$cid" \
          --instance-id "$INSTANCE" \
          --query 'StandardOutputContent' --output text
        local err
        err="$(aws_ssm get-command-invocation --command-id "$cid" \
          --instance-id "$INSTANCE" \
          --query 'StandardErrorContent' --output text)"
        [[ -n "$err" && "$err" != "None" ]] && printf '\n--- stderr ---\n%s\n' "$err"
        [[ "$status" == "Success" ]] && return 0
        printf '\n[ssm] status=%s\n' "$status"
        return 1
        ;;
      InProgress|Pending|Delayed) sleep 5; waited=$((waited+5)) ;;
      *) printf '[ssm] unexpected status: %s\n' "$status"; return 1 ;;
    esac
  done
  printf '[ssm] still running after %ss — use "bg" for long jobs\n' "$TIMEOUT"
  return 1
}

cmd="${1:-}"; shift || true

case "$cmd" in
  run)  collect "$(send "$1")" ;;
  runf) collect "$(send "$(cat "$1")")" ;;
  bg)
    tag="$1"; body="$2"
    # setsid detaches from the SSM worker so the job outlives the invocation.
    collect "$(send "rm -f ~/${tag}.done ~/${tag}.log
setsid bash -lc '{ ${body}
} > ~/${tag}.log 2>&1; echo \$? > ~/${tag}.done' >/dev/null 2>&1 &
echo LAUNCHED ${tag}")"
    ;;
  log)
    tag="$1"; n="${2:-60}"
    collect "$(send "tail -n ${n} ~/${tag}.log 2>/dev/null || echo '(no log yet)'
if [ -f ~/${tag}.done ]; then echo \"[done exit=\$(cat ~/${tag}.done)]\"; else echo '[still running]'; fi")"
    ;;
  wait)
    tag="$1"; max="${2:-30}"; waited=0
    while (( waited < max*60 )); do
      out="$(SSM_TIMEOUT=120 collect "$(send "if [ -f ~/${tag}.done ]; then echo \"DONE \$(cat ~/${tag}.done)\"; else tail -n 2 ~/${tag}.log 2>/dev/null | tr '\n' '|'; echo; fi")" 2>/dev/null)"
      if [[ "$out" == DONE* ]]; then echo "$out"; [[ "$out" == "DONE 0" ]] && return 0 || return 1; fi
      printf '[%s] %s\n' "$(date +%H:%M:%S)" "${out:-waiting}"
      sleep 30; waited=$((waited+30))
    done
    echo "[ssm] timed out after ${max}min"; return 1
    ;;
  *)
    sed -n '2,14p' "$0"; exit 1 ;;
esac
