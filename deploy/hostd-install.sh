#!/bin/bash
# hostd-install.sh RELEASE_DIR - install shippin-hostd from a release (release/build.sh) as a
# versioned release on Gold: $HOSTD_RELEASES/<id>/{shippin-hostd,rrflow-state,hostd.json,shippin-hostd.service},
# with $HOSTD_BIN and $HOSTD_CONFIG symlinked through releases/current. The first run adopts the
# installed binary + config as release 00000000000000-pre-versioned (rollback target).
# Refuses a hostd.json that differs from the live one unless ACCEPT_CONFIG=1 (the diff is shown;
# hostd.json holds no secrets). A failed health check rolls back automatically.
# Env (defaults are Gold's paths): HOSTD_RELEASES, HOSTD_BIN, HOSTD_CONFIG, HOSTD_UNIT, SYSTEMCTL.
set -euo pipefail
R=${1:?release dir}
D=${HOSTD_RELEASES:-/usr/local/lib/shippin-hostd/releases}
BIN=${HOSTD_BIN:-/usr/local/bin/shippin-hostd}; CFG=${HOSTD_CONFIG:-/etc/shippin/hostd.json}
UNIT=${HOSTD_UNIT:-/etc/systemd/system/shippin-hostd.service}; SYSTEMCTL=${SYSTEMCTL:-systemctl}
KEEP=${KEEP:-5}; HERE=$(dirname "$0")
ID=$(sed -n 's/^id=//p' "$R/RELEASE" 2>/dev/null); ID=${ID:-$(basename "$R")}
mkdir -p "$D"
if [ ! -e "$D/current" ] && [ -f "$BIN" ] && [ ! -L "$BIN" ]; then
  L=00000000000000-pre-versioned; mkdir -p "$D/$L"
  cp -a "$BIN" "$D/$L/shippin-hostd"; [ -f "$CFG" ] && cp -a "$CFG" "$D/$L/hostd.json"
  [ -f "$UNIT" ] && cp -a "$UNIT" "$D/$L/shippin-hostd.service"
  echo "id=$L" > "$D/$L/RELEASE"; ln -sfn "$L" "$D/current"; echo "adopted the installed hostd as $L"
fi
if [ -f "$CFG" ] && ! diff -q "$CFG" "$R/deploy/hostd.example.json" >/dev/null; then
  diff -u "$CFG" "$R/deploy/hostd.example.json" || true
  [ "${ACCEPT_CONFIG:-}" = 1 ] || { echo "hostd.json differs from the live one (above); rerun with ACCEPT_CONFIG=1" >&2; exit 1; }
fi
rm -rf "$D/$ID.tmp"; mkdir -p "$D/$ID.tmp"
install -m755 "$R/shippin-hostd" "$D/$ID.tmp/shippin-hostd"
[ -f "$R/rrflow-state" ] && install -m755 "$R/rrflow-state" "$D/$ID.tmp/rrflow-state"
[ -f "$R/shippin-credwriter" ] && install -m755 "$R/shippin-credwriter" "$D/$ID.tmp/shippin-credwriter"
install -m600 "$R/deploy/hostd.example.json" "$D/$ID.tmp/hostd.json"
install -m644 "$R/deploy/shippin-hostd.service" "$D/$ID.tmp/shippin-hostd.service"
cp "$R/RELEASE" "$D/$ID.tmp/RELEASE"
rm -rf "${D:?}/$ID"; mv "$D/$ID.tmp" "$D/$ID"
"$HERE/release/switch.sh" "$D" "$ID"
"$HERE/hostd-activate.sh"
if "$HERE/hostd-health.sh"; then
  "$HERE/release/prune.sh" "$D" "$KEEP"; echo "hostd release $ID is live"
else
  echo "hostd release $ID failed its health check; rolling back" >&2
  "$HERE/release/switch.sh" "$D" --rollback && "$HERE/hostd-activate.sh"; exit 1
fi
