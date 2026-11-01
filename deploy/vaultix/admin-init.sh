#!/bin/bash
# Create the first Vaultix instance admin (the platform operator) with Infisical's own SRP helper,
# inside the api container. Password comes from $VAULTIX_SECRETS_DIR/admin_password (0600; made here
# if missing), the e-mail from $VAULTIX_ADMIN_EMAIL. Prints ids only. Idempotent: refuses if set up.
set -euo pipefail
cd "$(dirname "$0")"; set -a; . ./.env; set +a
: "${VAULTIX_ADMIN_EMAIL:?set VAULTIX_ADMIN_EMAIL}"
C=${VAULTIX_API_CONTAINER:-vaultix_api_1}; D=$VAULTIX_SECRETS_DIR; umask 077
[ -s "$D/admin_password" ] || openssl rand -base64 24 | tr -d '/+=' > "$D/admin_password"
podman cp admin-init.ts "$C:/backend/src/admin-init.ts"
jq -n --arg e "$VAULTIX_ADMIN_EMAIL" --rawfile p "$D/admin_password" --arg port "$VAULTIX_API_PORT" \
  '{email:$e,password:($p|rtrimstr("\n")),port:$port}' |
  podman exec -i -e FIPS_ENABLED=false "$C" sh -c 'cd /backend && node_modules/.bin/tsx --tsconfig tsconfig.json src/admin-init.ts' |
  grep '^{"status"'
podman exec -u 0 "$C" rm -f /backend/src/admin-init.ts
jq -n --arg e "$VAULTIX_ADMIN_EMAIL" --rawfile p "$D/admin_password" '{email:$e,password:($p|rtrimstr("\n"))}' > "$D/operator.json"
echo "operator login written to $D/operator.json (0600)"
