#!/bin/bash
# Runs what CI runs that needs no dedicated machine. See docs/development.md.
set -uo pipefail
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
failed=()
step() {
  local name=$1 dir=$2
  shift 2
  echo "=== $name"
  if ! (cd "$root/$dir" && "$@"); then failed+=("$name"); fi
}

if [ "$(go env GOOS)" = windows ]; then
  # hostd and the launcher are Unix-only; CI tests them on Linux.
  step "console (build)" . go build ./cmd/console ./cmd/shippin-hostd
else
  step "console (test)" . go test ./...
fi
step "console (vet)" . go vet ./cmd/console ./internal/api ./internal/config ./internal/hv/...
step "agent" agent go test ./...
step "panel (install)" web npm ci --no-audit --no-fund
step "panel (test)" web npm test
step "panel (build)" web npm run build
step "panel (budget)" web npm run budget
step "governance (install)" governance npm ci --no-audit --no-fund
step "governance (test)" governance npm test

if [ "${#failed[@]}" -gt 0 ]; then
  echo "FAILED: ${failed[*]}"
  exit 1
fi
echo "All checks passed."
