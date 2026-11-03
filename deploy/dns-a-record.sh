#!/bin/bash
# Create one DNS-only A record <name>.<domain> -> PUBLIC_IP in the Cloudflare zone, if no record of
# that name exists (never modifies or deletes). Exports the zone first. Runs on Gold; the zone
# token comes from sops (shippin_cloud_dns_token) and is never printed.
#   PUBLIC_IP=<edge public IP> bash deploy/dns-a-record.sh ops     env: DOMAIN (shippin.cloud) BACKUP_DIR
set -euo pipefail
N=${1:?record name, e.g. ops}; D=${DOMAIN:?set DOMAIN}; IP=${PUBLIC_IP:?}
S=${SECRETS_FILE:-/var/lib/shippin/site/secrets/platform-core.sops.yaml}; B=${BACKUP_DIR:-/var/lib/shippin/changes/snapshots}
[[ "$N" =~ ^[a-z0-9-]+$ && "$IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "bad name/PUBLIC_IP" >&2; exit 1; }
H=$(mktemp); trap 'shred -u "$H"' EXIT; chmod 600 "$H"
printf 'Authorization: Bearer %s\n' "$(sops decrypt --extract '["shippin_cloud_dns_token"]' "$S")" > "$H"
cf() { curl -sf -H @"$H" -H "Content-Type: application/json" "$@"; }
API=https://api.cloudflare.com/client/v4
Z=$(cf "$API/zones?name=$D" | python3 -c 'import sys,json; r=json.load(sys.stdin)["result"]; print(r[0]["id"] if r else "")')
[ -n "$Z" ] || { echo "zone $D not visible to the token" >&2; exit 1; }
F=$B/dns-$D-$(date +%Y%m%d-%H%M%S).bind; cf "$API/zones/$Z/dns_records/export" > "$F"; chmod 600 "$F"; echo "zone exported to $F ($(wc -l < "$F") lines)"
if [ "$(cf "$API/zones/$Z/dns_records?name=$N.$D" | python3 -c 'import sys,json; print(len(json.load(sys.stdin)["result"]))')" != 0 ]; then
  echo "a record named $N.$D exists; left alone"; exit 0
fi
cf -X POST "$API/zones/$Z/dns_records" -d "{\"type\":\"A\",\"name\":\"$N.$D\",\"content\":\"$IP\",\"ttl\":300,\"proxied\":false}" |
  python3 -c 'import sys,json; r=json.load(sys.stdin)["result"]; print("created", r["type"], r["name"], "->", r["content"], "ttl", r["ttl"], "proxied", r["proxied"])'
