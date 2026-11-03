#!/bin/bash
# Idempotently create the Nomen project "Shippin Console" + OIDC PKCE app (auth method NONE).
# APP_NAME=operator APP_HOST=ops.<domain> registers the operator app (its own client id = its own
# audience, CONSOLE-IA); the default is the console app on console.<domain>. Runs on Gold. Uses the control-plane PAT from sops; never prints it. Prints only the client id.
set -euo pipefail
S=/var/lib/shippin/site/secrets/platform-core.sops.yaml; P="--project platform"
D=${DOMAIN:?set DOMAIN}; ORG=${PLATFORM_ORG_ID:?}; HOST=${APP_HOST:-${CONSOLE_HOST:-console.$D}}; APP=${APP_NAME:-console}
[[ "$APP" =~ ^[a-z][a-z0-9-]*$ && "$HOST" =~ ^[a-z0-9.-]+$ ]] || { echo "bad APP_NAME/APP_HOST" >&2; exit 1; }
T=$(sops decrypt --extract '["zitadel_control_plane_pat"]' $S)
api(){ incus exec edge $P --env T="$T" -- sh -c 'curl -sk --resolve id.'"$D"':443:127.0.0.1 -H "Authorization: Bearer $T" -H "x-nomen-orgid: '"$ORG"'" -H "Content-Type: application/json" "$@"' _ "$@" </dev/null; }
py(){ python3 -c "import sys,json;d=json.load(sys.stdin);$1"; }
PID=$(api -X POST https://id.$D/management/v1/projects/_search -d '{"queries":[{"nameQuery":{"name":"Shippin Console","method":"TEXT_QUERY_METHOD_EQUALS"}}]}' | py "r=d.get('result',[]);print(r[0]['id'] if r else '')")
if [ -z "$PID" ]; then
  PID=$(api -X POST https://id.$D/management/v1/projects -d '{"name":"Shippin Console"}' | py "print(d['id'])")
  echo "created project $PID"
fi
CID=$(api -X POST https://id.$D/management/v1/projects/$PID/apps/_search -d '{}' | py "r=[a for a in d.get('result',[]) if a['name']=='$APP'];print(r[0]['oidcConfig']['clientId'] if r else '')")
if [ -z "$CID" ]; then
  BODY=$(cat <<J
{"name":"$APP","redirectUris":["https://$HOST/auth/callback"],"postLogoutRedirectUris":["https://$HOST/login"],
 "responseTypes":["OIDC_RESPONSE_TYPE_CODE"],"grantTypes":["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"],
 "appType":"OIDC_APP_TYPE_WEB","authMethodType":"OIDC_AUTH_METHOD_TYPE_NONE","accessTokenType":"OIDC_TOKEN_TYPE_BEARER",
 "idTokenUserinfoAssertion":true,"devMode":false}
J
)
  CID=$(api -X POST https://id.$D/management/v1/projects/$PID/apps/oidc -d "$BODY" | py "print(d.get('clientId') or d)")
  echo "created app"
fi
unset T
echo "project=$PID client_id=$CID"
