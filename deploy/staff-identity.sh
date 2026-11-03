#!/bin/bash
# Staff roles in Nomen (run on Gold as root; uses the control-plane PAT via zapi.sh).
#   up:    machine user "console-team" in the operator org with ORG_USER_MANAGER +
#          ORG_USER_PERMISSION_EDITOR there and PROJECT_OWNER on the console project; a 1-year PAT
#          goes straight into sops as console_idp_team_token (never printed).
#   owner: create the first owner role key if missing and grant it to OWNER_USER_ID (bootstrap
#          only; every later grant is made in the Team page).
# Role keys/names/permissions are data in Postgres (staff_roles); the console syncs them to the
# project at startup and on every edit. Nothing here names a person.
#   env: PROJECT_ID, PLATFORM_ORG_ID, [OWNER_USER_ID, OWNER_ROLE=staff.owner]
set -euo pipefail
DIR=/var/lib/shippin/site; ZAPI=${ZAPI:-$DIR/zapi.sh}; S=$DIR/secrets/platform-core.sops.yaml
P=${PROJECT_ID:?}; ORG=${PLATFORM_ORG_ID:?}; MODE=${1:?up|owner}
zapi(){ bash "$ZAPI" "$@"; }
py(){ python3 -c "import sys,json;d=json.load(sys.stdin);$1"; }
find_user(){ zapi POST /management/v1/users/_search "{\"queries\":[{\"userNameQuery\":{\"userName\":\"$1\",\"method\":\"TEXT_QUERY_METHOD_EQUALS\"}}]}" "$ORG" | py "r=d.get('result',[]);print(r[0]['id'] if r else '')"; }
if [ "$MODE" = owner ]; then
  U=${OWNER_USER_ID:?}; R=${OWNER_ROLE:-staff.owner}
  zapi POST /management/v1/projects/$P/roles "{\"roleKey\":\"$R\",\"displayName\":\"Owner\",\"group\":\"staff\"}" $ORG >/dev/null || true
  G=$(zapi POST /management/v1/users/grants/_search "{\"queries\":[{\"projectIdQuery\":{\"projectId\":\"$P\"}},{\"userIdQuery\":{\"userId\":\"$U\"}}]}" $ORG | py "r=d.get('result',[]);print(r[0]['id'] if r else '')")
  if [ -z "$G" ]; then
    zapi POST /management/v1/users/$U/grants "{\"projectId\":\"$P\",\"roleKeys\":[\"$R\"]}" $ORG | py "print('grant', d.get('userGrantId') or d)"
  else
    K=$(zapi GET /management/v1/users/$U/grants/$G '' $ORG | py "print(' '.join(d['userGrant'].get('roleKeys',[])))")
    case " $K " in *" $R "*) echo "grant exists";; *)
      zapi PUT /management/v1/users/$U/grants/$G "$(python3 -c "import json,sys;print(json.dumps({'roleKeys':sys.argv[1].split()+[sys.argv[2]]}))" "$K" "$R")" $ORG >/dev/null; echo "role added";; esac
  fi
  exit 0
fi
M=$(find_user console-team)
if [ -z "$M" ]; then
  M=$(zapi POST /management/v1/users/machine '{"userName":"console-team","name":"Console team manager","description":"Shippin console Team page: staff invites, role grants, staff role sync","accessTokenType":"ACCESS_TOKEN_TYPE_BEARER"}' $ORG | py "print(d['userId'])")
fi
zapi POST /management/v1/orgs/me/members "{\"userId\":\"$M\",\"roles\":[\"ORG_USER_MANAGER\",\"ORG_USER_PERMISSION_EDITOR\"]}" $ORG >/dev/null || \
  zapi PUT /management/v1/orgs/me/members/$M '{"roles":["ORG_USER_MANAGER","ORG_USER_PERMISSION_EDITOR"]}' $ORG >/dev/null
zapi POST /management/v1/projects/$P/members "{\"userId\":\"$M\",\"roles\":[\"PROJECT_OWNER\"]}" $ORG >/dev/null || true
if sops decrypt --extract '["console_idp_team_token"]' $S >/dev/null 2>&1 && [ -z "${ROTATE:-}" ]; then
  echo "console-team ready (token already in sops; ROTATE=1 to replace)"; exit 0
fi
EXP=$(date -u -d '+1 year' +%Y-%m-%dT%H:%M:%SZ)
zapi POST /management/v1/users/$M/pats "{\"expirationDate\":\"$EXP\"}" $ORG | py "print(json.dumps(d['token']))" |
  sops set --value-stdin $S '["console_idp_team_token"]'
echo "console-team ready; token stored in sops (expires $EXP)"
