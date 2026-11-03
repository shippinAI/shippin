#!/bin/bash
# Live checks of customer-granted platform access against a THROWAWAY tenant (never a real one).
# Creates access requests/grants and audit entries on that tenant; changes no workspace.
#   ADMIN_PAT_FILE  platform admin PAT (not a member of TENANT_ID)
#   OWNER_PAT_FILE  PAT of an owner of TENANT_ID
#   TENANT_ID, WS_ID, WS_HOST (a running workspace of that tenant), EDGE_IP, CONSOLE_HOST
#   EXPIRY=<dur>    also test expiry with this (configured) duration, e.g. 1m
set -uo pipefail
: "${ADMIN_PAT_FILE:?}" "${OWNER_PAT_FILE:?}" "${TENANT_ID:?}" "${WS_ID:?}" "${WS_HOST:?}" "${EDGE_IP:?}" "${CONSOLE_HOST:?}"
pass=0; failn=0; A=$ADMIN_PAT_FILE; O=$OWNER_PAT_FILE; T=/v1/tenants/$TENANT_ID; J=/tmp/al.jar
ok() { pass=$((pass + 1)); printf '  ok   %s\n' "$1"; }
bad() { failn=$((failn + 1)); printf '  FAIL %s (got %s)\n' "$1" "$2"; }
req() { # req TOKENFILE METHOD PATH [BODY] -> status; body in /tmp/al.body
  curl -s -o /tmp/al.body -w '%{http_code}' --resolve "$CONSOLE_HOST:443:$EDGE_IP" -H "Authorization: Bearer $(cat "$1")" \
    -H 'Content-Type: application/json' -X "$2" ${4:+--data "$4"} "https://$CONSOLE_HOST$3"
}
expect() { local want=$1 desc=$2; shift 2; local got; got=$(req "$@"); [[ "$got" =~ ^($want)$ ]] && ok "$desc" || bad "$desc" "$got"; }
field() { python3 -c "import json;d=json.load(open('/tmp/al.body'));print($1)" 2>/dev/null; }
# ide TOKENFILE -> final status of opening the IDE through ws-login, the one-time code and forward_auth
ide() {
  rm -f $J
  local loc; loc=$(curl -s -o /dev/null -w '%{redirect_url}' --resolve "$CONSOLE_HOST:443:$EDGE_IP" -H "Authorization: Bearer $(cat "$1")" \
    "https://$CONSOLE_HOST/auth/ws-login?rd=https%3A%2F%2F$WS_HOST%2F")
  [[ "$loc" == https://$WS_HOST/_shippin/auth* ]] || { echo 403; return; }
  curl -s -o /dev/null -c $J --resolve "$WS_HOST:443:$EDGE_IP" "$loc"
  again
}
again() { curl -s -o /dev/null -w '%{http_code}' -b $J --resolve "$WS_HOST:443:$EDGE_IP" "https://$WS_HOST/"; }
audit_has() { req "$O" GET "$T/audit" >/dev/null; [ "$(field "sum(1 for e in d['items'] if e['action']=='$1' and e.get('grant_id')=='$2')")" -ge 1 ] 2>/dev/null; }
grant() { # grant SCOPE_WS DURATION -> id (request)
  req "$A" POST /v1/operator/access-requests "{\"tenant_id\":\"$TENANT_ID\",\"workspace_id\":\"$1\",\"reason\":\"Live access test: $3\",\"duration\":\"$2\"}" >/dev/null
  field "d['id']"
}

echo "== without a grant the admin sees metadata only"
expect 404 "tenant workspaces hidden" "$A" GET "$T/workspaces"
expect 404 "tenant settings hidden" "$A" GET "$T/settings"
expect 404 "customer access list hidden" "$A" GET "$T/access"
expect 403 "provider workspace action refused" "$A" POST "/v1/operator/workspaces/$WS_ID/stop"
[ "$(ide "$A")" = 403 ] && ok "IDE refused at ws-login" || bad "IDE refused" "allowed"
expect 200 "operator tenant list still works" "$A" GET /v1/operator/tenants
expect 200 "operator workspace list still works" "$A" GET /v1/operator/workspaces
expect 428 "break-glass with a PAT needs a fresh sign-in" "$A" POST /v1/operator/break-glass \
  "{\"tenant_id\":\"$TENANT_ID\",\"category\":\"security_incident\",\"justification\":\"Live test: PAT must not break glass\"}"
expect 400 "duration outside the configured choices" "$A" POST /v1/operator/access-requests "{\"tenant_id\":\"$TENANT_ID\",\"reason\":\"Live access test\",\"duration\":\"999h\"}"
expect 400 "reason too short" "$A" POST /v1/operator/access-requests "{\"tenant_id\":\"$TENANT_ID\",\"reason\":\"x\",\"duration\":\"1h\"}"
expect 400 "unknown fields refused" "$A" POST /v1/operator/access-requests "{\"tenant_id\":\"$TENANT_ID\",\"reason\":\"Live access test\",\"duration\":\"1h\",\"status\":\"approved\"}"
expect 404 "unknown tenant" "$A" POST /v1/operator/access-requests '{"tenant_id":"00000000-0000-7000-8000-000000000000","reason":"Live access test","duration":"1h"}'

echo "== request -> deny -> 403"
R=$(grant "" 1h deny)
[ -n "$R" ] && ok "request created ($R)" || bad "request created" "none"
expect 409 "duplicate open request refused" "$A" POST /v1/operator/access-requests "{\"tenant_id\":\"$TENANT_ID\",\"reason\":\"Live access test dup\",\"duration\":\"1h\"}"
req "$O" GET "$T/access" >/dev/null; [ "$(field "[g['status'] for g in d['items'] if g['id']=='$R'][0]")" = pending ] && ok "customer sees it pending" || bad "pending" "?"
expect 200 "customer denies" "$O" POST "$T/access/$R/deny"
expect 409 "second decision refused" "$O" POST "$T/access/$R/approve"
[ "$(ide "$A")" = 403 ] && ok "IDE still refused after deny" || bad "IDE after deny" "allowed"
audit_has access.request "$R" && audit_has access.deny "$R" && ok "request + deny audited" || bad "audit request/deny" "missing"

echo "== request -> approve -> IDE 200 (audited) -> revoke -> 403"
R=$(grant "" 1h approve)
expect "403|404" "admin can't approve their own request" "$A" POST "$T/access/$R/approve"
expect 200 "customer approves" "$O" POST "$T/access/$R/approve"
expect 200 "admin now sees the tenant's workspaces" "$A" GET "$T/workspaces"
expect 403 "grant holder can't create workspaces (SSH keys would outlive the grant)" "$A" POST "$T/workspaces" '{"name":"nope"}'
expect 403 "grant holder can't decide access" "$A" GET "$T/access"
[ "$(ide "$A")" = 200 ] && ok "IDE opens (200)" || bad "IDE opens" "$(again)"
sleep 1; audit_has access.ide "$R" && ok "IDE open audited with grant" || bad "IDE audit" "missing"
req "$O" GET "$T/audit" >/dev/null; [ -n "$(field "[e['ip'] for e in d['items'] if e['action']=='access.ide'][0]")" ] && ok "audit entry carries the client IP" || bad "audit ip" "empty"
expect 200 "customer revokes" "$O" POST "$T/access/$R/revoke"
c=$(again); [ "$c" = 302 ] && ok "existing IDE cookie refused at forward_auth after revoke ($c)" || bad "cookie after revoke" "$c"
[ "$(ide "$A")" = 403 ] && ok "IDE refused after revoke" || bad "IDE after revoke" "allowed"
expect 404 "tenant hidden again" "$A" GET "$T/workspaces"
audit_has access.revoke "$R" && ok "revoke audited" || bad "revoke audit" "missing"

echo "== workspace-scoped grant, ended early by the admin"
R=$(grant "$WS_ID" 1h scoped)
expect 200 "customer approves scoped request" "$O" POST "$T/access/$R/approve"
req "$A" GET "$T/workspaces" >/dev/null; [ "$(field "[w['id'] for w in d['items']]")" = "['$WS_ID']" ] && ok "admin lists only that workspace" || bad "scoped list" "$(field "len(d['items'])")"
expect 200 "admin ends it early" "$A" POST "/v1/operator/access-requests/$R/end"
expect 409 "customer can't revoke an ended grant" "$O" POST "$T/access/$R/revoke"
audit_has access.end "$R" && ok "end audited" || bad "end audit" "missing"

if [ -n "${EXPIRY:-}" ]; then
  echo "== expiry ($EXPIRY)"
  R=$(grant "" "$EXPIRY" expiry)
  expect 200 "customer approves short grant" "$O" POST "$T/access/$R/approve"
  [ "$(ide "$A")" = 200 ] && ok "IDE opens during the grant" || bad "IDE during grant" "refused"
  secs=$(python3 -c "import re;m=re.match(r'(\d+)([ms])','$EXPIRY');print(int(m[1])*(60 if m[2]=='m' else 1)+5)")
  sleep "$secs"
  c=$(again); [ "$c" = 302 ] && ok "cookie refused right after expiry" || bad "cookie after expiry" "$c"
  [ "$(ide "$A")" = 403 ] && ok "IDE refused after expiry" || bad "IDE after expiry" "allowed"
  sleep 35; audit_has access.expire "$R" && ok "sweeper audited the expiry" || bad "expire audit" "missing"
  req "$O" GET "$T/access" >/dev/null; [ "$(field "[g['status'] for g in d['items'] if g['id']=='$R'][0]")" = expired ] && ok "status expired" || bad "status" "?"
fi
rm -f $J /tmp/al.body
echo "passed $pass, failed $failn"; [ "$failn" = 0 ]
