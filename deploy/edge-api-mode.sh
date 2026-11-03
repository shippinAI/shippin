#!/bin/bash
# Switch the edge Caddy from Caddyfile mode to API mode (caddy-api.service, --resume), so
# shippin-hostd can add/remove workspace routes through the local admin API and they persist.
# The Caddyfile is kept as the documented base; it is adapted and loaded once.
set -euo pipefail
P="--project platform"
incus exec edge $P -- bash -euc '
if systemctl is-active -q caddy-api; then echo "already in API mode"; exit 0; fi
cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.bak-$(date +%Y%m%d-%H%M%S)
caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile > /etc/caddy/base.json 2>/dev/null
curl -sf -X POST -H "Content-Type: application/json" --data-binary @/etc/caddy/base.json http://localhost:2019/load
sleep 1; ls -la ~caddy/.config/caddy/autosave.json
systemctl disable -q --now caddy && systemctl enable -q --now caddy-api
sleep 2; systemctl is-active caddy-api; curl -s localhost:2019/config/apps/http/servers | head -c 200; echo
' </dev/null
