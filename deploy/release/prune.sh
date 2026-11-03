#!/bin/bash
# release/prune.sh DIR KEEP - delete all but the newest KEEP release dirs in DIR (names sort by
# time), never the ones the `current` or `previous` symlinks point at. Used on the build host and
# inside the console container (/opt/console/releases).
set -euo pipefail
D=${1:?releases dir}; KEEP=${2:?how many to keep}
[ "$KEEP" -ge 2 ] || { echo "keep at least 2 (current + one rollback)" >&2; exit 2; }
keep_link() { [ -L "$1" ] && basename "$(readlink "$1")" || true; }
CUR=$(keep_link "$D/current"); PREV=$(keep_link "$D/previous")
mapfile -t ALL < <(find "$D" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort -r)
for i in "${!ALL[@]}"; do
  n=${ALL[$i]}
  if [ "$i" -lt "$KEEP" ] || [ "$n" = "$CUR" ] || [ "$n" = "$PREV" ]; then continue; fi
  rm -rf -- "${D:?}/$n"; echo "pruned release $n"
done
