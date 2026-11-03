#!/bin/sh
# load-ownership-model.sh RELEASE_DIR [check] - run INSIDE the console container (R-J1a).
# Default: upload the release's deploy/openfga/model.json as a new authorization model in the
# configured store (models are immutable; the pinned shadow model is untouched) and point ownership
# writes at it with one console.env line SHIPPIN_FGA_OWNERSHIP_MODEL_ID=<id>. Prints only the id.
# check: read that model back (exit 0 only on 200 with type org). The OpenFGA token never reaches a
# command line: printf (a shell builtin) writes the header into a 0600 file that curl reads with -H @file.
# Backup: console.env.bak-rj1a (first run only).
set -eu
R=${1:?release dir}; MODE=${2:-load}; ENVF=${CONSOLE_ENV:-/etc/shippin/console.env}
CFG="$R/config/shippin.example.json"
URL=$(jq -er '.authz.url' "$CFG"); STORE=$(jq -er '.authz.store_id' "$CFG")
TOKEN=$(sed -n 's/^SHIPPIN_FGA_TOKEN=//p' "$ENVF" | tail -n1)
umask 077; H=$(mktemp "${TMPDIR:-/run}/fga-hdr.XXXXXX"); trap 'rm -f "$H"' EXIT
if [ -n "$TOKEN" ]; then printf 'Authorization: Bearer %s\n' "$TOKEN" > "$H"; fi
printf 'Content-Type: application/json\n' >> "$H"
if [ "$MODE" = check ]; then
  ID=$(sed -n 's/^SHIPPIN_FGA_OWNERSHIP_MODEL_ID=//p' "$ENVF" | tail -n1)
  printf '%s\n' "$ID" | grep -Eqx '[0-9A-HJKMNP-TV-Z]{26}'
  curl -fsS -m10 -H @"$H" "$URL/stores/$STORE/authorization-models/$ID" | jq -e '[.authorization_model.type_definitions[].type] | index("org") != null' >/dev/null
  exit 0
fi
[ -f "$ENVF.bak-rj1a" ] || cp -p "$ENVF" "$ENVF.bak-rj1a"
ID=$(curl -fsS -m10 -X POST -H @"$H" --data-binary @"$R/deploy/openfga/model.json" \
  "$URL/stores/$STORE/authorization-models" | jq -er '.authorization_model_id')
printf '%s\n' "$ID" | grep -Eqx '[0-9A-HJKMNP-TV-Z]{26}'
T=$(mktemp "$ENVF.XXXXXX"); grep -v '^SHIPPIN_FGA_OWNERSHIP_MODEL_ID=' "$ENVF" > "$T" || true
printf 'SHIPPIN_FGA_OWNERSHIP_MODEL_ID=%s\n' "$ID" >> "$T"; chmod --reference="$ENVF" "$T"; chown --reference="$ENVF" "$T"; mv "$T" "$ENVF"
echo "$ID"
