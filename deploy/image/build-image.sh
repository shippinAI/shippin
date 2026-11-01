#!/bin/bash
# Build a Shippin workspace image (Debian 13 + SSH + openvscode-server + a toolchain) and publish it
# under $ALIAS. TOOLCHAIN: dev (legacy: Go+Node), blank, node, python, go. The builder is removed after.
# AI=1 (default) preinstalls Claude Code, Codex and the Grok CLI (ai/install.sh); users sign in to
# their own subscriptions in their own container. WEZTERM=1 (default) adds the WezTerm mux; Zellij always. Pin CLIs via AI_PKGS / AI_OPTIONAL_PKGS (npm specs).
set -euo pipefail
P="--project platform"; TOOLCHAIN=${TOOLCHAIN:-dev}; ALIAS=${ALIAS:-shippin/dev-debian13}; B=img-builder-$$; HERE=$(dirname "$0")
OVS=$(curl -s https://api.github.com/repos/gitpod-io/openvscode-server/releases/latest | python3 -c "import sys,json;print(json.load(sys.stdin)['tag_name'])")
GOV=$(curl -s 'https://go.dev/dl/?mode=json' | python3 -c "import sys,json;print(json.load(sys.stdin)[0]['version'])")
case $TOOLCHAIN in
  dev) PKGS="nodejs npm"; GO=1; DESC="git, $GOV, Node" ;;
  blank) PKGS=""; GO=0; DESC="git" ;;
  node) PKGS="nodejs npm"; GO=0; DESC="git, Node, npm, pnpm" ;;
  python) PKGS="python3 python3-venv python3-pip python3-dev pipx"; GO=0; DESC="git, Python 3, venv, pip, pipx" ;;
  go) PKGS=""; GO=1; DESC="git, $GOV" ;;
  *) echo "unknown TOOLCHAIN $TOOLCHAIN" >&2; exit 1 ;;
