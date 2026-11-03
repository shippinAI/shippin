#!/bin/bash
# Create/update Incus container "console" (project platform) and install a versioned release.
# Usage on Gold: CLIENT_ID=... ./console-container.sh /path/to/release-dir   (from release/build.sh)
# KEEP_ENV=1 keeps the live /etc/shippin/console.env (no sops, no CLIENT_ID) and only repoints its paths.
# GITHUB_ENV=1 (with KEEP_ENV=1) also replaces only its SHIPPIN_GITHUB_* lines from sops github_* keys.
# GITHUB_ENV=1 writes exactly five keys, all from sops github_* on Gold, never printed: SHIPPIN_GITHUB_APP_ID, _CLIENT_ID,
#   _CLIENT_SECRET, _WEBHOOK_SECRET, _PRIVATE_KEY (base64). Every other console.env line is kept byte-for-byte (grep -v ^SHIPPIN_GITHUB_).
# Layout in the container: /opt/console/releases/<id>/ (console, web/dist, config, RELEASE),
# releases/current -> <id>, releases/previous -> the release before it, /opt/console/current -> releases/current.
# A failed health check switches back to the previous release automatically.
# Rollback by hand: ./console-rollback.sh. Prints no secrets.
set -euo pipefail
P="--project platform"; R=${1:?release dir}; IP=${CONSOLE_IP:?set CONSOLE_IP}; PORT=${CONSOLE_PORT:-8080}
KEEP=${KEEP:-5}; S=${SECRETS_FILE:-/var/lib/shippin/site/secrets/platform-core.sops.yaml}
ID=$(sed -n 's/^id=//p' "$R/RELEASE" 2>/dev/null); ID=${ID:-$(basename "$R")}
RD=/opt/console/releases/$ID
X() { incus exec console $P -- "$@" </dev/null; }
# GitHub App credentials (M3), each only if present in sops; the PEM key goes in as one-line base64.
gh_env() {
  for k in app_id:APP_ID client_id:CLIENT_ID client_secret:CLIENT_SECRET webhook_secret:WEBHOOK_SECRET; do
    if t=$(sops decrypt --extract "[\"github_${k%%:*}\"]" $S 2>/dev/null); then printf 'SHIPPIN_GITHUB_%s=%s\n' "${k#*:}" "$t"; fi
  done
  if t=$(sops decrypt --extract '["github_private_key"]' $S 2>/dev/null); then printf 'SHIPPIN_GITHUB_PRIVATE_KEY=%s\n' "$(printf '%s\n' "$t" | base64 -w0)"; fi
}
if ! incus info console $P >/dev/null 2>&1 </dev/null; then
  incus launch -q images:debian/13 console $P -d eth0,ipv4.address=$IP -c limits.cpu=2 -c limits.memory=1GiB -c boot.autostart=true </dev/null
  sleep 8
  X sh -c 'useradd -r -s /usr/sbin/nologin console; mkdir -p /opt/console /etc/shippin'
fi
if [ -n "${IDP_IP:-}" ] && [ -n "${IDP_HOST:-}" ]; then
  X sh -c "grep -q ' ${IDP_HOST}' /etc/hosts || echo '${IDP_IP} ${IDP_HOST}' >> /etc/hosts"
