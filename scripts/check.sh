#!/bin/bash
# Runs what CI runs that needs no dedicated machine. See docs/development.md.
set -uo pipefail
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
failed=()
go_bin=$(command -v go || command -v go.exe || printf go)
if ! command -v "$go_bin" >/dev/null 2>&1; then
  echo "Go is unavailable (looked for go and go.exe); Go checks will fail and the remaining checks will continue." >&2
fi
step() {
  local name=$1 dir=$2
  shift 2
  echo "=== $name"
  if ! (cd "$root/$dir" && "$@"); then failed+=("$name"); fi
}

if [ "$("$go_bin" env GOOS)" = windows ]; then
  # hostd and the launcher are Unix-only; CI tests them on Linux.
  step "console (build)" . "$go_bin" build ./cmd/console ./cmd/shippin-hostd
else
  step "console (test)" . "$go_bin" test ./...
fi
step "console (vet)" . "$go_bin" vet ./cmd/console ./internal/api ./internal/config ./internal/hv/...
step "agent" agent "$go_bin" test ./...
step "panel (install)" web npm ci --no-audit --no-fund
step "panel (test)" web npm test
step "panel (build)" web npm run build
step "panel (budget)" web npm run budget
step "panel (tokens)" web npm run tokens
step "governance (install)" governance npm ci --no-audit --no-fund
step "governance (test)" governance npm test
step "docs (regression)" . node --test scripts/check-docs.test.mjs
step "docs (links)" . node scripts/check-docs.mjs

if [ "${#failed[@]}" -gt 0 ]; then
  echo "FAILED: ${failed[*]}"
  exit 1
fi
echo "All checks passed."
