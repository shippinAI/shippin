#!/bin/bash
# Throwaway identities for live-testing platform access (never the operator's org or users).
#   up:   org "access-test" + machine user "access-owner" (ORG_OWNER, PAT -> $DIR/.access-owner-pat)
#         + human "ops-test-admin" in the platform org (IAM_OWNER, password -> $DIR/.ops-test-admin-pw)
#         + human "access-human-owner" (ORG_OWNER, no password; receives the tenant's e-mail notices)
#   down: drop ops-test-admin's IAM role, deactivate the users, shred the files.
# Notice addresses go to the Shippin Mail test mailbox: ADMIN_EMAIL, OWNER_EMAIL.
# Secrets are only ever written to mode-600 files; nothing is printed.
set -euo pipefail
DIR=/var/lib/shippin/site; PORG=${PLATFORM_ORG_ID:?}; MODE=${1:?up|down}
ADMIN_EMAIL=${ADMIN_EMAIL:?set ADMIN_EMAIL}; OWNER_EMAIL=${OWNER_EMAIL:?set OWNER_EMAIL}
py(){ python3 -c "import sys,json;d=json.load(sys.stdin);$1"; }
find_user(){ zapi POST /management/v1/users/_search "{\"queries\":[{\"userNameQuery\":{\"userName\":\"$1\",\"method\":\"TEXT_QUERY_METHOD_EQUALS\"}}]}" "$2" | py "r=d.get('result',[]);print(r[0]['id'] if r else '')"; }
ORG=$(zapi POST /admin/v1/orgs/_search '{"queries":[{"nameQuery":{"name":"access-test","method":"TEXT_QUERY_METHOD_EQUALS"}}]}' | py "r=d.get('result',[]);print(r[0]['id'] if r else '')")
if [ "$MODE" = down ]; then
  A=$(find_user ops-test-admin $PORG); O=${ORG:+$(find_user access-owner $ORG)}
  [ -n "$A" ] && zapi DELETE /admin/v1/members/$A >/dev/null && zapi POST /management/v1/users/$A/_deactivate '{}' $PORG >/dev/null
  [ -n "$O" ] && zapi POST /management/v1/users/$O/_deactivate '{}' $ORG >/dev/null
  H=${ORG:+$(find_user access-human-owner $ORG)}
  [ -n "$H" ] && zapi POST /management/v1/users/$H/_deactivate '{}' $ORG >/dev/null
  shred -u $DIR/.access-owner-pat $DIR/.ops-test-admin-pw 2>/dev/null || true
  echo "test identities deactivated, files shredded"; exit 0
fi
umask 077
if [ -z "$ORG" ]; then
  ORG=$(zapi POST /management/v1/orgs '{"name":"access-test"}' | py "print(d['id'])")
fi
O=$(find_user access-owner $ORG)
if [ -z "$O" ]; then
  O=$(zapi POST /management/v1/users/machine '{"userName":"access-owner","name":"Avery Owner (test)","description":"Throwaway tenant owner for access tests","accessTokenType":"ACCESS_TOKEN_TYPE_BEARER"}' $ORG | py "print(d['userId'])")
  zapi POST /management/v1/orgs/me/members "{\"userId\":\"$O\",\"roles\":[\"ORG_OWNER\"]}" $ORG >/dev/null
else
  zapi POST /management/v1/users/$O/_reactivate '{}' $ORG >/dev/null || true  # after a previous "down"
fi
H=$(find_user access-human-owner $ORG)
if [ -z "$H" ]; then
  H=$(zapi POST /management/v1/users/human/_import "{\"userName\":\"access-human-owner\",\"profile\":{\"firstName\":\"Harper\",\"lastName\":\"Owner (test)\"},\"email\":{\"email\":\"$OWNER_EMAIL\",\"isEmailVerified\":true}}" $ORG | py "print(d['userId'])")
  zapi POST /management/v1/orgs/me/members "{\"userId\":\"$H\",\"roles\":[\"ORG_OWNER\"]}" $ORG >/dev/null
else
  zapi POST /management/v1/users/$H/_reactivate '{}' $ORG >/dev/null || true
fi
# The org's creator (the control-plane user) becomes ORG_OWNER; drop that so admins are non-members.
ME=$(zapi GET /auth/v1/users/me | py "print(d['user']['id'])")
zapi DELETE /management/v1/orgs/me/members/$ME '' $ORG >/dev/null || true
EXP=$(date -u -d '+1 day' +%Y-%m-%dT%H:%M:%SZ)
zapi POST /management/v1/users/$O/pats "{\"expirationDate\":\"$EXP\"}" $ORG | py "open('$DIR/.access-owner-pat','w').write(d['token'])"
A=$(find_user ops-test-admin $PORG)
python3 -c "import secrets,string;a=string.ascii_letters+string.digits;print('Tq9!'+''.join(secrets.choice(a) for _ in range(24)),end='')" > $DIR/.ops-test-admin-pw
BODY=$(ADMIN_EMAIL=$ADMIN_EMAIL python3 - "$DIR/.ops-test-admin-pw" <<'PY'
import json,os,sys
pw=open(sys.argv[1]).read()
print(json.dumps({"userName":"ops-test-admin","profile":{"firstName":"Ops","lastName":"Test Admin","displayName":"Ops Test Admin"},
 "email":{"email":os.environ["ADMIN_EMAIL"],"isEmailVerified":True},"password":pw,"passwordChangeRequired":False}))
PY
)
if [ -z "$A" ]; then
  A=$(zapi POST /management/v1/users/human/_import "$BODY" $PORG | py "print(d['userId'])")
else
  zapi POST /management/v1/users/$A/_reactivate '{}' $PORG >/dev/null || true
  zapi PUT /management/v1/users/$A/email "{\"email\":\"$ADMIN_EMAIL\",\"isEmailVerified\":true}" $PORG >/dev/null || true
  zapi POST /management/v1/users/$A/password "$(python3 -c "import json,sys;print(json.dumps({'password':open('$DIR/.ops-test-admin-pw').read(),'noChangeRequired':True}))")" $PORG >/dev/null
fi
unset BODY
zapi POST /admin/v1/members "{\"userId\":\"$A\",\"roles\":[\"IAM_OWNER\"]}" >/dev/null || zapi PUT /admin/v1/members/$A '{"roles":["IAM_OWNER"]}' >/dev/null
echo "org=$ORG owner=$O human-owner=$H admin=$A (PAT and password in mode-600 files)"
