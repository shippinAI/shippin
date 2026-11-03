#!/bin/bash
# budget-check.sh - proves web/scripts/check-budget.mjs passes the real build and fails an
# over-budget one (initial entry and route chunk). Needs web/dist from `npm run build`.
set -euo pipefail
WEB=$(cd "$(dirname "$0")/../../web" && pwd); T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
fail() { echo "FAIL: $*"; exit 1; }
node "$WEB/scripts/check-budget.mjs" "$WEB/dist" >/dev/null || fail "real build over budget"
mkdir -p "$T/assets"
head -c 120000 /dev/urandom | base64 -w0 > "$T/assets/big.js"   # ~90+ KB gz, incompressible
echo 'x' > "$T/assets/small.js"
echo '<script type="module" src="/assets/big.js"></script>' > "$T/index.html"
echo '<script type="module" src="/assets/small.js"></script>' > "$T/ops.html"
if node "$WEB/scripts/check-budget.mjs" "$T" >/dev/null; then fail "over-budget entry passed"; fi
echo '<script type="module" src="/assets/small.js"></script>' > "$T/index.html"
if node "$WEB/scripts/check-budget.mjs" "$T" >/dev/null; then fail "over-budget route chunk passed"; fi
echo "budget-check: ok"
