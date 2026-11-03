#!/bin/bash
# jobs-resume.sh TOKENFILE TENANT - live check of durable jobs (needs jobs.enabled=true).
# Creates a throwaway workspace with an Idempotency-Key, checks a replay returns the same job,
# restarts the console mid-provisioning (RESTART_CMD), then waits for the job to succeed and
# deletes the workspace. The token is read from TOKENFILE and never printed.
# Env: CONSOLE_URL (https://console host), RESTART_CMD (e.g. "incus restart console"),
#      WAIT_SECONDS (default 600).
set -euo pipefail
TF=${1:?token file}; T=${2:?tenant id}
URL=${CONSOLE_URL:?CONSOLE_URL}; RESTART=${RESTART_CMD:?RESTART_CMD}; WAIT=${WAIT_SECONDS:-600}
api() { # api METHOD PATH [BODY] [KEY]
  local h=(-H "Authorization: Bearer $(cat "$TF")" -H "Content-Type: application/json")
  [ -n "${4:-}" ] && h+=(-H "Idempotency-Key: $4")
  curl -fsS -X "$1" "${h[@]}" ${3:+--data "$3"} "$URL$2"
}
KEY=$(cat /proc/sys/kernel/random/uuid)
BODY=$(jq -nc --arg n "jobs-resume-$(date +%s)" '{name:$n}')
first=$(api POST "/v1/tenants/$T/workspaces" "$BODY" "$KEY")
again=$(api POST "/v1/tenants/$T/workspaces" "$BODY" "$KEY")
WS=$(jq -r .id <<<"$first"); JOB=$(jq -r '.job.id // empty' <<<"$first")
[ -n "$JOB" ] || { echo "FAIL: no job in the 202 (is jobs.enabled true?)"; exit 1; }
[ "$(jq -r .job.id <<<"$again")" = "$JOB" ] || { echo "FAIL: replay returned another job"; exit 1; }
echo "workspace $WS job $JOB; replay OK"
sleep 5
echo "restarting the console mid-job"
sh -c "$RESTART"
deadline=$((SECONDS + WAIT)); status=""
until [ $SECONDS -ge $deadline ]; do
  status=$(api GET "/v1/jobs/$JOB" 2>/dev/null | jq -r .status || true)
  case $status in succeeded|failed|canceled) break ;; esac
  sleep 5
done
echo "job status: ${status:-unknown}"
api POST "/v1/tenants/$T/workspaces/$WS/actions/delete" >/dev/null || echo "WARN: delete $WS by hand"
[ "$status" = succeeded ] || { echo "FAIL: job did not survive the restart"; exit 1; }
echo "jobs-resume: OK"
