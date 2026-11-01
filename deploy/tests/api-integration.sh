#!/bin/bash
# Non-destructive integration checks against a live console (run on the Incus host or anywhere
# that can reach the edge). Creates, changes and deletes nothing: only reads, requests that must
# be refused (validation / authz), and requests aimed at ids that don't exist.
#
#   USER_PAT_FILE   PAT of a signed-in non-admin user (e.g. console-preview as IAM_OWNER_VIEWER)
#   ADMIN_PAT_FILE  optional: PAT of a platform admin (enables the provider-panel checks)
#   ONLY_ADMIN=1    USER_PAT_FILE is the admin too: skip the non-admin refusal checks
#   TEST_WS_ID      optional: a throwaway workspace in TENANT_ID for refused-write checks
#   EDGE_IP, CONSOLE_HOST, WS_BASE, TENANT_ID (a tenant the user can see), WS_HOST (a live workspace host)
set -uo pipefail
: "${USER_PAT_FILE:?}" "${EDGE_IP:?}" "${CONSOLE_HOST:?}" "${WS_BASE:?}" "${TENANT_ID:?}" "${WS_HOST:?}"
pass=0; failn=0
NOID=00000000-0000-7000-8000-000000000000
ok() { pass=$((pass + 1)); printf '  ok   %s\n' "$1"; }
bad() { failn=$((failn + 1)); printf '  FAIL %s (got %s)\n' "$1" "$2"; }

# req TOKENFILE METHOD PATH [BODY] -> prints HTTP status
req() {
  local tf=$1 m=$2 p=$3 body=${4:-} auth=()
  [ -n "$tf" ] && auth=(-H "Authorization: Bearer $(cat "$tf")")
  curl -s -o /tmp/it.body -w '%{http_code}' --resolve "$CONSOLE_HOST:443:$EDGE_IP" "${auth[@]}" \
    -H 'Content-Type: application/json' -X "$m" ${body:+--data "$body"} "https://$CONSOLE_HOST$p"
}
expect() { # expect WANT DESC TOKENFILE METHOD PATH [BODY]
  local want=$1 desc=$2; shift 2
  local got; got=$(req "$@")
  [[ "$got" =~ ^($want)$ ]] && ok "$desc" || bad "$desc" "$got"
}
host() { # host WANT DESC HOSTNAME
  local got; got=$(curl -s -o /tmp/it.body -w '%{http_code}' --resolve "$3:443:$EDGE_IP" "https://$3/")
  [[ "$got" =~ ^($1)$ ]] && ok "$2" || bad "$2" "$got"
}
U=$USER_PAT_FILE; T=/v1/tenants/$TENANT_ID

echo "== health and auth"
expect 200 "health is public" "" GET /healthz
expect 401 "API refuses anonymous callers" "" GET /v1/me
expect 401 "templates need sign-in" "" GET /v1/templates
expect 401 "operator panel refuses anonymous callers" "" GET /v1/operator/tenants
expect 200 "signed-in user sees /v1/me" "$U" GET /v1/me

