#!/bin/bash
# openapi-routes.sh OPENAPI_YAML - fail when the console's /v1 routes and another copy of the
# hand-written spec drift beyond internal/api/testdata/openapi-drift.txt (which may only shrink).
# `go test ./internal/api` already checks the repository's own api/openapi/openapi.yaml.
# Example: scripts/openapi-routes.sh path/to/openapi.yaml
set -euo pipefail
SPEC=$(realpath "${1:?path to openapi.yaml}")
cd "$(dirname "$0")/.."
OPENAPI_SPEC=$SPEC go test -count=1 -run '^TestOpenAPIContract$' ./internal/api