fi
X sh -c 'command -v curl >/dev/null || apt-get -qq install -y curl >/dev/null 2>&1'
incus exec edge $P -- cat /var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt </dev/null > /tmp/edge-root.crt
incus file push /tmp/edge-root.crt console/etc/shippin/edge-root.crt $P
# Shippin Mail's internal submission certificate (public part), pinned by the console's SMTP client.
MAILCA=${MAIL_CA:-/var/lib/shippin/site/shippin-mail/tls.crt}
if [ -f "$MAILCA" ]; then incus file push "$MAILCA" console/etc/shippin/mail-ca.crt $P --mode 0644; fi
# Release scripts + the release itself (an existing id is replaced: rebuilds of the same id).
X mkdir -p /opt/console/releases /opt/console/bin
for f in switch.sh prune.sh; do incus file push "$R/deploy/release/$f" "console/opt/console/bin/$f" $P --mode 0755; done
X rm -rf "$RD.tmp"; X mkdir -p "$RD.tmp/web"
incus file push -r -p "$R/web/dist" "console$RD.tmp/web/" $P
incus file push -r -p "$R/config" "console$RD.tmp/" $P
incus file push "$R/console" "console$RD.tmp/console" $P --mode 0755
if [ -f "$R/chat-probe" ]; then incus file push "$R/chat-probe" "console$RD.tmp/chat-probe" $P --mode 0755; fi
incus file push "$R/RELEASE" "console$RD.tmp/RELEASE" $P --mode 0644
X sh -c "rm -rf '$RD' && mv '$RD.tmp' '$RD'"
X sh -c "[ ! -f /etc/systemd/system/console.service ] || cp -p /etc/systemd/system/console.service /etc/systemd/system/console.service.bak-$ID"
incus file push "$R/deploy/console.service" console/etc/systemd/system/console.service $P
if [ "${KEEP_ENV:-}" = 1 ]; then
  # Keep the live console.env (secrets untouched, never read here); back it up and point only
  # its config/static paths at /opt/console/current.
  X sh -c "cp -p /etc/shippin/console.env /etc/shippin/console.env.bak-$ID &&
    sed -i -e 's#^SHIPPIN_CONFIG=.*#SHIPPIN_CONFIG=/opt/console/current/config/shippin.example.json#' \
      -e 's#^SHIPPIN_STATIC=.*#SHIPPIN_STATIC=/opt/console/current/web/dist#' /etc/shippin/console.env"
  if [ "${GITHUB_ENV:-}" = 1 ]; then
    ( umask 077; gh_env ) > /tmp/console-gh.env
    incus file push /tmp/console-gh.env console/etc/shippin/console-gh.env $P --mode 0600; shred -u /tmp/console-gh.env
    X sh -c 'umask 077; E=/etc/shippin; { grep -v "^SHIPPIN_GITHUB_" $E/console.env; cat $E/console-gh.env; } > $E/console.env.new &&
      mv $E/console.env.new $E/console.env && shred -u $E/console-gh.env'
  fi
else
  ( umask 077
    printf 'SHIPPIN_CONFIG=/opt/console/current/config/shippin.example.json\nSHIPPIN_STATIC=/opt/console/current/web/dist\nSHIPPIN_LISTEN=:%s\nSHIPPIN_OIDC_CLIENT_ID=%s\n' "$PORT" "${CLIENT_ID:?}"
    printf 'SHIPPIN_DATABASE_URL=postgres://console:%s@%s/shippin\n' "$(sops decrypt --extract '["console_db_password"]' $S)" "${PG_ADDR:?set PG_ADDR}"
    printf 'SHIPPIN_SESSION_KEY=%s\n' "$(sops decrypt --extract '["console_session_key"]' $S)"
    for k in idp_service_token:IDP_SERVICE_TOKEN idp_read_token:IDP_READ_TOKEN idp_team_token:IDP_TEAM_TOKEN; do
      if t=$(sops decrypt --extract "[\"console_${k%%:*}\"]" $S 2>/dev/null); then printf 'SHIPPIN_%s=%s\n' "${k#*:}" "$t"; fi
    done
    if t=$(sops decrypt --extract '["mail_notices_password"]' $S 2>/dev/null); then printf 'SHIPPIN_SMTP_PASSWORD=%s\n' "$t"; fi
    gh_env
  ) > /tmp/console.env
  incus file push /tmp/console.env console/etc/shippin/console.env $P --mode 0600
  shred -u /tmp/console.env
fi
# First versioned deploy: adopt the old flat layout (/opt/console/console) as a release, so the
# automatic rollback has somewhere to go.
X sh -c 'L=00000000000000-pre-versioned; D=/opt/console/releases
  if [ -x /opt/console/console ] && [ ! -e $D/current ]; then
    mkdir -p $D/$L/web && cp -a /opt/console/console /opt/console/config $D/$L/ && cp -a /opt/console/web/dist $D/$L/web/ &&
    echo id=$L > $D/$L/RELEASE && ln -sfn $L $D/current; fi'
X /opt/console/bin/switch.sh /opt/console/releases "$ID"
X sh -c 'ln -sfn releases/current /opt/console/current.tmp && mv -T /opt/console/current.tmp /opt/console/current'
X sh -c 'systemctl daemon-reload && systemctl enable console >/dev/null 2>&1 && systemctl restart console'
healthy() { for _ in 1 2 3 4 5 6 7 8 9 10; do X curl -fsS "localhost:$PORT/healthz" >/dev/null 2>&1 && return 0; sleep 2; done; return 1; }
if healthy; then
  X /opt/console/bin/prune.sh /opt/console/releases "$KEEP"
  echo "console release $ID is live"
else
  echo "console release $ID failed its health check; rolling back" >&2
  "$(dirname "$0")/console-rollback.sh"; exit 1
fi
