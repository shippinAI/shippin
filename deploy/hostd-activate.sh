#!/bin/bash
# hostd-activate.sh - point the hostd binary, config and unit at releases/current, then restart.
# Same env as hostd-install.sh. Used by install and rollback.
set -euo pipefail
D=${HOSTD_RELEASES:-/usr/local/lib/shippin-hostd/releases}
BIN=${HOSTD_BIN:-/usr/local/bin/shippin-hostd}; CFG=${HOSTD_CONFIG:-/etc/shippin/hostd.json}
UNIT=${HOSTD_UNIT:-/etc/systemd/system/shippin-hostd.service}; SYSTEMCTL=${SYSTEMCTL:-systemctl}
swap() { ln -sfn "$1" "$2.tmp" && mv -T "$2.tmp" "$2"; }
mkdir -p "$(dirname "$CFG")"
swap "$D/current/shippin-hostd" "$BIN"
[ -f "$D/current/hostd.json" ] && swap "$D/current/hostd.json" "$CFG"
[ -f "$D/current/shippin-hostd.service" ] && install -m644 "$D/current/shippin-hostd.service" "$UNIT"
$SYSTEMCTL daemon-reload
$SYSTEMCTL restart shippin-hostd
