#!/usr/bin/env bash
# Warden install â€” LXC 102 on server1
# Run on the PVE host as root. Source must already be at /tmp/warden-src/
# Captures everything to /tmp/warden-install.log

set -uo pipefail
LOG=/tmp/warden-install.log
exec > >(tee "$LOG") 2>&1

CT=102
echo "================================================================"
echo "  Warden install -> LXC $CT"
echo "  $(date -Iseconds)"
echo "================================================================"

# --- preflight ---
if ! pct status $CT >/dev/null 2>&1; then
  echo "FATAL: LXC $CT not found"; exit 1
fi
if [ ! -d /tmp/warden-src ] || [ ! -f /tmp/warden-src/main.go ]; then
  echo "FATAL: /tmp/warden-src/main.go not found on PVE host"
  echo "Upload source first (scp/rsync from the warden source tree  to /tmp/warden-src/)"
  exit 1
fi
echo "[0] LXC $CT: $(pct status $CT)"
echo "[0] Source: $(ls /tmp/warden-src/*.go | wc -l) .go files, $(ls /tmp/warden-src/templates/*.html 2>/dev/null | wc -l) templates"

# --- 1. install deps inside container ---
echo ""
echo "[1] Installing build deps in LXC $CT..."
pct exec $CT -- bash -c '
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq 2>&1 | tail -3
  apt-get install -y -qq golang-go libpam0g-dev gcc make git ca-certificates curl 2>&1 | tail -3
  go version
' || { echo "deps install FAILED"; exit 1; }

# --- 2. push source ---
echo ""
echo "[2] Pushing source into container..."
pct exec $CT -- rm -rf /opt/warden-src
pct exec $CT -- mkdir -p /opt/warden-src
tar -C /tmp/warden-src -cf - . | pct exec $CT -- tar -C /opt/warden-src -xf -
echo "    container has: $(pct exec $CT -- bash -c 'ls /opt/warden-src/*.go 2>/dev/null | wc -l') .go files"

# --- 3. build ---
echo ""
echo "[3] Building warden binary (CGO for PAM)..."
pct exec $CT -- bash -c '
  cd /opt/warden-src
  CGO_ENABLED=1 go build -ldflags="-s -w" -o /usr/local/bin/warden . 2>&1
  echo "exit: $?"
  ls -la /usr/local/bin/warden 2>/dev/null
' || { echo "build FAILED"; exit 1; }

# --- 4. systemd unit + config dir ---
echo ""
echo "[4] Installing systemd unit and config..."
pct exec $CT -- bash -c '
  cp /opt/warden-src/warden.service /etc/systemd/system/warden.service
  cp /opt/warden-src/install/warden-infisical-env-refresh.sh /usr/local/bin/warden-infisical-env-refresh
  chmod 700 /usr/local/bin/warden-infisical-env-refresh
  mkdir -p /etc/warden
  if [ ! -f /etc/warden/warden.env ]; then
    cat > /etc/warden/warden.env <<ENVEOF
PROXMOX_HOST=
PROXMOX_PORT=8006
PROXMOX_NODE=
PROXMOX_TOKEN_ID=
PROXMOX_TOKEN_SECRET=
PROXMOX_VERIFY_TLS=true
ENVEOF
    echo "[ok] wrote default /etc/warden/warden.env template (edit with real secrets)"
  else
    echo "[ok] preserving existing /etc/warden/warden.env"
  fi
  chmod 600 /etc/warden/warden.env
  mkdir -p /run/warden
  systemctl daemon-reload
  echo "[ok] unit + env file in place (runtime Infisical refresh helper installed)"
'

# --- 5. start ---
echo ""
echo "[5] Enabling + starting warden.service..."
pct exec $CT -- bash -c '
  systemctl enable warden 2>&1
  systemctl start warden
  sleep 3
  echo "is-active: $(systemctl is-active warden)"
  echo ""
  echo "--- last 20 log lines ---"
  journalctl -u warden -n 20 --no-pager
'

# --- 6. health check from PVE host ---
echo ""
echo "[6] Health check from PVE host..."
sleep 1
HTTP=$(curl -sk -o /tmp/warden-test.html -w "%{http_code}" --max-time 5 https://192.0.2.102:9006/login || echo "FAIL")
echo "    GET https://192.0.2.102:9006/login -> HTTP $HTTP"
if [ "$HTTP" = "200" ]; then
  echo "    [OK] Warden is serving"
  grep -oE '<title>[^<]+</title>' /tmp/warden-test.html | head -1
else
  echo "    [WARN] non-200 â€” check journal above"
fi

echo ""
echo "================================================================"
echo "  DONE. Log saved at $LOG"
echo "  Internal URL:  https://192.0.2.102:9006"
echo "  Next: reverse proxy on PVE host -> warden.<your-fqdn>"
echo "================================================================"