esac
AI=${AI:-1}; AI_PKGS=${AI_PKGS:-"@anthropic-ai/claude-code @openai/codex"}; AI_OPTIONAL_PKGS=${AI_OPTIONAL_PKGS:-"@xai-official/grok"}
PKGS="$PKGS gh" # GitHub CLI for imported projects (git/install.sh wires it to the workspace token)
# AI=1: Claude Code needs Node >= 22 (Debian 13 ships 20), so Node LTS comes from nodejs.org (SHASUMS256-checked)
# and replaces the distro nodejs/npm. Pin with NODE_VERSION=v22.x.y.
NODEV=""; if [ "$AI" = 1 ]; then PKGS=$(echo " $PKGS " | sed -e "s/ nodejs / /" -e "s/ npm / /")
  NODEV=${NODE_VERSION:-$(curl -s https://nodejs.org/dist/index.json | python3 -c "import sys,json;print(next(r['version'] for r in json.load(sys.stdin) if r['lts']))")}; DESC="$DESC, Claude Code, Codex, Grok CLI"; fi
# ttyd (sign-in terminal) is not packaged in Debian 13: pinned upstream static binary, checked against its SHA256SUMS.
TTYD=${TTYD_VERSION:-$(curl -s https://api.github.com/repos/tsl0922/ttyd/releases/latest | python3 -c "import sys,json;print(json.load(sys.stdin)['tag_name'])")}
trap 'incus delete -f $B $P </dev/null >/dev/null 2>&1 || true' EXIT  # never leave a builder behind
incus launch -q images:debian/13 $B $P </dev/null; sleep 10
incus exec $B $P --env GOV=$GOV --env OVS=$OVS --env PKGS="$PKGS" --env GO=$GO --env TC=$TOOLCHAIN --env AI=$AI --env TTYD=$TTYD --env NODEV=$NODEV -- bash -euxc '
export DEBIAN_FRONTEND=noninteractive
apt-get -qq update && apt-get -qq install -y --no-install-recommends openssh-server git curl ca-certificates sudo build-essential vim less htop tmux unzip jq locales iproute2 $PKGS >/dev/null
if [ "$GO" = 1 ]; then curl -sL https://go.dev/dl/$GOV.linux-amd64.tar.gz | tar -C /usr/local -xz; echo "export PATH=\$PATH:/usr/local/go/bin:\$HOME/go/bin" > /etc/profile.d/go.sh; fi
if [ "$AI" = 1 ]; then u=https://github.com/tsl0922/ttyd/releases/download/$TTYD; cd /tmp && curl -fsSLO $u/ttyd.x86_64 && curl -fsSL $u/SHA256SUMS | grep " ttyd.x86_64$" | sha256sum -c - && install -m 0755 ttyd.x86_64 /usr/bin/ttyd && rm ttyd.x86_64 && cd /; fi
if [ -n "$NODEV" ]; then u=https://nodejs.org/dist/$NODEV; f=node-$NODEV-linux-x64.tar.xz; cd /tmp && curl -fsSLO $u/$f && curl -fsSL $u/SHASUMS256.txt | grep " $f$" | sha256sum -c - && tar -C /usr/local --strip-components=1 -xJf $f --exclude=CHANGELOG.md --exclude=README.md --exclude=LICENSE && rm $f && cd / && node --version; fi
if [ "$TC" = node ]; then npm install -g --no-audit --no-fund pnpm >/dev/null; fi
mkdir -p /opt/openvscode-server && curl -sL https://github.com/gitpod-io/openvscode-server/releases/download/$OVS/$OVS-linux-x64.tar.gz | tar -C /opt/openvscode-server --strip-components=1 -xz
useradd -m -s /bin/bash -u 1000 dev && echo "dev ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/dev && chmod 440 /etc/sudoers.d/dev
printf "PasswordAuthentication no\nKbdInteractiveAuthentication no\nPermitRootLogin no\n" > /etc/ssh/sshd_config.d/shippin.conf
systemctl enable ssh; rm -f /etc/ssh/ssh_host_*; printf "[Unit]\nDescription=Regenerate SSH host keys\nBefore=ssh.service\nConditionPathExists=!/etc/ssh/ssh_host_ed25519_key\n[Service]\nType=oneshot\nExecStart=/usr/bin/ssh-keygen -A\n[Install]\nWantedBy=multi-user.target\n" > /etc/systemd/system/ssh-hostkeys.service; systemctl enable ssh-hostkeys
rm -rf /home/dev; apt-get clean; rm -rf /var/lib/apt/lists/*' </dev/null
for f in shippin-ws-init shippin-ide shippin-ws-bootstrap shippin-startup; do incus file push $HERE/$f $B/usr/local/sbin/$f $P --mode 0755; done
for f in shippin-ide.service shippin-startup.service; do incus file push $HERE/$f $B/etc/systemd/system/$f $P; done
incus exec $B $P -- systemctl enable shippin-ide </dev/null
incus file push -r $HERE/git $B/tmp/ $P
incus exec $B $P -- bash -euc '/tmp/git/install.sh; rm -rf /tmp/git' </dev/null
incus file push -r $HERE/term $B/tmp/ $P  # Zellij + WezTerm mux + shippin-term (Terminal view default)
incus exec $B $P --env WEZTERM=${WEZTERM:-1} --env ZELLIJ_VERSION=${ZELLIJ_VERSION:-} -- bash -euc 'apt-get -qq update && apt-get -qq install -y --no-install-recommends gnupg python3 >/dev/null; /tmp/term/install.sh; rm -rf /tmp/term' </dev/null
if [ "$AI" = 1 ]; then
  incus file push -r $HERE/ai $B/tmp/ $P
  incus exec $B $P --env AI_PKGS="$AI_PKGS" --env AI_OPTIONAL_PKGS="$AI_OPTIONAL_PKGS" -- bash -euc '/tmp/ai/install.sh; rm -rf /tmp/ai' </dev/null
fi
incus stop $B $P </dev/null
incus image alias delete $ALIAS </dev/null 2>/dev/null || true  # keep the old image, move the alias
incus publish $B $P --alias $ALIAS description="Shippin workspace $TOOLCHAIN (Debian 13, $DESC, SSH, $OVS)" </dev/null
incus image list $ALIAS </dev/null
