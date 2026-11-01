#!/bin/bash
# release/build.sh - build one versioned release dir: $RELEASES_DIR/<id>/ with console,
# shippin-hostd, rrflow-state, web/dist, config/, deploy/ and RELEASE (id, commit, built_at, dirty).
# <id> = <UTC yyyymmddHHMMSS>-<short sha>[-dirty]. Keeps the newest $KEEP (default 5) releases,
# never deleting the ones named in $RELEASES_DIR/{current,previous}. Prints the release dir.
# Env: RELEASES_DIR (required), KEEP (default 5), GO (default go), NPM (default npm).
set -euo pipefail
OUT=${RELEASES_DIR:?set RELEASES_DIR, e.g. /var/lib/shippin/site/releases/console}
KEEP=${KEEP:-5}; GO=${GO:-go}; NPM=${NPM:-npm}
cd "$(dirname "$0")/../.."
SHA=$(git rev-parse --short=12 HEAD)
DIRTY=""; git diff --quiet HEAD -- . ':!web/dist' || DIRTY="-dirty"
ID="$(date -u +%Y%m%d%H%M%S)-$SHA$DIRTY"
R="$OUT/$ID"; mkdir -p "$R/web"
( cd web && "$NPM" ci --silent && "$NPM" test --silent >/dev/null && "$NPM" run build --silent && "$NPM" run budget --silent ) >&2
CGO_ENABLED=0 "$GO" build -trimpath -o "$R/console" ./cmd/console
CGO_ENABLED=0 "$GO" build -trimpath -o "$R/shippin-hostd" ./cmd/shippin-hostd
# rrflow-state (02c-rrflow): installed beside shippin-hostd, which pushes it into service instances.
CGO_ENABLED=0 "$GO" build -trimpath -o "$R/rrflow-state" ./cmd/rrflow-state
# shippin-credwriter: hostd pushes it into workspaces as the per-workspace credential writer.
CGO_ENABLED=0 "$GO" build -trimpath -o "$R/shippin-credwriter" ./cmd/shippin-credwriter
# chat-probe: server-side chat round trip (same code path as the console), run in the console container.
CGO_ENABLED=0 "$GO" build -trimpath -o "$R/chat-probe" ./cmd/chat-probe
cp -r web/dist "$R/web/dist"
cp -r config "$R/config"
cp -r deploy "$R/deploy"
printf 'id=%s\ncommit=%s\nbuilt_at=%s\ndirty=%s\n' "$ID" "$(git rev-parse HEAD)" "$(date -u +%FT%TZ)" "${DIRTY:+yes}" > "$R/RELEASE"
"$(dirname "$0")/prune.sh" "$OUT" "$KEEP" >&2
echo "$R"
