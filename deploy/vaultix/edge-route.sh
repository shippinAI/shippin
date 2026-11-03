#!/bin/bash
# Add the Vaultix site to the existing Caddy edge (TLS by the edge; backup first, validate, reload).
# usage: edge-route.sh   env: DOMAIN, VAULTIX_HOST (default secrets.$DOMAIN), VAULTIX_UPSTREAM (ip:port), EDGE, EDGE_PROJECT
set -euo pipefail
: "${DOMAIN:?}" "${VAULTIX_UPSTREAM:?bridge ip:port of the Vaultix api}" "${EDGE:?edge instance}" "${EDGE_PROJECT:?}"
H=${VAULTIX_HOST:-secrets.$DOMAIN}; X() { incus exec "$EDGE" --project "$EDGE_PROJECT" -- "$@" </dev/null; }
if X grep -q "^$H " /etc/caddy/Caddyfile; then echo "route exists"; exit 0; fi
X sh -c "cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.bak-vaultix-\$(date +%Y%m%d-%H%M%S)"
X sh -c "printf '%s\n' '$H {' '	encode zstd gzip' '	header Strict-Transport-Security \"max-age=31536000\"' '	reverse_proxy $VAULTIX_UPSTREAM' '}' >> /etc/caddy/Caddyfile"
X caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null && X systemctl reload caddy && echo "reloaded: https://$H"
