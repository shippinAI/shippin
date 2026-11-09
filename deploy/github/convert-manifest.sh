#!/bin/bash
# Exchanges the one-time manifest code (valid 1 hour, single use) for the new App's credentials and
# stores them in sops as github_app_id, github_client_id, github_client_secret, github_private_key
# and github_webhook_secret. Prints only the App id, slug and owner, never a secret.
# Usage on the host: ./convert-manifest.sh <code>   (SECRETS_FILE overrides the sops file)
set -euo pipefail
CODE=${1:?code from the redirect URL}; S=${SECRETS_FILE:-/var/lib/shippin/site/secrets/platform-core.sops.yaml}
API=${GITHUB_API:-https://api.github.com}
[[ $CODE =~ ^[A-Za-z0-9_-]{8,128}$ ]] || { echo "code looks wrong" >&2; exit 2; }
T=$(mktemp); trap 'shred -u "$T" 2>/dev/null || rm -f "$T"' EXIT; chmod 600 "$T"
curl -fsS -X POST -H "Accept: application/vnd.github+json" "$API/app-manifests/$CODE/conversions" -o "$T" \
  || { echo "GitHub refused the code (expired or already used?)" >&2; exit 1; }
for k in id:app_id client_id:client_id client_secret:client_secret pem:private_key webhook_secret:webhook_secret; do
  v=$(jq -e ".${k%%:*} | tostring" "$T") || { echo "response has no ${k%%:*}" >&2; exit 1; }
  sops set "$S" "[\"github_${k#*:}\"]" "$v"
done
jq -r '"created App id \(.id), slug \(.slug), owner \(.owner.login); set github.app_slug to \(.slug)"' "$T"
