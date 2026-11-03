#!/bin/bash
# hostd-rollback.sh [ID] - switch shippin-hostd back to the previous release (or to ID): binary,
# hostd.json and unit together, restart, health-check. Same env as hostd-install.sh.
set -euo pipefail
D=${HOSTD_RELEASES:-/usr/local/lib/shippin-hostd/releases}; HERE=$(dirname "$0")
"$HERE/release/switch.sh" "$D" "${1:---rollback}"
"$HERE/hostd-activate.sh"
if "$HERE/hostd-health.sh"; then echo "hostd rolled back"; else echo "hostd unhealthy after rollback; journalctl -u shippin-hostd" >&2; exit 1; fi
