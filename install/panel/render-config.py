#!/usr/bin/env python3
# Writes the panel's config file from the example and the site's environment. The installer and the
# container entrypoint both use this, so a setting means the same thing however the panel is deployed.
import json, os, sys
root = sys.argv[1]
src = os.environ.get("SHIPPIN_CONFIG_TEMPLATE") or os.path.join(root, "config", "shippin.example.json")
with open(src) as f:
    cfg = json.load(f)
def set_if(env, obj, key):
    if os.environ.get(env):
        obj[key] = os.environ[env]
set_if("SHIPPIN_DOMAIN", cfg, "domain")
if os.environ.get("SHIPPIN_DOMAIN"):
    cfg["governance_domains"] = [os.environ["SHIPPIN_DOMAIN"]]
    cfg.setdefault("workspaces", {})["reserved_domains"] = [os.environ["SHIPPIN_DOMAIN"]]
set_if("SHIPPIN_CONSOLE_HOST", cfg, "console_host")
set_if("SHIPPIN_HOSTD_URL", cfg.setdefault("incus", {}), "hostd_url")
op = cfg.setdefault("operator", {})
set_if("SHIPPIN_OPERATOR_HOST", op, "host")
if os.environ.get("SHIPPIN_OPERATOR_HOST") == "none":
    op["host"] = ""
idp = cfg.setdefault("idp", {})
set_if("SHIPPIN_IDP_ISSUER", idp, "issuer")
set_if("SHIPPIN_IDP_NAME", idp, "name")
if os.environ.get("SHIPPIN_OIDC_CLIENT_ID"):
    idp["client_id"] = os.environ["SHIPPIN_OIDC_CLIENT_ID"]
set_if("SHIPPIN_IDP_DIALECT", idp, "dialect")
# Upstream sign-in buttons the IdP offers: id:Label:idp_id, comma separated.
providers = os.environ.get("SHIPPIN_IDP_PROVIDERS", "").strip()
if providers:
    idp["providers"] = []
    for item in providers.split(","):
        parts = [p.strip() for p in item.split(":")]
        if len(parts) != 3 or not all(parts):
            sys.exit("SHIPPIN_IDP_PROVIDERS entries must be id:Label:idp_id")
        idp["providers"].append({"id": parts[0], "label": parts[1], "idp_id": parts[2]})
set_if("SHIPPIN_OPERATOR_PLATFORM_ORG_ID", op, "platform_org_id")
set_if("SHIPPIN_STAFF_PROJECT_ID", cfg.setdefault("staff", {}), "project_id")
dr = cfg.setdefault("directory", {})
signup = os.environ.get("SHIPPIN_SIGNUP", "")
if signup:
    if signup not in ("open", "invite_only", "waitlist"):
        sys.exit("SHIPPIN_SIGNUP must be open, invite_only or waitlist")
    dr["signup"] = signup
captcha = os.environ.get("SHIPPIN_CAPTCHA_PROVIDER", "")
if captcha:
    if captcha not in ("none", "turnstile"):
        sys.exit("SHIPPIN_CAPTCHA_PROVIDER must be none or turnstile")
    dr.setdefault("captcha", {})["provider"] = captcha
if os.environ.get("SHIPPIN_SERVERS_ENABLED") in ("1", "true", "yes"):
    cfg.setdefault("servers", {})["enabled"] = True
dest = os.environ.get("SHIPPIN_CONFIG_OUT") or "/etc/shippin/shippin.json"
with open(dest, "w") as f:
    json.dump(cfg, f, indent=2)
    f.write("\n")
os.chmod(dest, 0o644)
