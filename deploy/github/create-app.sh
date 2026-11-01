#!/bin/bash
# Writes a one-page form that creates the Shippin GitHub App from app-manifest.json (manifest flow).
# Usage: ./create-app.sh <github-org> [app-name] [out.html]. Open the page while signed in to GitHub
# as an owner of <github-org>, press the button, then run convert-manifest.sh with the code GitHub
# appends to the redirect URL. Reads the console host from config/shippin.example.json. Holds no secrets.
set -euo pipefail
D=$(cd "$(dirname "$0")" && pwd); ORG=${1:?github org}; NAME=${2:-Shippin}; OUT=${3:-create-shippin-github-app.html}
[[ $ORG =~ ^[A-Za-z0-9-]{1,39}$ ]] || { echo "org must be a GitHub login" >&2; exit 2; }
[[ $NAME =~ ^[A-Za-z0-9\ ._-]{1,34}$ ]] || { echo "app name: up to 34 letters, digits, space . _ -" >&2; exit 2; }
HOST=$(jq -er .console_host "$D/../../config/shippin.example.json")
M=$(sed -e "s#__CONSOLE_HOST__#$HOST#g" -e "s#__APP_NAME__#$NAME#g" "$D/app-manifest.json" | jq -c .)
ESC=$(jq -rn --arg m "$M" '$m | @html')
STATE=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')
cat > "$OUT" <<HTML
<!doctype html><meta charset="utf-8"><title>Create the $NAME GitHub App</title>
<form action="https://github.com/organizations/$ORG/settings/apps/new?state=$STATE" method="post">
<p>Creates the GitHub App <b>$NAME</b> owned by <b>$ORG</b>, pointing at https://$HOST.</p>
<input type="hidden" name="manifest" value="$ESC"><button type="submit">Create GitHub App</button></form>
HTML
echo "wrote $OUT (state $STATE; GitHub echoes it back with the code)"
