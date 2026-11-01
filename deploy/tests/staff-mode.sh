#!/bin/bash
# staff-mode.sh MODE "reason" - ops switch of the staff authz mode on Gold (legacy|shadow|enforce),
# audit row written in the same transaction. The IAM_OWNER fallback is left as it is.
# Use the Team > Settings page instead whenever an Owner can sign in with MFA.
set -euo pipefail
M=$1; WHY=$2
case $M in legacy|shadow|enforce) ;; *) echo "mode?"; exit 2;; esac
[ ${#WHY} -ge 10 ] || { echo "reason too short"; exit 2; }
incus exec pg --project platform -- su postgres -c "psql -d shippin -v ON_ERROR_STOP=1 -v m='$M' -v why='$WHY'" <<'SQL'
BEGIN;
INSERT INTO audit_log(actor, actor_name, action, detail)
  SELECT 'ops', 'shippin-ops (deploy/tests/staff-mode.sh)', 'team.settings',
         'mode ' || (data->>'mode') || ' → ' || :'m' || ' · ' || :'why' FROM staff_settings WHERE id=1;
UPDATE staff_settings SET data = jsonb_set(data, '{mode}', to_jsonb(:'m'::text)),
  updated_by='ops', updated_at=now() WHERE id=1;
COMMIT;
SELECT data->>'mode', data->>'legacy_fallback' FROM staff_settings;
SQL
echo "restart not needed; settings are re-read within 5 s"
