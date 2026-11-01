#!/bin/bash
# Set console-preview's role in a tenant org (e.g. ORG_OWNER briefly for workspace-IDE screenshots,
# then back to ORG_OWNER_VIEWER). Usage: TENANT_ORG_ID=... ./preview-role.sh ROLE
set -euo pipefail
S=/var/lib/shippin/site/secrets/platform-core.sops.yaml; P="--project platform"
D=${DOMAIN:?set DOMAIN}; ORG=${TENANT_ORG_ID:?}; ROLE=${1:?role}
T=$(sops decrypt --extract '["zitadel_control_plane_pat"]' $S)
api(){ incus exec edge $P --env T="$T" -- sh -c 'curl -sk --resolve id.'"$D"':443:127.0.0.1 -H "Authorization: Bearer $T" -H "x-nomen-orgid: '"$ORG"'" -H "Content-Type: application/json" "$@"' _ "$@" </dev/null; }
UID_=$(api -X POST https://id.$D/management/v1/users/_search -d '{"queries":[{"userNameQuery":{"userName":"console-preview","method":"TEXT_QUERY_METHOD_EQUALS"}}]}' | python3 -c "import sys,json;print(json.load(sys.stdin)['result'][0]['id'])")
api -X PUT https://id.$D/management/v1/orgs/me/members/$UID_ -d "{\"roles\":[\"$ROLE\"]}" | head -c 120; echo
unset T
