#!/bin/bash
# First boot of an empty database only. The superuser stays inside this container; the panel gets its
# own login, which owns its database and nothing else.
set -euo pipefail
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v app="$SHIPPIN_DB_USER" -v password="$SHIPPIN_DB_PASSWORD" -v db="$POSTGRES_DB" <<'SQL'
CREATE ROLE :"app" LOGIN PASSWORD :'password' NOSUPERUSER NOCREATEDB NOCREATEROLE;
ALTER DATABASE :"db" OWNER TO :"app";
SQL
