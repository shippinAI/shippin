#!/bin/bash
# ws-port-isolation.sh TARGET_TENANT TARGET_WS OTHER_TENANT OTHER_WS - run on Gold (read-only).
# openvscode-server runs --without-connection-token on :IDE_PORT, which is safe only while the
# port is reachable through the edge forward_auth alone. This checks that a workspace of another
# tenant cannot open TARGET_WS's IDE port directly.
# Env: IDE_PORT (default from /etc/shippin/hostd.json edge.ide_port), HOSTD_CONFIG.
set -euo pipefail
T=${1:?target tenant}; W=${2:?target workspace}; OT=${3:?other tenant}; OW=${4:?other workspace}
CFG=${HOSTD_CONFIG:-/etc/shippin/hostd.json}
PORT=${IDE_PORT:-$(jq -r '.edge.ide_port' "$CFG")}
inner() { # inner TENANT WS ARGS... : run incus inside the tenant VM against the workspace's project
  local t=$1 w=$2; shift 2
  local pj; pj=$(incus exec "tvm-$t" --project "t-$t" -- incus list --all-projects --format=json "^w-$w\$" | jq -r '.[0].project')
  incus exec "tvm-$t" --project "t-$t" -- incus --project="$pj" "$@"
}
IP=$(inner "$T" "$W" list --format=json "^w-$W\$" | jq -r '.[0].state.network.eth0.addresses[] | select(.family=="inet" and .scope=="global") | .address' | head -1)
[ -n "$IP" ] || { echo "FAIL: target has no address"; exit 1; }
fails=0
if inner "$OT" "$OW" exec "w-$OW" -- timeout 5 bash -c "</dev/tcp/$IP/$PORT" 2>/dev/null; then
  echo "FAIL: other tenant's workspace reached $IP:$PORT"; fails=1
else echo "ok: other tenant cannot reach the IDE port"; fi
# The host routes to tenant subnets for the edge, so this one is informational, not a failure.
if timeout 5 bash -c "</dev/tcp/$IP/$PORT" 2>/dev/null; then
  echo "info: the host itself reaches $IP:$PORT (expected: the edge proxies through it)"
else echo "info: the host itself cannot reach $IP:$PORT"; fi
exit $fails
