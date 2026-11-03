#!/bin/bash
# Machine user "console-operator-read" (Shippin Platform org) with IAM_OWNER_VIEWER (read-only), so the provider
# panel can list users, IdPs and apps. PAT goes to sops console_idp_read_token. Never printed.
set -euo pipefail
S=/var/lib/shippin/site/secrets/platform-core.sops.yaml; PORG=${PLATFORM_ORG_ID:?}
py(){ python3 -c "import sys,json;d=json.load(sys.stdin);$1"; }
UID_=$(zapi POST /management/v1/users/_search '{"queries":[{"userNameQuery":{"userName":"console-operator-read","method":"TEXT_QUERY_METHOD_EQUALS"}}]}' $PORG | py "r=d.get('result',[]);print(r[0]['id'] if r else '')")
if [ -z "$UID_" ]; then
  UID_=$(zapi POST /management/v1/users/machine '{"userName":"console-operator-read","name":"Console operator read","description":"Read-only Nomen views for the provider panel (console)","accessTokenType":"ACCESS_TOKEN_TYPE_BEARER"}' $PORG | py "print(d['userId'])")
  zapi POST /admin/v1/members "{\"userId\":\"$UID_\",\"roles\":[\"IAM_OWNER_VIEWER\"]}" >/dev/null
  echo "created console-operator-read $UID_"
fi
EXP=$(date -u -d '+1 year' +%Y-%m-%dT%H:%M:%SZ)
cp $S $S.bak-$(date +%s)
zapi POST /management/v1/users/$UID_/pats "{\"expirationDate\":\"$EXP\"}" $PORG | py "print(json.dumps(d['token']),end='')" | sops set --value-stdin $S '["console_idp_read_token"]'
echo "PAT stored in sops console_idp_read_token (expires $EXP)"
