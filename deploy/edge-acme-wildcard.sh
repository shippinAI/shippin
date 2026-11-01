#!/bin/bash
# Real Let's Encrypt certs on the edge via ACME DNS-01 (Cloudflare) for the named platform hosts
# and *.<ws base domain>. Usage on Gold: ./edge-acme-wildcard.sh /path/to/caddy-with-cloudflare
# Token source of truth: sops key shippin_cloud_dns_token. It is written root-only (0600) to the edge
# as an EnvironmentFile for caddy-api.service. Never printed. API mode is kept (config via :2019).
set -euo pipefail
S=/var/lib/shippin/site/secrets/platform-core.sops.yaml; P="--project platform"; BIN=${1:?caddy binary}
D=${DOMAIN:?set DOMAIN}; WS=${WS_DOMAIN:-ws.$D}; EMAIL=${ACME_EMAIL:-ops@$D}
HOSTS=${HOSTS:-console id secrets edge}
if ! sops decrypt --extract '["shippin_cloud_dns_token"]' $S >/dev/null 2>&1; then
  echo "BLOCKED: no shippin_cloud_dns_token in sops. Keeping internal CA."; exit 2
fi
"$BIN" list-modules | grep -q '^dns.providers.cloudflare$' || { echo "binary lacks dns.providers.cloudflare"; exit 2; }
TS=$(date +%Y%m%d-%H%M%S)
incus exec edge $P -- sh -c "cp /usr/bin/caddy /usr/bin/caddy.bak-$TS; cp ~caddy/.config/caddy/autosave.json /etc/caddy/autosave.bak-$TS.json" </dev/null
( umask 077; printf 'CF_API_TOKEN=%s\n' "$(sops decrypt --extract '["shippin_cloud_dns_token"]' $S)" > /var/lib/shippin/site/.cf.env )
incus file push /var/lib/shippin/site/.cf.env edge/etc/caddy/cloudflare.env $P --mode 0600 --uid 0 --gid 0 -q
shred -u /var/lib/shippin/site/.cf.env
incus exec edge $P -- sh -c 'mkdir -p /etc/systemd/system/caddy-api.service.d && printf "[Service]\nEnvironmentFile=/etc/caddy/cloudflare.env\n" > /etc/systemd/system/caddy-api.service.d/cloudflare.conf && systemctl daemon-reload' </dev/null
incus exec edge $P -- systemctl stop caddy-api </dev/null
incus file push "$BIN" edge/usr/bin/caddy $P --mode 0755 -q
incus exec edge $P -- sh -c 'setcap cap_net_bind_service=+ep /usr/bin/caddy 2>/dev/null; systemctl start caddy-api; sleep 2; systemctl is-active caddy-api' </dev/null
subj=$(printf '"%s.'"$D"'",' $HOSTS)"\"*.$WS\""
pol='{"subjects":['"$subj"'],"issuers":[{"module":"acme","email":"'"$EMAIL"'","challenges":{"dns":{"provider":{"name":"cloudflare","api_token":"{env.CF_API_TOKEN}"},"resolvers":["1.1.1.1"]}}}]}'
incus exec edge $P -- sh -c "
curl -sf localhost:2019/config/apps/tls/automation/policies >/dev/null || curl -sf -X POST -H 'Content-Type: application/json' localhost:2019/config/apps/tls/automation/policies -d '[]'
curl -sf -X PUT -H 'Content-Type: application/json' localhost:2019/config/apps/tls/automation/policies/0 -d '$pol' && echo policy-ok" </dev/null
