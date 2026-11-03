#!/usr/bin/env python3
"""Local Nomen (stock Zitadel, decision 15) setup for the Vaultix demo: console project + OIDC app and
two tenants (an org each) with one person each. Admin PAT from $NOMEN_PAT_FILE; user passwords are
generated into $NOMEN_USERS_FILE (0600). Prints ids only."""
import json, os, secrets, sys, urllib.request

BASE, PAT = os.environ["NOMEN_URL"], open(os.environ["NOMEN_PAT_FILE"]).read().strip()
REDIRECT = os.environ["CONSOLE_URL"] + "/auth/callback"

def call(m, p, b=None, org=None):
    h = {"Authorization": "Bearer " + PAT, "Content-Type": "application/json"}
    if org: h["x-zitadel-orgid"] = org
    req = urllib.request.Request(BASE + p, json.dumps(b).encode() if b is not None else None, h, method=m)
    try:
        return json.loads(urllib.request.urlopen(req).read() or b"{}")
    except urllib.error.HTTPError as e:
        sys.exit(f"{m} {p}: {e.code} {e.read()[:300]}")

org = call("GET", "/management/v1/orgs/me")["org"]["id"]
pid = call("POST", "/management/v1/projects", {"name": "Shippin Console"}, org)["id"]
app = call("POST", f"/management/v1/projects/{pid}/apps/oidc", {"name": "console", "redirectUris": [REDIRECT],
    "postLogoutRedirectUris": [os.environ["CONSOLE_URL"] + "/login"], "responseTypes": ["OIDC_RESPONSE_TYPE_CODE"],
    "grantTypes": ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"], "appType": "OIDC_APP_TYPE_WEB", "authMethodType": "OIDC_AUTH_METHOD_TYPE_NONE",
    "accessTokenType": "OIDC_TOKEN_TYPE_BEARER", "idTokenUserinfoAssertion": True, "devMode": True}, org)
users = []
for name, given in (("tenant-alpha", "Alice"), ("tenant-beta", "Bob")):
    o = call("POST", "/management/v1/orgs", {"name": name})["id"]
    email, pw = f"{given.lower()}@{name}.example.com", "Vx-" + secrets.token_urlsafe(18)
    u = call("POST", "/management/v1/users/human/_import", {"userName": email, "profile": {"firstName": given, "lastName": "Tester"},
        "email": {"email": email, "isEmailVerified": True}, "password": pw, "passwordChangeRequired": False}, o)
    users.append({"org": o, "orgName": name, "user": u["userId"], "email": email, "password": pw})
old = os.umask(0o077)
json.dump(users, open(os.environ["NOMEN_USERS_FILE"], "w"))
os.umask(old)
print(json.dumps({"org": org, "project": pid, "client_id": app["clientId"],
                  "tenants": [{k: u[k] for k in ("org", "orgName", "user", "email")} for u in users]}))
