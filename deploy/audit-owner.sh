#!/bin/bash
# Moves audit_log/access_grants (and their trigger function) to the NOLOGIN role
# shippin_audit_owner; the console role keeps SELECT+INSERT (+UPDATE of grant status columns).
# Then installs the audit hash chain (internal/db/audit_chain.sql) in the same transaction.
# Idempotent; run on the Gold host after the console has applied migration 005 (and again after
# any migration that recreates those tables). No password is involved: the owner can't log in,
# and future changes to these tables go through this superuser path, not the app.
#   bash deploy/audit-owner.sh [repo dir]      env: PG_INSTANCE=pg PG_PROJECT=platform APP_ROLE=console
set -euo pipefail
R=${1:-$(cd "$(dirname "$0")/.." && pwd)}
I=${PG_INSTANCE:-pg}; P=(--project "${PG_PROJECT:-platform}"); APP=${APP_ROLE:-console}
[[ "$APP" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "bad APP_ROLE" >&2; exit 1; }
{ echo "BEGIN; SET LOCAL shippin.app_role = '$APP';"; cat "$R/internal/db/audit_owner.sql" "$R/internal/db/audit_chain.sql"; echo "COMMIT;"; } |
  incus exec -T "$I" "${P[@]}" -- su postgres -c "psql -q -v ON_ERROR_STOP=1 -d shippin"
incus exec -T "$I" "${P[@]}" -- su postgres -c "psql -At -d shippin" <<'SQL'
SELECT 'owner ' || tablename || '=' || tableowner FROM pg_tables WHERE tablename IN ('audit_log', 'access_grants') ORDER BY 1;
SELECT 'login shippin_audit_owner=' || rolcanlogin FROM pg_roles WHERE rolname = 'shippin_audit_owner';
SELECT 'chain rows=' || n_rows || ' ok=' || ok || ' head=' || coalesce(head, '') FROM shippin_audit_verify();
SQL
