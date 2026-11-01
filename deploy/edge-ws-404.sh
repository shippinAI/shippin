#!/bin/bash
# Installs the catch-all 404 page for unknown workspace/preview hosts (*.<ws base domain>) as
# the LAST route of the edge Caddy server, so every published workspace route (inserted first by
# shippin-hostd) still wins. Idempotent: the route (@id ws-notfound) is replaced on each run.
# Usage: edge-ws-404.sh <hostd.json> <console host>     (runs on the Incus host)
set -euo pipefail
CFG=${1:?hostd.json}; CONSOLE=${2:?console host}
DIR=$(cd "$(dirname "$0")" && pwd)
read -r EDGE PROJ SRV BASE ADMIN < <(python3 -c '
import json,sys; c=json.load(open(sys.argv[1])); e=c["edge"]
print(e["instance"], c["platform_project"], e["caddy_server"], e["ws_base_domain"], e["caddy_admin"])' "$CFG")
python3 - "$DIR/edge/ws-404.html" "$BASE" "$CONSOLE" > /tmp/ws-404-route.json <<'PY'
import json, sys
html = open(sys.argv[1]).read().replace("{$WS_BASE}", sys.argv[2]).replace("{$CONSOLE_HOST}", sys.argv[3])
print(json.dumps({"@id": "ws-notfound", "match": [{"host": ["*." + sys.argv[2]]}], "terminal": True,
  "handle": [{"handler": "static_response", "status_code": 404, "body": html,
              "headers": {"Content-Type": ["text/html; charset=utf-8"], "Cache-Control": ["no-store"], "X-Robots-Tag": ["noindex"]}}]}))
PY
incus exec "$EDGE" --project "$PROJ" -- curl -s -o /dev/null -X DELETE "$ADMIN/id/ws-notfound" </dev/null || true
incus exec "$EDGE" --project "$PROJ" -- curl -sf -X POST -H "Content-Type: application/json" --data-binary @- \
  "$ADMIN/config/apps/http/servers/$SRV/routes" < /tmp/ws-404-route.json
rm -f /tmp/ws-404-route.json
echo "ws-notfound installed for *.$BASE"
