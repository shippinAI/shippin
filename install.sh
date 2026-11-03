#!/bin/bash
# Installs the panel on a Debian or Ubuntu machine, or updates one this script installed.
#
#   curl -fsSL https://raw.githubusercontent.com/shippinAI/shippin/main/install.sh | sudo bash
#
# It keeps its settings in /etc/shippin/site.env, which it writes only the first time. Edit that file to
# change them and run this again. Options: --host <name or address> --port <port> --dir <path>
set -euo pipefail

repo=${SHIPPIN_SOURCE:-https://github.com/shippinAI/shippin.git}
branch=${SHIPPIN_BRANCH:-main}
dir=/opt/shippin
host=""
port=8443
while [ $# -gt 0 ]; do
  case "$1" in
    --host) host=${2:?--host needs a name or address}; shift 2 ;;
    --port) port=${2:?--port needs a number}; shift 2 ;;
    --dir) dir=${2:?--dir needs a path}; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[ "$(id -u)" = 0 ] || { echo "Run this as root, for example with sudo." >&2; exit 1; }
command -v apt-get >/dev/null || { echo "This needs Debian or Ubuntu. For anything else, use Docker: see docs/deploy.md." >&2; exit 1; }
if ! command -v git >/dev/null; then
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git ca-certificates >/dev/null
fi

if [ -d "$dir/.git" ]; then
  git -C "$dir" pull --ff-only -q
else
  git clone -q --branch "$branch" "$repo" "$dir"
fi

site=/etc/shippin/site.env
if [ ! -f "$site" ]; then
  [ -n "$host" ] || host=$(hostname -I 2>/dev/null | awk '{print $1}')
  [ -n "$host" ] || host=$(hostname)
  install -d -m 0700 /etc/shippin
  (umask 077; cat > "$site" <<SITE
# This installation's settings. config/site.env.example in $dir lists the rest.
SHIPPIN_DOMAIN=$host
SHIPPIN_CONSOLE_HOST=$host
SHIPPIN_ORIGIN=https://$host:$port
SHIPPIN_LISTEN=0.0.0.0:$port
SHIPPIN_TLS=self-signed
SHIPPIN_OPERATOR_HOST=none
SITE
  )
fi

log=$(mktemp)
trap 'rm -f "$log"' EXIT
"$dir/install/panel/install.sh" --site "$site" | tee "$log"
origin=$(sed -n 's/^SHIPPIN_ORIGIN=//p' "$site" | tail -n1)
code=$(sed -n 's/^setup_code=//p' "$log" | tail -n1)
echo
if [ -n "$code" ]; then
  echo "Shippin is running. Open $origin/setup and enter this code to create the owner:"
  echo
  echo "    $code"
  echo
  echo "Your browser will warn about the certificate, which is self-signed for this machine."
else
  echo "Shippin is up to date at $origin"
fi