if [ -z "${ONLY_ADMIN:-}" ]; then # USER_PAT_FILE is a non-admin
echo "== provider panel is platform-admin only"
for p in "GET /v1/operator/tenants" "GET /v1/operator/workspaces" "GET /v1/operator/templates" "GET /v1/operator/host" \
  "GET /v1/operator/edge" "GET /v1/operator/identity" "POST /v1/operator/templates" "DELETE /v1/operator/tenants/$NOID" \
  "POST /v1/operator/workspaces/$NOID/delete" "POST /v1/operator/tenants/$NOID/suspend"; do
  expect 403 "non-admin: $p" "$U" ${p% *} ${p#* } '{}'
done

fi
echo "== customer reads"
expect 200 "template gallery" "$U" GET /v1/templates
grep -q '"repo_hosts"' /tmp/it.body && ok "gallery publishes repo rule" || bad "gallery publishes repo rule" "missing"
grep -q '"startup":"[^"]' /tmp/it.body && bad "gallery hides startup scripts" "present" || ok "gallery hides startup scripts"
expect 200 "workspace list" "$U" GET "$T/workspaces"
expect 404 "unknown tenant" "$U" GET "/v1/tenants/$NOID/workspaces"
if [ -z "${ONLY_ADMIN:-}" ]; then
expect 403 "viewer can't create workspaces" "$U" POST "$T/workspaces" '{"name":"itest"}'
expect 403 "viewer can't run actions" "$U" POST "$T/workspaces/$NOID/actions/stop"
expect '403|404' "viewer can't change firewall" "$U" PUT "$T/workspaces/$NOID/firewall" '{"rules":[]}'
expect 404 "previews on unknown workspace" "$U" PUT "$T/workspaces/$NOID/previews" '{"previews":[]}'
fi

echo "== edge"
host 404 "unknown workspace host is a real 404" "nothing-000000.$WS_BASE"
grep -q "no workspace here" /tmp/it.body && ok "404 page is the branded page" || bad "404 page is the branded page" "body"
host 404 "unknown preview host is a real 404" "5173--nothing-000000.$WS_BASE"
host 302 "workspace IDE needs sign-in" "$WS_HOST"
grep -q "ws-login" /tmp/it.body && ok "redirects to console sign-in" || bad "redirects to console sign-in" "body"

if [ -n "${ADMIN_PAT_FILE:-}" ]; then
  A=$ADMIN_PAT_FILE
  echo "== provider panel (admin, read + refused writes only)"
  for p in tenants workspaces templates host edge identity access; do expect 200 "admin GET /v1/operator/$p" "$A" GET "/v1/operator/$p"; done
  expect 400 "template: foreign image refused" "$A" POST /v1/operator/templates '{"slug":"it-x","name":"x","image":"docker.io/x","cpu":2,"memory_gib":4,"disk_gib":20}'
  expect 400 "template: bad repo refused" "$A" POST /v1/operator/templates '{"slug":"it-x","name":"x","image":"shippin/tpl-blank","cpu":2,"memory_gib":4,"disk_gib":20,"repo":"https://evil.example/a/b"}'
  expect 400 "template: duplicate ports refused" "$A" POST /v1/operator/templates '{"slug":"it-x","name":"x","image":"shippin/tpl-blank","cpu":2,"memory_gib":4,"disk_gib":20,"ports":[5173,5173]}'
  expect 404 "template: PUT to unknown id is not an upsert" "$A" PUT "/v1/operator/templates/$NOID" '{"slug":"it-x","name":"x","image":"shippin/tpl-blank","cpu":2,"memory_gib":4,"disk_gib":20}'
  expect 404 "template: delete unknown" "$A" DELETE "/v1/operator/templates/$NOID"
  expect 400 "workspace action: unknown action" "$A" POST "/v1/operator/workspaces/$NOID/rebuild"
  expect 404 "workspace delete: unknown id" "$A" POST "/v1/operator/workspaces/$NOID/delete" '{"confirm":"x"}'
  expect 404 "tenant delete: unknown id" "$A" DELETE "/v1/operator/tenants/$NOID" '{"confirm":"x"}'
  if [ -n "${TEST_WS_ID:-}" ]; then # a throwaway workspace; every request below is refused before any change
    W=$T/workspaces/$TEST_WS_ID
    expect 400 "firewall: edge port 443 refused" "$A" PUT "$W/firewall" '{"rules":[{"protocol":"tcp","port":443}]}'
    expect 400 "firewall: SSH range refused" "$A" PUT "$W/firewall" '{"rules":[{"protocol":"tcp","port":22001}]}'
    expect 400 "firewall: IPv6 source refused" "$A" PUT "$W/firewall" '{"rules":[{"protocol":"tcp","port":8080,"source":"2001:db8::/32"}]}'
    expect 400 "domains: platform name refused" "$A" PUT "$W/domains" "{\"domains\":[\"$CONSOLE_HOST\"]}"
    expect 400 "previews: duplicate port refused" "$A" PUT "$W/previews" '{"previews":[{"port":5173},{"port":5173}]}'
    expect 400 "previews: IDE port refused" "$A" PUT "$W/previews" '{"previews":[{"port":3000}]}'
    expect 400 "provider delete needs the typed name" "$A" POST "/v1/operator/workspaces/$TEST_WS_ID/delete" '{"confirm":"wrong"}'
  fi
  expect 400 "create: bad repo refused" "$A" POST "$T/workspaces" '{"name":"itest","template_id":"node-vite","repo":"https://github.com/a/.."}'
  expect 400 "create: bad name refused" "$A" POST "$T/workspaces" '{"name":"Bad Name"}'
fi
rm -f /tmp/it.body
echo "== $pass passed, $failn failed"
[ "$failn" = 0 ]
