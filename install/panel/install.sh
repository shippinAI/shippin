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
python3 "$root/install/panel/render-config.py" "$root"

db_is_local=0
if [ -z "${SHIPPIN_DATABASE_URL:-}" ]; then
  db_is_local=1
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

# ClickHouse holds metrics and events (CORE-01). It comes from ClickHouse's signed LTS apt repository, and
# the signing key is checked against its fingerprint before apt trusts it. The panel gets a user that can
# only reach its own database; the built-in passwordless admin is locked. A site that brings its own
# ClickHouse sets SHIPPIN_CLICKHOUSE_URL, and nothing is installed.
clickhouse_signer_fingerprint=3A9EA1193A97B548BE1457D48919F6BD2B48D754
if [ -z "${SHIPPIN_CLICKHOUSE_URL:-}" ]; then
  if ! command -v clickhouse-server >/dev/null 2>&1; then
    apt-get install -y -qq gnupg >/dev/null
    key=$(mktemp)
    curl -fsSL --retry 3 -o "$key" https://packages.clickhouse.com/rpm/lts/repodata/repomd.xml.key
    fingerprint=$(gpg --show-keys --with-colons "$key" 2>/dev/null | awk -F: '$1 == "fpr" {print $10; exit}')
    if [ "$fingerprint" != "$clickhouse_signer_fingerprint" ]; then
      echo "ClickHouse's signing key has fingerprint ${fingerprint:-none}, expected $clickhouse_signer_fingerprint" >&2
      rm -f "$key"
      exit 1
    fi
    gpg --dearmor < "$key" > /usr/share/keyrings/clickhouse-keyring.gpg
    rm -f "$key"
    echo "deb [signed-by=/usr/share/keyrings/clickhouse-keyring.gpg arch=$(dpkg --print-architecture)] https://packages.clickhouse.com/deb lts main" \
      > /etc/apt/sources.list.d/clickhouse.list
    apt-get update -qq
    apt-get install -y -qq clickhouse-server clickhouse-client >/dev/null
  fi
  # A re-run keeps the password, or the panel loses its history store until the next restart.
  ch_pass=$(sed -n 's|^SHIPPIN_CLICKHOUSE_URL=http://shippin:\([0-9a-f]*\)@127\.0\.0\.1:8123/shippin$|\1|p' /etc/shippin/console.env 2>/dev/null | tail -n1 || true)
  if [ -z "$ch_pass" ]; then ch_pass=$(openssl rand -hex 24); fi
  users_d=/etc/clickhouse-server/users.d
  install -d -m 0750 -g clickhouse "$users_d"
  (umask 027 && cat > "$users_d/shippin.xml" <<XML
<clickhouse>
  <users>
    <shippin>
      <password_sha256_hex>$(printf '%s' "$ch_pass" | sha256sum | cut -d' ' -f1)</password_sha256_hex>
      <networks><ip>::1</ip><ip>127.0.0.1</ip></networks>
      <profile>default</profile>
      <quota>default</quota>
      <grants><query>GRANT ALL ON shippin.*</query></grants>
    </shippin>
  </users>
</clickhouse>
XML
  )
  # Lock the package's passwordless default user, unless an administrator already gave it a password.
  if [ ! -f "$users_d/default-password.xml" ] && [ ! -f "$users_d/default-locked.xml" ]; then
    (umask 027 && cat > "$users_d/default-locked.xml" <<XML
<clickhouse>
  <users>
    <default>
      <password remove="remove"/>
      <password_sha256_hex>$(openssl rand -hex 32 | sha256sum | cut -d' ' -f1)</password_sha256_hex>
      <networks replace="replace"><ip>::1</ip><ip>127.0.0.1</ip></networks>
    </default>
  </users>
</clickhouse>
XML
    )
  fi
  chgrp clickhouse "$users_d"/*.xml
  if [ -d /run/systemd/system ]; then
    systemctl enable clickhouse-server.service >/dev/null 2>&1
    systemctl restart clickhouse-server.service
  elif ! curl -fsS http://127.0.0.1:8123/ping >/dev/null 2>&1; then
    install -d -o clickhouse -g clickhouse /run/clickhouse-server
    runuser -u clickhouse -- clickhouse-server --config-file=/etc/clickhouse-server/config.xml \
      --pid-file=/run/clickhouse-server/clickhouse-server.pid --daemon
  fi
  for _ in $(seq 1 60); do curl -fsS http://127.0.0.1:8123/ping >/dev/null 2>&1 && break; sleep 1; done
  # The password goes to curl on stdin, never on a command line.
  ch_ok=0
  for _ in $(seq 1 30); do
    if printf 'header = "X-ClickHouse-User: shippin"\nheader = "X-ClickHouse-Key: %s"\n' "$ch_pass" \
      | curl -fsS -K - --data-binary 'CREATE DATABASE IF NOT EXISTS shippin' http://127.0.0.1:8123/ >/dev/null; then
      ch_ok=1
      break
    fi
    sleep 1
  done
  [ "$ch_ok" -eq 1 ] || { echo "ClickHouse did not accept the panel's user" >&2; exit 1; }
  SHIPPIN_CLICKHOUSE_URL="http://shippin:${ch_pass}@127.0.0.1:8123/shippin"
  echo "ClickHouse ready: $(clickhouse-server --version 2>/dev/null | head -n1)"
fi
# The session key signs sessions and seals secrets saved in the panel, so a re-run must keep it: a new
# key signs everyone out and leaves every saved host secret unreadable.
if [ -z "${SHIPPIN_SESSION_KEY:-}" ] && [ -f /etc/shippin/console.env ]; then
  SHIPPIN_SESSION_KEY=$(sed -n 's/^SHIPPIN_SESSION_KEY=//p' /etc/shippin/console.env | tail -n1)
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
SHIPPIN_DEPLOY_METHOD=installer
SHIPPIN_SELF_GUEST=${SHIPPIN_SELF_GUEST:-}
SHIPPIN_SOURCE_DIR=${root}
SHIPPIN_SITE_FILE=${site}
SHIPPIN_STATIC=${root}/web/dist
SHIPPIN_LISTEN=${SHIPPIN_LISTEN}
SHIPPIN_DATABASE_URL=${SHIPPIN_DATABASE_URL}
SHIPPIN_CLICKHOUSE_URL=${SHIPPIN_CLICKHOUSE_URL}
SHIPPIN_SESSION_KEY=${SHIPPIN_SESSION_KEY}
SHIPPIN_OIDC_CLIENT_ID=${SHIPPIN_OIDC_CLIENT_ID:-}
SHIPPIN_OPS_OIDC_CLIENT_ID=${SHIPPIN_OPS_OIDC_CLIENT_ID:-}
SHIPPIN_SMTP_PASSWORD=${SHIPPIN_SMTP_PASSWORD:-}
SHIPPIN_MAIL_SINK=${SHIPPIN_MAIL_SINK:-}
SHIPPIN_TLS_CERT=${SHIPPIN_TLS_CERT}
SHIPPIN_TLS_KEY=${SHIPPIN_TLS_KEY}
SHIPPIN_CAPTCHA_SECRET=${SHIPPIN_CAPTCHA_SECRET:-}
SHIPPIN_OIDC_CLIENT_SECRET=${SHIPPIN_OIDC_CLIENT_SECRET:-}
SHIPPIN_SSO_SECRET=${SHIPPIN_SSO_SECRET:-}
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
After=network.target postgresql.service clickhouse-server.service

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
    printf '%s\n' "$body" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("status")=="ok" and d.get("db") is True and d.get("clickhouse")=="up", d'
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

# The audit log must not be something the panel's own database role can rewrite. This hands it to a role
# nobody can log in as, and adds the hash chain. It needs a superuser, and it has to run after the panel
# has applied its migrations. A database the site brought itself is the site's to protect.
if [ "$db_is_local" -eq 1 ]; then
  { echo "BEGIN; SET LOCAL shippin.app_role = 'shippin';"; cat "$root/internal/db/audit_owner.sql" "$root/internal/db/audit_chain.sql"; echo "COMMIT;"; } \
    | runuser -u postgres -- psql -q -v ON_ERROR_STOP=1 -d shippin
  audit_ok=$(runuser -u postgres -- psql -At -d shippin -c "SELECT ok FROM shippin_audit_verify()")
  [ "$audit_ok" = "t" ] || { echo "the audit log failed its integrity check" >&2; exit 1; }
  echo "audit log protected and intact"
else
  echo "this database was not created by the installer, so the audit log was not protected; see docs/deploy.md"
fi

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
  example_password=$(openssl rand -hex 24)
  setup_json=$(curl -fsS "${curl_tls[@]}" -H 'content-type: application/json' -d "{\"code\":\"${code}\",\"email\":\"owner@example.invalid\",\"name\":\"Owner\",\"username\":\"owner\",\"password\":\"${example_password}\",\"organization\":\"Example organization\",\"brand\":{\"name\":\"Shippin\"}}" "$base/v1/setup")
  printf '%s\n' "$setup_json" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["user_id"] and len(d["recovery_codes"])==8; print("setup_ok")'
  sid=$(printf '%s\n' "$setup_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"])')
  curl -fsS "${curl_tls[@]}" -H 'content-type: application/json' -d '{"email":"wait@example.invalid","name":"Wait","company":"Example"}' "$base/v1/waitlist" >/dev/null
  token=$(python3 -c 'import json; print(json.load(open("/var/lib/shippin/mail-sink.log"))["body"].split()[-1])')
  curl -fsS "${curl_tls[@]}" -H 'content-type: application/json' -d "{\"token\":\"${token}\"}" "$base/v1/waitlist/confirm" >/dev/null
  curl -fsS "${curl_tls[@]}" -H "Cookie: shippin_session=${sid}" "$base/v1/admin/waitlist.csv" | python3 -c 'import sys; body=sys.stdin.read(); assert "wait@example.invalid" in body and "confirmed" in body; print(body.strip())'
fi
