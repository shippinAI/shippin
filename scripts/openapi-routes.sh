#!/bin/bash
# openapi-routes.sh OPENAPI_YAML - fail when the console's /v1 routes and the OpenAPI spec drift
# beyond internal/api/testdata/openapi-drift.txt (which may only shrink).
# Example: scripts/openapi-routes.sh ../../gold-homebase/SHIPPIN-CLOUD/console/openapi/openapi.yaml
set -euo pipefail
SPEC=$(realpath "${1:?path to openapi.yaml}")
cd "$(dirname "$0")/.."
OPENAPI_SPEC=$SPEC go test -count=1 -run '^TestOpenAPIContract$' ./internal/api
