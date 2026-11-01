#!/bin/bash
# release/switch.sh DIR ID - point DIR/current at release ID and DIR/previous at the old current
# (atomic rename of a temp symlink). `switch.sh DIR --rollback` swaps current and previous.
# Prints "current=<id> previous=<id>". Runs on Gold's host and inside the console container.
set -euo pipefail
D=${1:?releases dir}; ID=${2:?release id or --rollback}
link() { ln -sfn "$1" "$D/.$2.tmp" && mv -T "$D/.$2.tmp" "$D/$2"; }
OLD=$( [ -L "$D/current" ] && basename "$(readlink "$D/current")" || true )
if [ "$ID" = "--rollback" ]; then
  [ -L "$D/previous" ] || { echo "no previous release to roll back to" >&2; exit 1; }
  ID=$(basename "$(readlink "$D/previous")")
fi
[ -d "$D/$ID" ] || { echo "release $ID not found in $D" >&2; exit 1; }
[ "$ID" = "$OLD" ] && { echo "current=$ID previous=$( [ -L "$D/previous" ] && basename "$(readlink "$D/previous")")"; exit 0; }
link "$ID" current
[ -n "$OLD" ] && link "$OLD" previous
echo "current=$ID previous=${OLD:-none}"
