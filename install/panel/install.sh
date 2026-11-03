#!/bin/bash
# Install the Shippin panel on a Debian or Ubuntu machine.
#
#   sudo ./install/panel/install.sh --site /etc/shippin/site.env
#
# --example uses config/shippin.example.json and a generated local database.
# It is for a disposable machine and a smoke test, not a production site.
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
site=""
example=0
listen=""
skip_web="${SHIPPIN_SKIP_WEB:-}"
prefix=/usr/local/bin

usage() {
  echo "usage: sudo $0 --site /etc/shippin/site.env | --example [--listen host:port]" >&2
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --site) site=${2:?}; shift 2 ;;
    --example) example=1; shift ;;
    --listen) listen=${2:?}; shift 2 ;;
    --skip-web) skip_web=1; shift ;;
    -h|--help) usage ;;
    *) echo "unknown argument: $1" >&2; usage ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root" >&2
  exit 1
fi
if [ -n "$site" ] && [ "$example" -eq 1 ]; then
  echo "pass either --site or --example" >&2
  exit 1
fi
if [ -z "$site" ] && [ "$example" -eq 0 ]; then
  usage
fi
if [ -n "$site" ] && [ ! -f "$site" ]; then
  echo "site config not found: $site" >&2
  exit 1
fi

. /etc/os-release
case "${ID:-}" in
  debian|ubuntu) ;;
  *) echo "this installer supports Debian and Ubuntu (found ${ID:-unknown})" >&2; exit 1 ;;
esac

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq postgresql postgresql-client curl ca-certificates openssl python3 git xz-utils >/dev/null
if ! pg_isready -q; then
  ver=$(pg_lsclusters --no-header | awk 'NR==1 {print $1}')
  name=$(pg_lsclusters --no-header | awk 'NR==1 {print $2}')
  pg_ctlcluster "$ver" "$name" start
fi
for _ in $(seq 1 30); do pg_isready -q && break; sleep 1; done

# Build toolchains. Distribution packages are older than go.mod and web/ need, so the installer
# fetches the pinned upstream releases and checks their SHA-256 before unpacking. A toolchain that
# is already new enough is used as is.
go_version=1.26.10
go_sha256_amd64=ccfe7ae822d2c2f41f4fe7c956199967a4dac22d0ee0d2b2bfdb440aab3b20c5
go_sha256_arm64=3f4227ce7b206a513e794f1393adb473a07b46e94d18bd15d1c684bbb7319797
node_version=24.22.2
node_sha256_x64=48cc0e21e048790e54c0cf5749c2725005313d652651c40b7a21feb9697aaf75
node_sha256_arm64=eb187837a2230d4047161ac0be008584e16b46f726e9c9490ee35dc6b17d9ba8
toolchains=/usr/local/lib/shippin-toolchains
case "$(uname -m)" in
  x86_64|amd64) go_arch=amd64; node_arch=x64 ;;
  aarch64|arm64) go_arch=arm64; node_arch=arm64 ;;
  *) echo "unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

fetch_checked() { # url sha256 dest
  curl -fsSL --retry 3 -o "$3" "$1"
  if ! printf '%s  %s\n' "$2" "$3" | sha256sum -c --status; then
    echo "checksum mismatch for $1" >&2
    rm -f "$3"
    exit 1
  fi
}

need_go=$(sed -n 's/^go \([0-9.]*\).*/\1/p' "$root/go.mod")
go_ok() {
  command -v go >/dev/null 2>&1 || return 1
  have=$(go env GOVERSION 2>/dev/null | sed 's/^go//')
  [ "$(printf '%s\n%s\n' "$need_go" "$have" | sort -V | head -n1)" = "$need_go" ]
}
if ! go_ok; then
  dir="$toolchains/go$go_version"
  if [ ! -x "$dir/bin/go" ]; then
    sha_var="go_sha256_$go_arch"
    tmp=$(mktemp -d)
    fetch_checked "https://go.dev/dl/go${go_version}.linux-${go_arch}.tar.gz" "${!sha_var}" "$tmp/go.tgz"
    install -d "$dir"
    tar -xzf "$tmp/go.tgz" -C "$dir" --strip-components=1
    rm -rf "$tmp"
  fi
  export PATH="$dir/bin:$PATH"
  go_ok || { echo "Go $need_go or newer is required" >&2; exit 1; }
fi
export GOTOOLCHAIN=local
echo "using $(go version)"

