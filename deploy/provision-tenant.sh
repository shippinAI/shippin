#!/bin/bash
# Provision a tenant for an existing Nomen org through the console API (POST /v1/tenants),
# authenticated as the control-plane operator. The PAT never leaves Gold and is not printed.
set -euo pipefail
S=/var/lib/shippin/site/secrets/platform-core.sops.yaml; P="--project platform"; D=${DOMAIN:?set DOMAIN}
ORG=${ORG_ID:?}; NAME=${TENANT_NAME:?}; PLAN=${PLAN:-founder}
T=$(sops decrypt --extract '["zitadel_control_plane_pat"]' $S)
incus exec edge $P --env T="$T" -- sh -c 'curl -sk --max-time 300 --resolve console.'"$D"':443:127.0.0.1 -H "Authorization: Bearer $T" -H "Content-Type: application/json" -X POST https://console.'"$D"'/v1/tenants -d "$1"' _ \
  "{\"idp_org_id\":\"$ORG\",\"name\":\"$NAME\",\"plan_id\":\"$PLAN\"}" </dev/null
unset T; echo
