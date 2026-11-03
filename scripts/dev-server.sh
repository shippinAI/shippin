#!/bin/bash
# Runs the console from source against a containerized database, at https://localhost:8443.
# See docs/development.md. Keys and certificates stay under .run/, which git ignores.
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root"
port=${SHIPPIN_DEV_DB_PORT:-15432}

docker compose -f compose.yaml -f compose.dev.yaml up -d --wait db

mkdir -p .run/tls
[ -s .run/session.key ] || openssl rand -hex 32 > .run/session.key
if [ ! -s .run/tls/panel.key ]; then
  # MSYS_NO_PATHCONV keeps Git Bash on Windows from rewriting the certificate subject as a path.
  MSYS_NO_PATHCONV=1 openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 825 \
    -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
    -keyout .run/tls/panel.key -out .run/tls/panel.crt 2>/dev/null
fi

export SHIPPIN_CONFIG=${SHIPPIN_CONFIG:-config/shippin.example.json}
export SHIPPIN_STATIC=${SHIPPIN_STATIC:-web/dist}
export SHIPPIN_LISTEN=${SHIPPIN_LISTEN:-:8443}
export SHIPPIN_DATABASE_URL=${SHIPPIN_DATABASE_URL:-postgres://shippin:${SHIPPIN_DB_PASSWORD:-shippin-local-only}@127.0.0.1:$port/shippin?sslmode=disable}
SHIPPIN_SESSION_KEY=$(cat .run/session.key)
export SHIPPIN_SESSION_KEY
export SHIPPIN_TLS_CERT=.run/tls/panel.crt SHIPPIN_TLS_KEY=.run/tls/panel.key

code=$(go run ./cmd/console setup | sed -n 's/^setup_code=//p')
if [ -n "$code" ]; then echo "setup_code=$code   (open /setup and enter it)"; fi
exec go run ./cmd/console
