#!/bin/bash
# Starts the panel in its container. Everything an installation chooses comes from the environment
# (config/site.env.example lists it). What must survive a rebuild lives in /var/lib/shippin.
set -euo pipefail
state=/var/lib/shippin
root=/opt/shippin
umask 077

: "${SHIPPIN_DATABASE_URL:?set SHIPPIN_DATABASE_URL}"
if [ -z "${SHIPPIN_SESSION_KEY:-}" ]; then
  [ -s "$state/session.key" ] || openssl rand -hex 32 > "$state/session.key"
  SHIPPIN_SESSION_KEY=$(cat "$state/session.key")
fi

# With no site names set this is a local trial: serve localhost and offer no second operator origin.
export SHIPPIN_CONSOLE_HOST=${SHIPPIN_CONSOLE_HOST:-localhost}
export SHIPPIN_DOMAIN=${SHIPPIN_DOMAIN:-$SHIPPIN_CONSOLE_HOST}
export SHIPPIN_OPERATOR_HOST=${SHIPPIN_OPERATOR_HOST:-none}

# Session cookies are Secure and writes must come from an https origin, so the panel needs TLS.
# A proxy in front can terminate it instead: set SHIPPIN_TLS=proxy.
if [ "${SHIPPIN_TLS:-}" = proxy ]; then
  SHIPPIN_TLS_CERT=""
  SHIPPIN_TLS_KEY=""
elif [ -z "${SHIPPIN_TLS_CERT:-}" ]; then
  SHIPPIN_TLS_CERT=$state/tls/panel.crt
  SHIPPIN_TLS_KEY=$state/tls/panel.key
  if [ ! -s "$SHIPPIN_TLS_CERT" ] || [ ! -s "$SHIPPIN_TLS_KEY" ]; then
    mkdir -p "$state/tls"
    if printf '%s' "$SHIPPIN_CONSOLE_HOST" | grep -Eq '^[0-9.]+$'; then san="IP:$SHIPPIN_CONSOLE_HOST"; else san="DNS:$SHIPPIN_CONSOLE_HOST"; fi
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 825 \
      -subj "/CN=$SHIPPIN_CONSOLE_HOST" -addext "subjectAltName=$san,DNS:localhost,IP:127.0.0.1" \
      -keyout "$SHIPPIN_TLS_KEY" -out "$SHIPPIN_TLS_CERT" 2>/dev/null
  fi
fi
export SHIPPIN_SESSION_KEY SHIPPIN_TLS_CERT SHIPPIN_TLS_KEY

SHIPPIN_CONFIG_OUT=$state/shippin.json python3 "$root/install/panel/render-config.py" "$root"
export SHIPPIN_CONFIG=$state/shippin.json
export SHIPPIN_STATIC=$root/web/dist

# While no owner exists this issues a fresh one-time code on every start; afterwards it prints nothing.
code=$(shippin-console setup | sed -n 's/^setup_code=//p')
if [ -n "$code" ]; then
  echo "setup_code=$code"
  echo "No owner yet. Open https://$SHIPPIN_CONSOLE_HOST:${SHIPPIN_PORT:-8443}/setup and enter that code. It works once and expires in 24 hours."
fi

exec shippin-console
