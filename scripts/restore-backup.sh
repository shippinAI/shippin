#!/bin/bash
# Restores a panel backup into this Compose install, replacing everything in its database.
#   scripts/restore-backup.sh <backup.dump> --i-understand
set -euo pipefail
file=${1:?usage: restore-backup.sh <backup.dump> --i-understand}
[ "${2:-}" = --i-understand ] || { echo "This replaces the whole database. Run it again with --i-understand." >&2; exit 2; }
[ -s "$file" ] || { echo "no backup at $file" >&2; exit 1; }
cd "$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
compose() { docker compose "$@"; }
python3 install/docker/configure.py 2>/dev/null || python install/docker/configure.py
user=$(sed -n 's/^SHIPPIN_DB_USER=//p' .env | tail -n1); user=${user:-shippin}
db=$(sed -n 's/^SHIPPIN_DB_NAME=//p' .env | tail -n1); db=${db:-shippin}
project=$(compose config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["name"])' 2>/dev/null || compose config --format json | python -c 'import json,sys; print(json.load(sys.stdin)["name"])')
compose stop panel
compose rm -sf db db-secure
docker volume rm "${project}_db"
compose up -d --wait db
compose cp "$file" db:/tmp/restore.dump
compose exec -T db pg_restore -U postgres -d "$db" --no-owner --no-privileges --role="$user" --exit-on-error /tmp/restore.dump
compose exec -T db rm -f /tmp/restore.dump
compose up -d --build
echo "Restored. The panel is starting."
