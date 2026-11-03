#!/bin/bash
# Throwaway identities for the tenant ↔ admin switch check (never the operator's org or users).
#   up:   org "switch-test" with machine "switch-admin" (ORG_OWNER there + IAM_OWNER) and machine
#         "switch-member" (ORG_OWNER there, no IAM role); 1-day PATs -> $DIR/.switch-{admin,member}-pat
#   down: drop the IAM role, deactivate both users, shred the PAT files.
set -euo pipefail
DIR=/var/lib/shippin/site; MODE=${1:?up|down}; ZAPI=${ZAPI:-$DIR/zapi.sh}
zapi(){ bash "$ZAPI" "$@"; }
py(){ python3 -c "import sys,json;d=json.load(sys.stdin);$1"; }
find_user(){ zapi POST /management/v1/users/_search "{\"queries\":[{\"userNameQuery\":{\"userName\":\"$1\",\"method\":\"TEXT_QUERY_METHOD_EQUALS\"}}]}" "$2" | py "r=d.get('result',[]);print(r[0]['id'] if r else '')"; }
ORG=$(zapi POST /admin/v1/orgs/_search '{"queries":[{"nameQuery":{"name":"switch-test","method":"TEXT_QUERY_METHOD_EQUALS"}}]}' | py "r=d.get('result',[]);print(r[0]['id'] if r else '')")
if [ "$MODE" = down ]; then
  for u in switch-admin switch-member; do
    id=${ORG:+$(find_user $u $ORG)}; [ -n "$id" ] || continue
    [ $u = switch-admin ] && zapi DELETE /admin/v1/members/$id >/dev/null
    zapi POST /management/v1/users/$id/_deactivate '{}' $ORG >/dev/null
  done
  shred -u $DIR/.switch-admin-pat $DIR/.switch-member-pat 2>/dev/null || true
  echo "switch-test identities deactivated, PAT files shredded"; exit 0
fi
umask 077
[ -n "$ORG" ] || ORG=$(zapi POST /management/v1/orgs '{"name":"switch-test"}' | py "print(d['id'])")
EXP=$(date -u -d '+1 day' +%Y-%m-%dT%H:%M:%SZ)
for u in switch-admin switch-member; do
  id=$(find_user $u $ORG)
  if [ -z "$id" ]; then
    id=$(zapi POST /management/v1/users/machine "{\"userName\":\"$u\",\"name\":\"$u (test)\",\"accessTokenType\":\"ACCESS_TOKEN_TYPE_BEARER\"}" $ORG | py "print(d['userId'])")
  else
    zapi POST /management/v1/users/$id/_reactivate '{}' $ORG >/dev/null || true
  fi
  zapi POST /management/v1/orgs/me/members "{\"userId\":\"$id\",\"roles\":[\"ORG_OWNER\"]}" $ORG >/dev/null || true
  [ $u = switch-admin ] && { zapi POST /admin/v1/members "{\"userId\":\"$id\",\"roles\":[\"IAM_OWNER\"]}" >/dev/null || true; }
  zapi POST /management/v1/users/$id/pats "{\"expirationDate\":\"$EXP\"}" $ORG | py "open('$DIR/.$u-pat','w').write(d['token'])"
done
ME=$(zapi GET /auth/v1/users/me | py "print(d['user']['id'])")
zapi DELETE /management/v1/orgs/me/members/$ME '' $ORG >/dev/null || true
echo "org=$ORG (PATs in mode-600 files)"
