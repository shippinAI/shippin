#!/bin/bash
# Runs after the panel is healthy, so its migrations have applied. Hands the audit log to a role nobody
# can log in as and installs the hash chain, as the superuser. Safe to run on every deploy.
set -euo pipefail
export PGOPTIONS='-c client_min_messages=warning'
{
  printf "BEGIN; SET LOCAL shippin.app_role = '%s';\n" "$SHIPPIN_DB_USER"
  cat /sql/audit_owner.sql /sql/audit_chain.sql
  echo "COMMIT;"
} | psql -q -v ON_ERROR_STOP=1
[ "$(psql -At -c 'SELECT ok FROM shippin_audit_verify()')" = t ] || { echo "the audit log failed its integrity check" >&2; exit 1; }
echo "audit log protected and intact"
