#!/bin/bash
# hostd-health.sh - wait up to 20 s for shippin-hostd to answer GET /host/stats on its socket
# (socket path from the live hostd.json). HOSTD_HEALTH_CMD overrides the probe (tests).
set -euo pipefail
CFG=${HOSTD_CONFIG:-/etc/shippin/hostd.json}
probe() {
  if [ -n "${HOSTD_HEALTH_CMD:-}" ]; then bash -c "$HOSTD_HEALTH_CMD"; return; fi
  curl -fsS --unix-socket "$(jq -r .socket "$CFG")" http://hostd/host/stats >/dev/null
}
for _ in $(seq 1 10); do probe 2>/dev/null && exit 0; sleep 2; done
exit 1
