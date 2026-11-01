#!/bin/bash
# Render $VAULTIX_SECRETS_DIR from fresh random values (local) or from sops (Gold: SOPS_FILE set).
# Prints nothing secret. usage: secrets-render.sh   (reads .env)
set -euo pipefail
cd "$(dirname "$0")"; set -a; . ./.env; set +a
D=$VAULTIX_SECRETS_DIR; umask 077; mkdir -p "$D"
if [ -n "${SOPS_FILE:-}" ]; then
  sops -d --extract '["vaultix_pg_password"]' "$SOPS_FILE" > "$D/pg_password"
  EK=$(sops -d --extract '["vaultix_encryption_key"]' "$SOPS_FILE"); AS=$(sops -d --extract '["vaultix_auth_secret"]' "$SOPS_FILE")
else
  [ -s "$D/pg_password" ] || openssl rand -hex 24 > "$D/pg_password"
  EK=$(openssl rand -hex 16); AS=$(openssl rand -base64 32)
  [ -s "$D/api.env" ] && { echo "secrets exist in $D (kept)"; exit 0; }
fi
PW=$(cat "$D/pg_password")
printf 'ENCRYPTION_KEY=%s\nAUTH_SECRET=%s\nDB_CONNECTION_URI=postgres://%s:%s@%s:%s/%s\n' "$EK" "$AS" \
  "$VAULTIX_PG_USER" "$PW" "$VAULTIX_BIND" "$VAULTIX_PG_PORT" "$VAULTIX_PG_DB" > "$D/api.env"
chmod 600 "$D"/*; echo "rendered $D (pg_password, api.env; 0600)"
