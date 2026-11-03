#!/usr/bin/env bash
# Generate a runtime env overlay from Infisical export.
# Safe to run repeatedly; never prints secret values.

set -euo pipefail

TARGET_FILE="${1:-/run/warden/runtime.env}"
TMP_FILE="$(mktemp)"
TARGET_DIR="$(dirname "$TARGET_FILE")"
CANONICAL_ENV="${WARDEN_CANONICAL_ENV_FILE:-/etc/warden/warden.env}"
SECRETS_ROOT="${WARDENCLYFFE_SECRETS_DIR:-/etc/warden}"
LOG_PREFIX="[warden-infisical-refresh]"

cleanup() {
  rm -f "$TMP_FILE"
}
trap cleanup EXIT

log() {
  printf '%s %s\n' "$LOG_PREFIX" "$*"
}

load_dotenv_file() {
  local f="$1"
  [ -f "$f" ] || return 0
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ""|\#*) continue ;;
      *=*)
        export "${line%%=*}=${line#*=}"
        ;;
    esac
  done < "$f"
}

load_dotenv_file "$SECRETS_ROOT/.env"
load_dotenv_file "$SECRETS_ROOT/infisical.env"
load_dotenv_file "$CANONICAL_ENV"
log "loaded bootstrap env layers"

if ! command -v infisical >/dev/null 2>&1; then
  log "infisical CLI not found; skipping runtime export"
  exit 0
fi

if [ -z "${INFISICAL_TOKEN:-}" ] && [ -n "${INFISICAL_PAT:-}" ]; then
  INFISICAL_TOKEN="$INFISICAL_PAT"
  export INFISICAL_TOKEN
  log "using INFISICAL_PAT as break-glass token"
fi

if [ -z "${INFISICAL_TOKEN:-}" ] && [ -n "${INFISICAL_UNIVERSAL_AUTH_CLIENT_ID:-}" ] && [ -n "${INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET:-}" ]; then
  log "minting short-lived token via Universal Auth"
  api="${INFISICAL_API_URL:-${INFISICAL_URL:-https://app.infisical.com}}"
  token="$(
    curl -fsS \
      -H "Content-Type: application/x-www-form-urlencoded" \
      --data-urlencode "clientId=$INFISICAL_UNIVERSAL_AUTH_CLIENT_ID" \
      --data-urlencode "clientSecret=$INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET" \
      "${api%/}/api/v1/auth/universal-auth/login" \
      | python3 -c 'import json,sys; p=json.load(sys.stdin); print((p.get("accessToken") or "").strip())'
  )"
  if [ -n "$token" ]; then
    INFISICAL_TOKEN="$token"
    export INFISICAL_TOKEN
    log "universal auth token mint succeeded"
  else
    log "universal auth token mint returned empty token"
  fi
fi

if [ -z "${INFISICAL_TOKEN:-}" ]; then
  log "no auth token available; skipping runtime export"
  exit 0
fi

PROJECT_ID="${INFISICAL_PROJECT_ID:-${INFISICAL_PROJECTID:-}}"
if [ -z "$PROJECT_ID" ]; then
  log "no INFISICAL_PROJECT_ID selector; skipping runtime export"
  exit 0
fi

cmd=(infisical export --format=dotenv "--projectId=$PROJECT_ID")
if [ -n "${INFISICAL_ENV:-${INFISICAL_ENVIRONMENT:-}}" ]; then
  cmd+=("--env=${INFISICAL_ENV:-$INFISICAL_ENVIRONMENT}")
fi
if [ -n "${INFISICAL_PATH:-}" ]; then
  cmd+=("--path=$INFISICAL_PATH")
fi

if ! "${cmd[@]}" > "$TMP_FILE"; then
  log "infisical export failed; leaving existing runtime env untouched"
  exit 0
fi

mkdir -p "$TARGET_DIR"
install -m 600 -o root -g root "$TMP_FILE" "$TARGET_FILE"
log "runtime env refreshed at $TARGET_FILE"
