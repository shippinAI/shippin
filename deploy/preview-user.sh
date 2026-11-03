#!/bin/bash
# Create machine user "console-preview" in a tenant org (read-only roles) and mint a short-lived PAT
# into $OUT (mode 600) for headless screenshots. Never prints the PAT.
set -euo pipefail
S=/var/lib/shippin/site/secrets/platform-core.sops.yaml; P="--project platform"
D=${DOMAIN:?set DOMAIN}; ORG=${TENANT_ORG_ID:?}; OUT=${OUT:-/var/lib/shippin/site/.preview-pat}
T=$(sops decrypt --extract '["zitadel_control_plane_pat"]' $S)
api(){ incus exec edge $P --env T="$T" -- sh -c 'curl -sk --resolve id.'"$D"':443:127.0.0.1 -H "Authorization: Bearer $T" -H "x-nomen-orgid: '"$ORG"'" -H "Content-Type: application/json" "$@"' _ "$@" </dev/null; }
py(){ python3 -c "import sys,json;d=json.load(sys.stdin);$1"; }
UID_=$(api -X POST https://id.$D/management/v1/users/_search -d '{"queries":[{"userNameQuery":{"userName":"console-preview","method":"TEXT_QUERY_METHOD_EQUALS"}}]}' | py "r=d.get('result',[]);print(r[0]['id'] if r else '')")
if [ -z "$UID_" ]; then
  UID_=$(api -X POST https://id.$D/management/v1/users/machine -d '{"userName":"console-preview","name":"Console Preview","description":"Headless screenshots of the console (read-only)","accessTokenType":"ACCESS_TOKEN_TYPE_BEARER"}' | py "print(d['userId'])")
  api -X POST https://id.$D/management/v1/orgs/me/members -d "{\"userId\":\"$UID_\",\"roles\":[\"ORG_OWNER_VIEWER\"]}" >/dev/null
  api -X POST https://id.$D/admin/v1/members -d "{\"userId\":\"$UID_\",\"roles\":[\"IAM_OWNER_VIEWER\"]}" >/dev/null
  echo "created console-preview $UID_"
fi
EXP=$(date -u -d '+1 day' +%Y-%m-%dT%H:%M:%SZ)
umask 077
api -X POST https://id.$D/management/v1/users/$UID_/pats -d "{\"expirationDate\":\"$EXP\"}" | py "open('$OUT','w').write(d['token'])"
unset T; echo "PAT written to $OUT (expires $EXP)"
