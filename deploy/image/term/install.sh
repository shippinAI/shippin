#!/bin/bash
# Runs as root inside the image builder (build-image.sh). Terminal tooling for the console's
# Terminal view and for WezTerm on the user's machine:
#  - Zellij (pinned upstream musl binary, checked against its .sha256sum) + system config/layout;
#  - wezterm (APT repo apt.fury.io/wez) so `wezterm connect` ssh domains can run the remote mux
#    (`wezterm cli proxy`); the console's downloadable wezterm.lua sets default_prog=shippin-term;
#  - shippin-term: the login wrapper hostd runs (attach-or-create Zellij session "shippin").
set -euo pipefail
HERE=$(dirname "$0")
ZJ=${ZELLIJ_VERSION:-$(curl -s https://api.github.com/repos/zellij-org/zellij/releases/latest | python3 -c "import sys,json;print(json.load(sys.stdin)['tag_name'])")}
f=zellij-x86_64-unknown-linux-musl; u=https://github.com/zellij-org/zellij/releases/download/$ZJ
# The .sha256sum covers the extracted binary (as upstream publishes it), so check after extraction.
cd /tmp && curl -fsSLO $u/$f.tar.gz && tar -xzf $f.tar.gz zellij \
  && echo "$(curl -fsSL $u/$f.sha256sum | awk '{print $1}')  zellij" | sha256sum -c - \
  && install -m 0755 zellij /usr/local/bin/zellij && rm -f zellij $f.tar.gz && cd /
install -d /etc/zellij/layouts
install -m 0644 "$HERE/zellij-config.kdl" /etc/zellij/config.kdl
install -m 0644 "$HERE/shippin.kdl" /etc/zellij/layouts/shippin.kdl
echo 'export ZELLIJ_CONFIG_DIR=/etc/zellij' > /etc/profile.d/shippin-zellij.sh
install -m 0755 "$HERE/shippin-term" /usr/local/bin/shippin-term
if [ "${WEZTERM:-1}" = 1 ]; then
  curl -fsSL https://apt.fury.io/wez/gpg.key | gpg --yes --dearmor -o /usr/share/keyrings/wezterm-fury.gpg
  chmod 644 /usr/share/keyrings/wezterm-fury.gpg
  echo 'deb [signed-by=/usr/share/keyrings/wezterm-fury.gpg] https://apt.fury.io/wez/ * *' > /etc/apt/sources.list.d/wezterm.list
  DEBIAN_FRONTEND=noninteractive apt-get -qq update && apt-get -qq install -y --no-install-recommends wezterm >/dev/null
  apt-get clean; rm -rf /var/lib/apt/lists/*
fi
zellij --version
