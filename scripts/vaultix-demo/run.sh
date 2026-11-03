#!/bin/bash
# Start vaultix-demo (loopback). Every path/URL comes from the environment; see the plan for values.
# Needs: deploy/vaultix stack up, a Nomen (Zitadel) instance set up by nomen-setup.py.
set -euo pipefail
: "${VX_SBX:?dir with ids.json, operator.json, seal.key}" "${PG_URL:?console database URL}" "${NOMEN_ISSUER:?}" "${VAULTIX_URL:?}"
cd "$(dirname "$0")/../.."
go build -o bin/vaultix-demo ./cmd/vaultix-demo
export VX_DEMO_ADDR=${VX_DEMO_ADDR:-localhost:8497} DATABASE_URL=$PG_URL SHIPPIN_CONFIG=config/shippin.example.json \
  NOMEN_CLIENT_ID=$(jq -r .client_id "$VX_SBX/ids.json") VAULTIX_OPERATOR_FILE=$VX_SBX/operator.json \
  VAULTIX_SEAL_KEY_FILE=$VX_SBX/seal.key VAULTIX_PUBLIC_URL=${VAULTIX_PUBLIC_URL:-$VAULTIX_URL} \
  VAULTIX_ENVS=${VAULTIX_ENVS:-dev,staging,prod} VX_DEMO_STATIC=web/dist
exec ./bin/vaultix-demo
