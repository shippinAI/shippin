#!/usr/bin/env python3
"""Throwaway staff users for the live authz test (run on Gold as root).

  staff-live-users.py up   ROLE...  -> creates one human per role in the operator org with a
                                       password and a verified TOTP, grants the role, and writes
                                       credentials to $OUT (mode 600, never printed).
  staff-live-users.py down          -> deletes every user listed in $OUT, then shreds it.
The control-plane PAT is read from sops and never printed. ROLE "none" creates a user with MFA
but no staff role; "nomfa:<role>" creates one with the role but no second factor.
"""
import base64, hashlib, hmac, http.client, json, os, secrets, socket, ssl, struct, subprocess, sys, time

ORG = os.environ.get("PLATFORM_ORG_ID", "")
PROJECT = os.environ.get("PROJECT_ID", "")
OUT = os.environ.get("OUT", "/var/lib/shippin/site/staff-live-creds.json")
HOST, LAN = os.environ["IDP_HOST"], os.environ.get("EDGE_IP", "192.0.2.252")
SOPS = "/var/lib/shippin/site/secrets/platform-core.sops.yaml"
TOKEN = subprocess.run(["sops", "decrypt", "--extract", '["zitadel_control_plane_pat"]', SOPS],
                       check=True, capture_output=True, text=True).stdout.strip()


def call(method, path, body=None):
    # Connect to the LAN edge while keeping SNI/Host = the IdP's name.
    conn = _Conn(LAN, ssl.create_default_context())
    hdr = {"Authorization": "Bearer " + TOKEN, "Content-Type": "application/json",
           "x-nomen-orgid": ORG}
    conn.request(method, path, json.dumps(body) if body is not None else None, hdr)
    r = conn.getresponse()
    data = r.read()
    if r.status >= 300:
        raise SystemExit(f"{method} {path}: HTTP {r.status} {data[:200]!r}")
    return json.loads(data or b"{}")



class _Conn(http.client.HTTPSConnection):
    def __init__(self, ip, ctx):
        super().__init__(HOST, 443, context=ctx)
        self._ip = ip

    def connect(self):
        sock = socket.create_connection((self._ip, 443), self.timeout)
        self.sock = self._context.wrap_socket(sock, server_hostname=HOST)


def totp(secret, at=None):
    key = base64.b32decode(secret + "=" * (-len(secret) % 8))
    msg = struct.pack(">Q", int((at or time.time()) // 30))
    h = hmac.new(key, msg, hashlib.sha1).digest()
    o = h[-1] & 15
    return "%06d" % ((struct.unpack(">I", h[o:o + 4])[0] & 0x7FFFFFFF) % 1000000)


def make(spec):
    mfa, role = (False, spec[6:]) if spec.startswith("nomfa:") else (True, spec)
    tag = (role.replace("staff.", "") + ("-nomfa" if not mfa else "")).replace("_", "-")
    login = f"staff-live-{tag}-{secrets.token_hex(3)}@{HOST.split('.', 1)[1]}"
    pw = "Sl-" + secrets.token_urlsafe(18) + "9!"
    u = call("POST", "/v2/users/human", {
        "organization": {"orgId": ORG}, "username": login,
        "profile": {"givenName": "Live", "familyName": tag},
        "email": {"email": login, "isVerified": True},
        "password": {"password": pw, "changeRequired": False}})
    uid = u["userId"]
    rec = {"spec": spec, "role": role, "user_id": uid, "login": login, "password": pw, "totp": ""}
    if mfa:
        t = call("POST", f"/v2/users/{uid}/totp", {})
        call("POST", f"/v2/users/{uid}/totp/verify", {"code": totp(t["secret"])})
        rec["totp"] = t["secret"]
    if role != "none":
        call("POST", f"/management/v1/users/{uid}/grants", {"projectId": PROJECT, "roleKeys": [role]})
    return rec


def save(recs):
    fd = os.open(OUT, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as f:
        json.dump(recs, f)


def main():
    if sys.argv[1:2] == ["up"]:
        recs = []
        try:
            for spec in sys.argv[2:]:
                recs.append(make(spec))
                print("created", recs[-1]["spec"], recs[-1]["user_id"])
        finally:
            save(recs)
    elif sys.argv[1:2] == ["down"]:
        recs = json.load(open(OUT)) if os.path.exists(OUT) else []
        for r in recs:
            try:
                call("DELETE", f"/v2/users/{r['user_id']}")
                print("deleted", r["spec"], r["user_id"])
            except SystemExit as e:
                print("delete failed:", e)
        subprocess.run(["shred", "-u", OUT], check=False)
    elif sys.argv[1:2] == ["code"]:
        print(totp(sys.argv[2]))
    else:
        raise SystemExit(__doc__)


main()
