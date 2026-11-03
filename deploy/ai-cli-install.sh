#!/bin/bash
# Install the ai1-image AI CLIs (same pins/method) into an existing workspace. Snapshot first.
# usage: ai-cli-install.sh <project> <instance> <release-dir>
set -euo pipefail
P=$1; I=$2; R=$3; NODEV=v24.22.0; F=node-$NODEV-linux-x64.tar.xz
PKGS="@anthropic-ai/claude-code@2.1.303 @openai/codex@0.163.0"; OPT="@xai-official/grok@1.0.62"
X() { incus exec -T "$I" --project "$P" -- "$@" </dev/null; }
S=pre-ai-cli-$(date +%Y%m%d%H%M%S); incus snapshot create "$I" "$S" --project "$P"; echo "snapshot $P/$I/$S"
cd /tmp; [ -f $F ] || { curl -fsSLO https://nodejs.org/dist/$NODEV/$F && curl -fsSL https://nodejs.org/dist/$NODEV/SHASUMS256.txt | grep " $F$" | sha256sum -c -; }
incus file push /tmp/$F "$I/tmp/$F" --project "$P" >/dev/null
X sh -c "tar -C /usr/local --strip-components=1 -xJf /tmp/$F --exclude=CHANGELOG.md --exclude=README.md --exclude=LICENSE && rm /tmp/$F && /usr/local/bin/node --version"
incus file push -r "$R/deploy/image/ai" "$I/tmp/" --project "$P" >/dev/null
X env PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin AI_PKGS="$PKGS" AI_OPTIONAL_PKGS="$OPT" bash -euc '/tmp/ai/install.sh; rm -rf /tmp/ai'
X su - dev -c 'command -v claude codex; claude --version; codex --version; echo CLAUDE_CONFIG_DIR=$CLAUDE_CONFIG_DIR'
X sh -c 'install -d -o dev -g dev -m 0700 /home/dev/.ai /home/dev/.ai/claude /home/dev/.ai/codex /home/dev/.ai/grok /home/dev/.ai/shared; ls -ld /home/dev/.ai'
