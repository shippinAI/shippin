#!/bin/bash
# Route the operator app's origin (ops.<domain>, CONSOLE-IA) to the console on the edge, through
# the Caddy admin API (API mode, persisted by --resume), and add the host to the ACME DNS-01
# policy. Idempotent; the live config is saved first. Runs on Gold. Prints no secrets.
#   bash deploy/edge-ops-route.sh      env: DOMAIN (required) OPS_HOST (ops.$DOMAIN) CONSOLE_UPSTREAM (203.0.113.50:8080)
set -euo pipefail
D=${DOMAIN:?set DOMAIN}; H=${OPS_HOST:-ops.$D}; U=${CONSOLE_UPSTREAM:?set CONSOLE_UPSTREAM}
[[ "$H" =~ ^[a-z0-9.-]+$ && "$U" =~ ^[0-9.]+:[0-9]+$ ]] || { echo "bad OPS_HOST/CONSOLE_UPSTREAM" >&2; exit 1; }
E() { incus exec -T edge --project platform -- "$@" </dev/null; }
A=http://localhost:2019
TS=$(date +%Y%m%d-%H%M%S)
E sh -c "curl -sf $A/config/ > /etc/caddy/autosave.bak-$TS-ops.json && echo saved /etc/caddy/autosave.bak-$TS-ops.json"
ROUTE=$(printf '{"@id":"ops-panel","match":[{"host":["%s"]}],"terminal":true,"handle":[{"handler":"subroute","routes":[{"handle":[{"handler":"encode","encodings":{"gzip":{},"zstd":{}},"prefer":["zstd","gzip"]},{"handler":"reverse_proxy","upstreams":[{"dial":"%s"}]}]}]}]}' "$H" "$U")
if E curl -sf "$A/id/ops-panel" >/dev/null 2>&1; then
  echo "route ops-panel exists"
else
  E curl -sf -X PUT -H "Content-Type: application/json" -d "$ROUTE" "$A/config/apps/http/servers/srv0/routes/0" && echo "route ops-panel added for $H"
fi
if E curl -sf "$A/config/apps/tls/automation/policies/0/subjects" | grep -q "\"$H\""; then
  echo "tls subject $H exists"
else
  E curl -sf -X POST -H "Content-Type: application/json" -d "\"$H\"" "$A/config/apps/tls/automation/policies/0/subjects" && echo "tls subject $H added"
fi
E curl -sf "$A/id/ops-panel" | head -c 300; echo
