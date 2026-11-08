#!/usr/bin/env python3
"""Exercise a disposable, fresh Docker panel. Never run against an owned install.

SHIPPIN_TEST_PANEL_URL must be loopback HTTPS; SHIPPIN_TEST_SETUP_CODE comes from
that disposable container. Credentials/recovery codes remain in memory. The
script prints only check outcomes and retains no authentication material.
"""
import http.cookiejar
import json
import os
import secrets
import ssl
import urllib.error
import urllib.parse
import urllib.request
import uuid


def run(restart=None):
    origin = os.environ["SHIPPIN_TEST_PANEL_URL"].rstrip("/")
    parsed = urllib.parse.urlparse(origin)
    if parsed.scheme != "https" or parsed.hostname not in ("localhost", "127.0.0.1"):
        raise RuntimeError("Disposable smoke test requires loopback HTTPS")
    # The disposable panel uses its generated local certificate.
    tls = ssl._create_unverified_context()

    def client():
        return urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), urllib.request.HTTPSHandler(context=tls))

    owner = client()

    def request(who, method, path, body=None, expected=200, key=None):
        headers = {"Origin": origin, "Content-Type": "application/json", "Idempotency-Key": key or str(uuid.uuid4())}
        req = urllib.request.Request(origin + path, None if body is None else json.dumps(body).encode(), headers, method=method)
        try:
            response = who.open(req, timeout=20)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read()
            if response.status != expected:
                message = json.loads(raw).get("message", "") if raw.startswith(b"{") else ""
                raise AssertionError(f"{method} {path}: {response.status}, expected {expected}: {message}")
            return json.loads(raw) if raw else None

    settings = request(owner, "GET", "/v1/settings/public")
    if not settings["setup_open"]:
        raise RuntimeError("Refusing to alter an installation that already has an owner")
    ready = request(owner, "GET", "/readyz")
    assert ready["db"] and ready["cache"] == "up"
    password = secrets.token_urlsafe(24)
    setup = {"code": os.environ["SHIPPIN_TEST_SETUP_CODE"], "username": "smoke-owner", "name": "Smoke Owner", "email": "owner@example.invalid", "password": password, "organization": "Smoke Operations", "brand": {"name": "Smoke Cloud", "short": "Smoke", "color": "#123456"}}
    request(owner, "POST", "/v1/setup", {**setup, "code": "incorrect"}, 401)
    request(owner, "POST", "/v1/setup", {**setup, "password": "short"}, 400)
    installed = request(owner, "POST", "/v1/setup", setup)
    assert len(installed["recovery_codes"]) == 8
    request(owner, "POST", "/v1/setup", setup, 409)
    me = request(owner, "GET", "/v1/me")
    assert me["platform_roles"] and not me["tenants"]
    assert me["capabilities"]["infra.hosts"]
    assert "providers" in request(owner, "GET", "/v1/infra/overview")
    if os.environ.get("SHIPPIN_TEST_COMMIT"):
        version = request(owner, "GET", "/v1/version")["running"]
        assert version["commit"] == os.environ["SHIPPIN_TEST_COMMIT"] and not version.get("modified", False)
    mode = request(owner, "GET", "/v1/operator/staff-mode")
    if mode["required"]:
        request(owner, "GET", "/v1/operator/system", expected=403)
        request(owner, "POST", "/v1/tenants", {"name": "Denied without staff mode", "owner_user_id": installed["user_id"]}, 403)
        request(owner, "POST", "/v1/operator/staff-mode", {"reason": "Verify initial installation ownership"})
    system = request(owner, "GET", "/v1/operator/system")
    assert system["organization"]["name"] == "Smoke Operations"
    assert request(owner, "GET", "/v1/settings/public")["brand"]["name"] == "Smoke Cloud"
    print("PASS setup validation, replay rejection, internal ownership and saved branding")
    plans = request(owner, "GET", "/v1/operator/catalog/plans")["items"]
    plan = next(p["id"] for p in plans if p["active"])
    key = str(uuid.uuid4())
    body = {"name": "Customer One", "owner_user_id": installed["user_id"], "plan_id": plan}
    tenant = request(owner, "POST", "/v1/tenants", body, 201, key)
    repeated = request(owner, "POST", "/v1/tenants", body, 201, key)
    assert tenant["id"] == repeated["id"] and tenant["status"] == "draft"
    tenant_id = tenant["id"]
    base = f"/v1/tenants/{tenant_id}/organization-workspaces"
    spaces = request(owner, "GET", base)
    assert len(spaces) == 1 and spaces[0]["is_default"]
    group = request(owner, "POST", base, {"name": "Product", "description": "Two projects"}, 201)
    request(owner, "PATCH", base + "/" + group["id"], {"name": "Product delivery", "description": "Saved"})
    for name in ("first-app", "second-app"):
        project = request(owner, "POST", f"/v1/tenants/{tenant_id}/projects", {"name": name, "workspace_id": group["id"]}, 201)
        assert project["workspace_id"] == group["id"] and project["state"] == "pending"
    assert len(request(owner, "GET", base + "/" + group["id"] + "/projects")) == 2
    request(owner, "DELETE", base + "/" + group["id"], expected=409)
    request(owner, "DELETE", base + "/" + spaces[0]["id"], expected=409)
    request(client(), "GET", base, expected=401)
    request(owner, "GET", f"/v1/tenants/{uuid.uuid4()}/organization-workspaces", expected=404)
    print("PASS explicit customer creation, idempotency, default/additional workspace, two pending projects and deletion guards")
    second = client()
    request(second, "POST", "/v1/session/password", {"identifier": "smoke-owner", "password": "wrong"}, 401)
    request(second, "POST", "/v1/session/password", {"identifier": "smoke-owner", "password": password})
    if restart is not None:
        restart()
    assert len(request(second, "GET", base + "/" + group["id"] + "/projects")) == 2
    next_password = secrets.token_urlsafe(24)
    request(second, "PUT", "/v1/directory/me/password", {"current_password": "wrong", "password": next_password}, 401)
    request(second, "PUT", "/v1/directory/me/password", {"current_password": password, "password": next_password})
    request(client(), "POST", "/v1/session/password", {"identifier": "smoke-owner", "password": password}, 401)
    request(client(), "POST", "/v1/session/password", {"identifier": "smoke-owner", "password": next_password})
    print("PASS separate password session, persistent project reads, wrong-password denial and password rotation")


if __name__ == "__main__":
    run()