node_ok() {
  command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1 || return 1
  major=$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)
  [ "$major" -ge 24 ]
}
if [ "$skip_web" != 1 ] && ! node_ok; then
  dir="$toolchains/node-v$node_version"
  if [ ! -x "$dir/bin/node" ]; then
    sha_var="node_sha256_$node_arch"
    tmp=$(mktemp -d)
    fetch_checked "https://nodejs.org/dist/v${node_version}/node-v${node_version}-linux-${node_arch}.tar.xz" "${!sha_var}" "$tmp/node.txz"
    install -d "$dir"
    tar -xJf "$tmp/node.txz" -C "$dir" --strip-components=1
    rm -rf "$tmp"
  fi
  export PATH="$dir/bin:$PATH"
  node_ok || { echo "Node.js 24 or newer is required to build the panel UI" >&2; exit 1; }
fi
cd "$root"

set -a
if [ -n "$site" ]; then
  # shellcheck disable=SC1090
  . "$site"
fi
set +a

if [ -n "$listen" ]; then
  SHIPPIN_LISTEN=$listen
fi
SHIPPIN_LISTEN=${SHIPPIN_LISTEN:-127.0.0.1:8080}

install -d -m 0755 /etc/shippin /var/lib/shippin
# Workspace templates seed (workspaces.template_seed). Keep a site's own copy if it has one.
if [ ! -f /etc/shippin/templates.json ]; then
  install -m 0644 "$root/config/templates.json" /etc/shippin/templates.json
fi
python3 - "$root" <<'PY'
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
op = cfg.setdefault("operator", {})
set_if("SHIPPIN_OPERATOR_HOST", op, "host")
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
dest = "/etc/shippin/shippin.json"
with open(dest, "w") as f:
    json.dump(cfg, f, indent=2)
    f.write("\n")
os.chmod(dest, 0o644)
PY

if [ -z "${SHIPPIN_DATABASE_URL:-}" ]; then
  db_pass=$(openssl rand -hex 24)
  runuser -u postgres -- psql -v ON_ERROR_STOP=1 <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'shippin') THEN
    CREATE ROLE shippin LOGIN PASSWORD '${db_pass}';
  ELSE
    ALTER ROLE shippin PASSWORD '${db_pass}';
  END IF;
END
\$\$;
SQL
  runuser -u postgres -- psql -v ON_ERROR_STOP=1 -tc "SELECT 1 FROM pg_database WHERE datname = 'shippin'" | grep -q 1 \
    || runuser -u postgres -- createdb -O shippin shippin
  SHIPPIN_DATABASE_URL="postgres://shippin:${db_pass}@127.0.0.1:5432/shippin?sslmode=disable"
fi
if [ -z "${SHIPPIN_SESSION_KEY:-}" ]; then
  SHIPPIN_SESSION_KEY=$(openssl rand -hex 32)
fi
if [ "$example" -eq 1 ] && [ -z "${SHIPPIN_MAIL_SINK:-}" ]; then
  SHIPPIN_MAIL_SINK=/var/lib/shippin/mail-sink.log
fi

# TLS. Session cookies are Secure and writes must come from an https origin, so a browser can only
# use the panel over https. Either terminate TLS in a proxy in front of it, or let the panel serve
# TLS itself: set SHIPPIN_TLS_CERT and SHIPPIN_TLS_KEY to existing files (reloaded when they change),
# or set SHIPPIN_TLS=self-signed to generate a pair for SHIPPIN_CONSOLE_HOST.
SHIPPIN_TLS=${SHIPPIN_TLS:-}
SHIPPIN_TLS_CERT=${SHIPPIN_TLS_CERT:-}
SHIPPIN_TLS_KEY=${SHIPPIN_TLS_KEY:-}
if [ "$SHIPPIN_TLS" = self-signed ] && [ -z "$SHIPPIN_TLS_CERT" ]; then
  install -d -m 0700 /etc/shippin/tls
  SHIPPIN_TLS_CERT=/etc/shippin/tls/panel.crt
  SHIPPIN_TLS_KEY=/etc/shippin/tls/panel.key
  if [ ! -s "$SHIPPIN_TLS_CERT" ] || [ ! -s "$SHIPPIN_TLS_KEY" ]; then
    cn=${SHIPPIN_CONSOLE_HOST:-localhost}
    if printf '%s' "$cn" | grep -Eq '^[0-9.]+$'; then san="IP:$cn"; else san="DNS:$cn"; fi
    (umask 077 && openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 825 \
      -subj "/CN=$cn" -addext "subjectAltName=$san,DNS:localhost,IP:127.0.0.1" \
      -keyout "$SHIPPIN_TLS_KEY" -out "$SHIPPIN_TLS_CERT" 2>/dev/null)
  fi
