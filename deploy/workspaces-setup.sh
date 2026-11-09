#!/bin/bash
# One-time setup for workspaces on the host: tenant key store, console DB role + session key (sops),
# shippin-hostd service, and re-pointing the console proxy device to it. Prints no secrets.
set -euo pipefail
cd /var/lib/shippin/site; R=${1:?release dir}; P="--project platform"
S=secrets/platform-core.sops.yaml; K=secrets/zfs-keys.sops.yaml
if [ ! -f $K ]; then printf 'created: "%s"\n' "$(date -u +%F)" > $K && sops -e -i $K; echo "key store created"; fi
gen(){ sops decrypt --extract "[\"$1\"]" $S >/dev/null 2>&1 || { python3 -c "import secrets,json;print(json.dumps(secrets.token_hex(32)))" | sops set --value-stdin $S "[\"$1\"]"; echo "$1 created"; }; }
gen console_db_password; gen console_session_key
PW=$(sops decrypt --extract '["console_db_password"]' $S)
incus exec pg $P -- su postgres -c "psql -q -d shippin" <<SQL >/dev/null
DO \$\$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='console') THEN CREATE ROLE console LOGIN; END IF;
END \$\$;
ALTER ROLE console PASSWORD '$PW';
GRANT CONNECT ON DATABASE shippin TO console;
GRANT USAGE, CREATE ON SCHEMA public TO console;
SQL
unset PW; echo "db role console ready"
systemctl daemon-reload
if systemctl is-enabled shippin-incus-ro >/dev/null 2>&1; then systemctl disable --now shippin-incus-ro; echo "incus-ro disabled (binary kept)"; fi
"$R/deploy/hostd-install.sh" "$R"   # versioned install + health check (rollback: hostd-rollback.sh)
systemctl enable shippin-hostd >/dev/null 2>&1
incus config device remove console incusro $P </dev/null >/dev/null 2>&1 || true
incus config device show console $P </dev/null | grep -q '^hostd:' || \
  incus config device add console hostd proxy $P listen=tcp:127.0.0.1:8444 connect=unix:/run/shippin/hostd.sock bind=instance </dev/null
echo "hostd ready"
