#!/bin/bash
# console-rollback.sh [ID] - switch the console container back to the previous release (or to
# release ID), restart and health-check. The database schema is not rolled back: migrations are
# additive, so the older binary keeps working on the newer schema.
set -euo pipefail
P="--project platform"; PORT=${CONSOLE_PORT:-8080}
X() { incus exec console $P -- "$@" </dev/null; }
X /opt/console/bin/switch.sh /opt/console/releases "${1:---rollback}"
ID=$(X sh -c 'basename "$(readlink /opt/console/releases/current)"')
X systemctl restart console
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if X curl -fsS "localhost:$PORT/healthz" >/dev/null 2>&1; then echo "console back on release $ID"; exit 0; fi
  sleep 2
done
echo "console on release $ID is not healthy either; check: incus exec console $P -- journalctl -u console -n 50" >&2
exit 1
