#!/bin/bash
# Build the local proof image. Pins come from deploy/ai-cli-install.sh (single source of truth).
set -euo pipefail
cd "$(dirname "$0")/../.."
PKGS=$(sed -n 's/^PKGS="\([^"]*\)".*/\1/p' deploy/ai-cli-install.sh)
CGO_ENABLED=0 go build -o bin/ai-launcher ./cmd/ai-launcher
(cd web && npm run build >/dev/null)
podman build -q --network "${BUILD_NETWORK:-host}" -f deploy/ai-launcher/Containerfile --build-arg BASE_IMAGE="${BASE_IMAGE:?e.g. docker.io/library/node:22-bookworm-slim}" \
  --build-arg AI_PKGS="$PKGS" -t "${IMAGE:-localhost/shippin-ai-launcher:dev}" .
