#!/bin/bash
# Add the console.<domain> site to the edge Caddyfile (backup first, validate, reload).
set -euo pipefail
P="--project platform"; D=${DOMAIN:?set DOMAIN}; IP=${CONSOLE_IP:?set CONSOLE_IP}
if incus exec edge $P -- grep -q "^console\.$D" /etc/caddy/Caddyfile </dev/null; then echo "route exists"; exit 0; fi
incus exec edge $P -- sh -c "cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.bak-\$(date +%Y%m%d-%H%M%S) && printf '%s\n' 'console.$D {' '	tls internal' '	encode zstd gzip' '	reverse_proxy $IP:8080' '}' >> /etc/caddy/Caddyfile && caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null && systemctl reload caddy && echo reloaded" </dev/null
