#!/bin/bash
# Machine user "console-onboarding" (Shippin Platform org) with IAM_ORG_MANAGER, so the console can
# create an org for a self-registered user. PAT goes to sops console_idp_service_token. Never printed.
set -euo pipefail
S=/var/lib/shippin/site/secrets/platform-core.sops.yaml; PORG=${PLATFORM_ORG_ID:?}
py(){ python3 -c "import sys,json;d=json.load(sys.stdin);$1"; }
UID_=$(zapi POST /management/v1/users/_search '{"queries":[{"userNameQuery":{"userName":"console-onboarding","method":"TEXT_QUERY_METHOD_EQUALS"}}]}' $PORG | py "r=d.get('result',[]);print(r[0]['id'] if r else '')")
if [ -z "$UID_" ]; then
  UID_=$(zapi POST /management/v1/users/machine '{"userName":"console-onboarding","name":"Console onboarding","description":"Creates orgs for self-registered users (console)","accessTokenType":"ACCESS_TOKEN_TYPE_BEARER"}' $PORG | py "print(d['userId'])")
  zapi POST /admin/v1/members "{\"userId\":\"$UID_\",\"roles\":[\"IAM_ORG_MANAGER\"]}" >/dev/null
  echo "created console-onboarding $UID_"
fi
EXP=$(date -u -d '+1 year' +%Y-%m-%dT%H:%M:%SZ)
cp $S $S.bak-$(date +%s)
zapi POST /management/v1/users/$UID_/pats "{\"expirationDate\":\"$EXP\"}" $PORG | py "print(json.dumps(d['token']),end='')" | sops set --value-stdin $S '["console_idp_service_token"]'
echo "PAT stored in sops console_idp_service_token (expires $EXP)"