elif [ -n "$SHIPPIN_TLS" ] && [ "$SHIPPIN_TLS" != self-signed ]; then
  echo "SHIPPIN_TLS must be self-signed or unset" >&2
  exit 1
fi
if [ -n "$SHIPPIN_TLS_CERT$SHIPPIN_TLS_KEY" ]; then
  for f in "$SHIPPIN_TLS_CERT" "$SHIPPIN_TLS_KEY"; do
    [ -s "$f" ] || { echo "TLS file not found: ${f:-SHIPPIN_TLS_CERT and SHIPPIN_TLS_KEY must both be set}" >&2; exit 1; }
  done
fi

# Secrets that only live in console.env (IdP machine tokens, client and SMTP secrets) survive a
# re-run: a value the site file leaves unset keeps the one already installed.
keep_secrets="SHIPPIN_IDP_SERVICE_TOKEN SHIPPIN_IDP_READ_TOKEN SHIPPIN_IDP_TEAM_TOKEN SHIPPIN_OIDC_CLIENT_SECRET SHIPPIN_SMTP_PASSWORD SHIPPIN_CAPTCHA_SECRET"
if [ -f /etc/shippin/console.env ]; then
  for k in $keep_secrets; do
    if [ -z "${!k:-}" ]; then
      v=$(sed -n "s/^${k}=//p" /etc/shippin/console.env | tail -n1)
      if [ -n "$v" ]; then printf -v "$k" '%s' "$v"; fi
    fi
  done
fi

# Hypervisor tokens (SHIPPIN_HV_*) are added by the operator and named from the panel; keep them too.
hv_env=$(grep -E '^SHIPPIN_HV_[A-Z0-9_]*=' /etc/shippin/console.env 2>/dev/null || true)

umask 077
cat > /etc/shippin/console.env <<EOF
SHIPPIN_CONFIG=/etc/shippin/shippin.json
SHIPPIN_STATIC=${root}/web/dist
SHIPPIN_LISTEN=${SHIPPIN_LISTEN}
SHIPPIN_DATABASE_URL=${SHIPPIN_DATABASE_URL}
SHIPPIN_SESSION_KEY=${SHIPPIN_SESSION_KEY}
SHIPPIN_OIDC_CLIENT_ID=${SHIPPIN_OIDC_CLIENT_ID:-}
SHIPPIN_OPS_OIDC_CLIENT_ID=${SHIPPIN_OPS_OIDC_CLIENT_ID:-}
SHIPPIN_SMTP_PASSWORD=${SHIPPIN_SMTP_PASSWORD:-}
SHIPPIN_MAIL_SINK=${SHIPPIN_MAIL_SINK:-}
SHIPPIN_TLS_CERT=${SHIPPIN_TLS_CERT}
SHIPPIN_TLS_KEY=${SHIPPIN_TLS_KEY}
SHIPPIN_CAPTCHA_SECRET=${SHIPPIN_CAPTCHA_SECRET:-}
SHIPPIN_OIDC_CLIENT_SECRET=${SHIPPIN_OIDC_CLIENT_SECRET:-}
SHIPPIN_NOMEN_ISSUER=${SHIPPIN_NOMEN_ISSUER:-}
SHIPPIN_IDP_SERVICE_TOKEN=${SHIPPIN_IDP_SERVICE_TOKEN:-}
SHIPPIN_IDP_READ_TOKEN=${SHIPPIN_IDP_READ_TOKEN:-}
SHIPPIN_IDP_TEAM_TOKEN=${SHIPPIN_IDP_TEAM_TOKEN:-}
EOF
if [ -n "$hv_env" ]; then printf '%s\n' "$hv_env" >> /etc/shippin/console.env; fi
chmod 600 /etc/shippin/console.env

if [ "$skip_web" != 1 ]; then
  (cd "$root/web" && npm ci --no-audit --no-fund && npm run build)
fi
if [ ! -d "$root/web/dist" ]; then
  install -d "$root/web/dist"
  printf '<!doctype html><title>Shippin</title><p>Panel API is up. Build web/ to serve the UI.</p>\n' > "$root/web/dist/index.html"
fi

(cd "$root" && go build -o "$prefix/shippin-console" ./cmd/console)

if [ -d /run/systemd/system ] && [ "$example" -eq 0 ]; then
  cat > /etc/systemd/system/shippin-console.service <<'UNIT'
