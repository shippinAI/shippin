#!/usr/bin/env bash
# Topology-first rollout helper for Warden.
# Runs on the Proxmox host and upgrades Warden in LXC 102 by default.
#
# Usage:
#   ./rollout-warden.sh dev
#   ./rollout-warden.sh prod
#
# Required:
#   - /tmp/warden-src contains current warden source
#   - pct can access target container
#
# Optional env:
#   - WARDEN_CT (default 102)
#   - WARDEN_HEALTH_URL_DEV  (default https://192.0.2.102:9006/login)
#   - WARDEN_HEALTH_URL_PROD (default https://warden.example.invalid/login)

set -euo pipefail

TARGET="${1:-prod}" # dev | prod
CT="${WARDEN_CT:?set WARDEN_CT}"
SRC="/tmp/warden-src"
LOG="/tmp/warden-rollout-${TARGET}.log"
exec > >(tee "$LOG") 2>&1

if [[ "$TARGET" != "dev" && "$TARGET" != "prod" ]]; then
  echo "usage: $0 [dev|prod]"
  exit 1
fi

DEV_URL="${WARDEN_HEALTH_URL_DEV:?set WARDEN_HEALTH_URL_DEV}"
PROD_URL="${WARDEN_HEALTH_URL_PROD:-https://warden.example.invalid/login}"
CHECK_URL="$DEV_URL"
if [[ "$TARGET" == "prod" ]]; then
  CHECK_URL="$PROD_URL"
fi

echo "=== Warden rollout ($TARGET) @ $(date -Iseconds) ==="

if ! pct status "$CT" >/dev/null 2>&1; then
  echo "FATAL: LXC $CT not found"
  exit 1
fi
if [[ ! -f "$SRC/main.go" ]]; then
  echo "FATAL: $SRC/main.go not found"
  exit 1
fi

EXPECTED_VERSION="$(grep -E '^const Version = ' "$SRC/main.go" | sed -E 's/.*"([^"]+)".*/\1/' | head -1)"
if [[ -z "$EXPECTED_VERSION" ]]; then
  echo "FATAL: could not determine Version from $SRC/main.go"
  exit 1
fi
echo "expected version: $EXPECTED_VERSION"

echo "[1/6] install build deps"
pct exec "$CT" -- bash -lc '
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq golang-go libpam0g-dev gcc make git ca-certificates curl
  go version
'

echo "[2/6] sync source"
pct exec "$CT" -- rm -rf /opt/warden-src
pct exec "$CT" -- mkdir -p /opt/warden-src
tar -C "$SRC" -cf - . | pct exec "$CT" -- tar -C /opt/warden-src -xf -

echo "[3/6] build linux binary in container"
pct exec "$CT" -- bash -lc '
  cd /opt/warden-src
  CGO_ENABLED=1 go build -ldflags="-s -w" -o /usr/local/bin/warden .
  /usr/local/bin/warden --help >/dev/null 2>&1 || true
  ls -la /usr/local/bin/warden
'

echo "[4/6] install unit/helper and preserve env"
pct exec "$CT" -- bash -lc '
  cp /opt/warden-src/warden.service /etc/systemd/system/warden.service
  cp /opt/warden-src/install/warden-infisical-env-refresh.sh /usr/local/bin/warden-infisical-env-refresh
  chmod 700 /usr/local/bin/warden-infisical-env-refresh
  mkdir -p /etc/warden /run/warden
  if [ ! -f /etc/warden/warden.env ]; then
    cp /opt/warden-src/warden.env.example /etc/warden/warden.env || true
    chmod 600 /etc/warden/warden.env || true
    echo "[warn] /etc/warden/warden.env was missing; template copied. fill real values."
  fi
  systemctl daemon-reload
'

echo "[5/6] restart service and check local status"
pct exec "$CT" -- bash -lc '
  systemctl enable warden >/dev/null 2>&1 || true
  systemctl restart warden
  sleep 3
  systemctl is-active warden
  journalctl -u warden -n 20 --no-pager
'

echo "[6/6] verify page version via $CHECK_URL"
PAGE="$(curl -sk --max-time 10 "$CHECK_URL" || true)"
if [[ -z "$PAGE" ]]; then
  echo "FATAL: health page request failed"
  exit 1
fi
LIVE_VERSION="$(echo "$PAGE" | grep -Eo 'v[0-9]+\.[0-9]+(-[a-z0-9]+)?' | head -1 | sed 's/^v//')"
echo "live version: ${LIVE_VERSION:-unknown}"
if [[ "$LIVE_VERSION" != "$EXPECTED_VERSION" ]]; then
  echo "FATAL: rollout mismatch expected=$EXPECTED_VERSION live=${LIVE_VERSION:-unknown}"
  exit 1
fi

echo "ROLL_OUT_OK target=$TARGET version=$EXPECTED_VERSION"
echo "log: $LOG"
