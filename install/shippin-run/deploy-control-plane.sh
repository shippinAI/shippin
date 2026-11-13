#!/bin/bash
# Install the Shippin Run control plane on a Linux machine.
# Run from a checkout. Fill /etc/shippin-run/env before starting the service.
set -euo pipefail
if [ "$(uname -s)" != Linux ]; then
  echo "shippin-run control plane installs on Linux. On Windows, create the Hyper-V VM first." >&2
  exit 1
fi
if [ "$(id -u)" -ne 0 ]; then
  echo "re-run with sudo" >&2
  exit 1
fi
root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
install -d -m 0750 /etc/shippin-run /var/lib/shippin-run
if ! id shippin-run >/dev/null 2>&1; then
  useradd --system --home-dir /var/lib/shippin-run --shell /usr/sbin/nologin shippin-run
fi
chown shippin-run:shippin-run /var/lib/shippin-run
chmod 0700 /var/lib/shippin-run
if [ ! -x /usr/local/bin/shippin-run ]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "go is not installed and /usr/local/bin/shippin-run is absent" >&2
    exit 1
  fi
  (cd "$root" && go build -o /tmp/shippin-run ./cmd/shippin-run)
  install -m 0755 /tmp/shippin-run /usr/local/bin/shippin-run
  rm -f /tmp/shippin-run
fi
if [ ! -f /etc/shippin-run/shippin-run.json ]; then
  install -m 0640 -o root -g shippin-run "$root/config/shippin-run.example.json" /etc/shippin-run/shippin-run.json
fi
if [ ! -f /etc/shippin-run/env ]; then
  install -m 0640 -o root -g shippin-run "$root/config/shippin-run.env.example" /etc/shippin-run/env
  echo "Wrote /etc/shippin-run/env. Set SHIPPIN_RUN_API_TOKEN before systemctl start." >&2
fi
install -m 0644 "$root/install/shippin-run/shippin-run.service" /etc/systemd/system/shippin-run.service
systemctl daemon-reload
echo "Unit installed. Edit /etc/shippin-run/env, then: systemctl enable --now shippin-run"