[Unit]
Description=Shippin panel
After=network.target postgresql.service

[Service]
EnvironmentFile=/etc/shippin/console.env
ExecStart=/usr/local/bin/shippin-console
Restart=on-failure
User=root

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable shippin-console.service
  # restart, not start: a re-run must replace a panel that is already running with the new build
  # and the new console.env, or the health check below passes against the old process.
  systemctl restart shippin-console.service
else
  if [ -f /var/lib/shippin/console.pid ] && kill -0 "$(cat /var/lib/shippin/console.pid)" 2>/dev/null; then
    kill "$(cat /var/lib/shippin/console.pid)" || true
    sleep 1
  fi
  set -a
  # shellcheck disable=SC1091
  . /etc/shippin/console.env
  set +a
  nohup "$prefix/shippin-console" >/var/lib/shippin/console.log 2>&1 &
  echo $! > /var/lib/shippin/console.pid
fi

host=${SHIPPIN_LISTEN%%:*}
port=${SHIPPIN_LISTEN##*:}
if [ "$host" = "0.0.0.0" ] || [ "$host" = "" ]; then
  host=127.0.0.1
fi
scheme=http
curl_tls=()
if [ -n "$SHIPPIN_TLS_CERT" ]; then
  scheme=https
  curl_tls=(--cacert "$SHIPPIN_TLS_CERT")
  # A certificate for a public name does not cover 127.0.0.1; the health check only needs the
  # listener, so it skips name checks there and nowhere else.
  if ! openssl x509 -in "$SHIPPIN_TLS_CERT" -noout -checkip "$host" 2>/dev/null | grep -q 'does match'; then
    curl_tls=(-k)
  fi
fi
url="${scheme}://${host}:${port}/healthz"
ok=0
for _ in $(seq 1 60); do
  if body=$(curl -fsS "${curl_tls[@]}" "$url" 2>/dev/null); then
    printf '%s\n' "$body"
    printf '%s\n' "$body" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("status")=="ok" and d.get("db") is True'
    ok=1
    break
  fi
  sleep 0.5
done
if [ "$ok" -ne 1 ]; then
  echo "health check failed: $url" >&2
  tail -n 80 /var/lib/shippin/console.log 2>/dev/null || journalctl -u shippin-console -n 80 --no-pager || true
  exit 1
fi
echo "panel health ok at $url"

# First owner. While no owner exists, every run issues a fresh one-time setup code (valid 24 hours)
# and invalidates any earlier unredeemed one. Once an owner exists nothing is issued.
setup_out=$(set -a; . /etc/shippin/console.env; set +a; "$prefix/shippin-console" setup)
code=${setup_out#setup_code=}
code=${code%%$'\n'*}
if [ -n "$code" ]; then
  printf 'setup_code=%s\n' "$code"
  if [ "$example" -eq 0 ]; then
    echo "Redeem it once at https://<console host>/setup to create the owner (docs/deploy.md, First owner)."
  fi
else
  echo "an owner already exists; no setup code issued"
fi
if [ "$example" -eq 1 ]; then
  [ -n "$code" ] || { echo "the disposable install expected a setup code" >&2; exit 1; }
  base="${scheme}://${host}:${port}"
  setup_json=$(curl -fsS "${curl_tls[@]}" -H 'content-type: application/json' -d "{\"code\":\"${code}\",\"email\":\"owner@example.invalid\",\"name\":\"Owner\"}" "$base/v1/setup")
  printf '%s\n' "$setup_json" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["user_id"] and len(d["recovery_codes"])==8; print("setup_ok")'
  sid=$(printf '%s\n' "$setup_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"])')
  curl -fsS "${curl_tls[@]}" -H 'content-type: application/json' -d '{"email":"wait@example.invalid","name":"Wait","company":"Example"}' "$base/v1/waitlist" >/dev/null
  token=$(python3 -c 'import json; print(json.load(open("/var/lib/shippin/mail-sink.log"))["body"].split()[-1])')
  curl -fsS "${curl_tls[@]}" -H 'content-type: application/json' -d "{\"token\":\"${token}\"}" "$base/v1/waitlist/confirm" >/dev/null
  curl -fsS "${curl_tls[@]}" -H "Cookie: shippin_session=${sid}" "$base/v1/admin/waitlist.csv" | python3 -c 'import sys; body=sys.stdin.read(); assert "wait@example.invalid" in body and "confirmed" in body; print(body.strip())'
fi
