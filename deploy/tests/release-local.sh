#!/bin/bash
# release-local.sh - exercises the versioned hostd install/rollback and the release scripts in a
# temp dir (no Incus, no systemd, no root). Run from anywhere; exits non-zero on the first failure.
set -euo pipefail
HERE=$(cd "$(dirname "$0")/.." && pwd); T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
export HOSTD_RELEASES=$T/lib/releases HOSTD_BIN=$T/bin/shippin-hostd HOSTD_CONFIG=$T/etc/hostd.json
export HOSTD_UNIT=$T/unit/shippin-hostd.service SYSTEMCTL=true KEEP=2
mkdir -p "$T/bin" "$T/etc" "$T/unit"
echo old-binary > "$HOSTD_BIN"; echo '{"socket":"/x"}' > "$HOSTD_CONFIG"; echo old-unit > "$HOSTD_UNIT"
mkrel() { # mkrel ID CONFIG
  local r=$T/build/$1; mkdir -p "$r/deploy/release"
  echo "bin-$1" > "$r/shippin-hostd"; echo "$2" > "$r/deploy/hostd.example.json"; echo unit > "$r/deploy/shippin-hostd.service"
  echo "id=$1" > "$r/RELEASE"; echo "$r"
}
cur() { basename "$(readlink "$HOSTD_RELEASES/current")"; }
prev() { basename "$(readlink "$HOSTD_RELEASES/previous")"; }
fail() { echo "FAIL: $*"; exit 1; }
A=$(mkrel 20261018000001-aaa '{"socket":"/x"}')
HOSTD_HEALTH_CMD=true "$HERE/hostd-install.sh" "$A" >/dev/null
[ "$(cur)" = 20261018000001-aaa ] && [ "$(prev)" = 00000000000000-pre-versioned ] || fail "first install $(cur)/$(prev)"
[ "$(cat "$HOSTD_BIN")" = bin-20261018000001-aaa ] || fail "binary not switched"
[ "$(cat "$HOSTD_RELEASES/previous/shippin-hostd")" = old-binary ] || fail "old binary not adopted"
B=$(mkrel 20261018000002-bbb '{"socket":"/y"}')
if HOSTD_HEALTH_CMD=true "$HERE/hostd-install.sh" "$B" >/dev/null 2>&1; then fail "config change accepted silently"; fi
[ "$(cur)" = 20261018000001-aaa ] || fail "refused install changed current"
if ACCEPT_CONFIG=1 HOSTD_HEALTH_CMD=false "$HERE/hostd-install.sh" "$B" >/dev/null 2>&1; then fail "unhealthy install succeeded"; fi
[ "$(cur)" = 20261018000001-aaa ] && [ "$(cat "$HOSTD_BIN")" = bin-20261018000001-aaa ] || fail "no auto rollback"
[ "$(jq -r .socket "$HOSTD_CONFIG")" = /x ] || fail "config not rolled back"
ACCEPT_CONFIG=1 HOSTD_HEALTH_CMD=true "$HERE/hostd-install.sh" "$B" >/dev/null
[ "$(cur)" = 20261018000002-bbb ] && [ "$(prev)" = 20261018000001-aaa ] || fail "second install"
HOSTD_HEALTH_CMD=true "$HERE/hostd-rollback.sh" >/dev/null
[ "$(cur)" = 20261018000001-aaa ] && [ "$(prev)" = 20261018000002-bbb ] && [ "$(jq -r .socket "$HOSTD_CONFIG")" = /x ] || fail "rollback"
C=$(mkrel 20261018000003-ccc '{"socket":"/x"}')
HOSTD_HEALTH_CMD=true "$HERE/hostd-install.sh" "$C" >/dev/null
n=$(find "$HOSTD_RELEASES" -mindepth 1 -maxdepth 1 -type d | wc -l)
[ -d "$HOSTD_RELEASES/20261018000001-aaa" ] && [ -d "$HOSTD_RELEASES/20261018000003-ccc" ] || fail "prune removed current/previous"
[ "$n" -le 3 ] || fail "prune kept $n releases"
"$HERE/release/switch.sh" "$HOSTD_RELEASES" nope >/dev/null 2>&1 && fail "switch to a missing release"
echo "release-local: all checks passed"
