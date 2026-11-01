#!/bin/bash
# Throwaway identities for the VM-first live test (never the operator's org or users; separate from access-test).
#   up:   org "vm-test" + machine "vm-owner" (ORG_OWNER, 1-day PAT -> $DIR/.vm-owner-pat)
#         + machine "vm-test-admin" in the platform org (IAM_OWNER, 1-day PAT -> $DIR/.vm-admin-pat)
#   down: drop vm-test-admin's IAM role, deactivate both machine users, shred the PAT files.
# Usage on Gold: PLATFORM_ORG_ID=... ZAPI=/var/lib/shippin/site/zapi.sh ./vm-test-identity.sh up|down
set -euo pipefail
DIR=/var/lib/shippin/site; PORG=${PLATFORM_ORG_ID:?}; MODE=${1:?up|down}; ZAPI=${ZAPI:-$DIR/zapi.sh}
zapi(){ bash "$ZAPI" "$@"; }
py(){ python3 -c "import sys,json;d=json.load(sys.stdin);$1"; }
find_user(){ zapi POST /management/v1/users/_search "{\"queries\":[{\"userNameQuery\":{\"userName\":\"$1\",\"method\":\"TEXT_QUERY_METHOD_EQUALS\"}}]}" "$2" | py "r=d.get('result',[]);print(r[0]['id'] if r else '')"; }
machine(){ # machine USERNAME NAME ORG -> id (created or reactivated)
  local id; id=$(find_user "$1" "$3")
  if [ -z "$id" ]; then
    id=$(zapi POST /management/v1/users/machine "{\"userName\":\"$1\",\"name\":\"$2\",\"description\":\"Throwaway VM-first test identity\",\"accessTokenType\":\"ACCESS_TOKEN_TYPE_BEARER\"}" "$3" | py "print(d['userId'])")
  else
    zapi POST /management/v1/users/$id/_reactivate '{}' "$3" >/dev/null || true
  fi
  echo "$id"
}
ORG=$(zapi POST /admin/v1/orgs/_search '{"queries":[{"nameQuery":{"name":"vm-test","method":"TEXT_QUERY_METHOD_EQUALS"}}]}' | py "r=d.get('result',[]);print(r[0]['id'] if r else '')")
if [ "$MODE" = down ]; then
  A=$(find_user vm-test-admin $PORG); O=${ORG:+$(find_user vm-owner $ORG)}
  [ -n "$A" ] && { zapi DELETE /admin/v1/members/$A >/dev/null; zapi POST /management/v1/users/$A/_deactivate '{}' $PORG >/dev/null; }
  [ -n "$O" ] && zapi POST /management/v1/users/$O/_deactivate '{}' $ORG >/dev/null
  shred -u $DIR/.vm-owner-pat $DIR/.vm-admin-pat 2>/dev/null || true
  echo "vm-test identities deactivated, PAT files shredded"; exit 0
fi
umask 077
[ -n "$ORG" ] || ORG=$(zapi POST /management/v1/orgs '{"name":"vm-test"}' | py "print(d['id'])")
O=$(machine vm-owner "Vera Owner (vm test)" $ORG)
zapi POST /management/v1/orgs/me/members "{\"userId\":\"$O\",\"roles\":[\"ORG_OWNER\"]}" $ORG >/dev/null || true
ME=$(zapi GET /auth/v1/users/me | py "print(d['user']['id'])")
zapi DELETE /management/v1/orgs/me/members/$ME '' $ORG >/dev/null || true  # admins stay non-members
A=$(machine vm-test-admin "VM Test Admin" $PORG)
zapi POST /admin/v1/members "{\"userId\":\"$A\",\"roles\":[\"IAM_OWNER\"]}" >/dev/null || zapi PUT /admin/v1/members/$A '{"roles":["IAM_OWNER"]}' >/dev/null
EXP=$(date -u -d '+1 day' +%Y-%m-%dT%H:%M:%SZ)
zapi POST /management/v1/users/$O/pats "{\"expirationDate\":\"$EXP\"}" $ORG | py "open('$DIR/.vm-owner-pat','w').write(d['token'])"
zapi POST /management/v1/users/$A/pats "{\"expirationDate\":\"$EXP\"}" $PORG | py "open('$DIR/.vm-admin-pat','w').write(d['token'])"
echo "org=$ORG owner=$O admin=$A (PATs in mode-600 files)"
