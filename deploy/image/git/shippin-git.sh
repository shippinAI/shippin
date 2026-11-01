# Shippin: `gh` uses the workspace's current GitHub App installation token (refreshed by the
# platform every 45 min), read at call time so long-lived shells never hold a stale one.
gh() { GH_TOKEN="$(head -n 1 "${SHIPPIN_GIT_TOKEN:-/run/shippin/git/token}" 2>/dev/null)" command gh "$@"; }
