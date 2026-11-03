#!/bin/bash
# Runs as root inside the image builder (build-image.sh, AI=1). Preinstalls the vendor AI CLIs and
# the activation plumbing. No login is ever baked in: each user signs in to their OWN subscription
# inside their own container and the login lives on their AI home volume (~/.ai), never here.
set -euo pipefail
HERE=$(dirname "$0")
# AI_PKGS / AI_OPTIONAL_PKGS: npm specs; pin with name@x.y.z. Optional ones may fail without
# failing the build (their CLI then shows "not installed").
npm install -g --no-audit --no-fund ${AI_PKGS:?} >/dev/null
for p in ${AI_OPTIONAL_PKGS:-}; do npm install -g --no-audit --no-fund "$p" >/dev/null || echo "optional AI CLI $p not installed" >&2; done
install -d -m 0755 /etc/shippin
npm ls -g --depth=0 --json > /etc/shippin/ai-versions.json
install -m 0644 "$HERE/shippin-ai.sh" /etc/profile.d/shippin-ai.sh
grep -q shippin-ai.sh /etc/bash.bashrc || echo '[ -r /etc/profile.d/shippin-ai.sh ] && . /etc/profile.d/shippin-ai.sh' >> /etc/bash.bashrc
install -m 0644 "$HERE/ai-probes" /etc/shippin/ai-probes
install -m 0755 "$HERE/shippin-ai-status" /usr/local/sbin/shippin-ai-status
install -m 0755 "$HERE/shippin-chat-turn" /usr/local/bin/shippin-chat-turn
install -m 0644 "$HERE/shippin-ai.tmpfiles" /etc/tmpfiles.d/shippin-ai.conf
for u in shippin-ai-status.service shippin-ai-status.timer shippin-ttyd.service; do install -m 0644 "$HERE/$u" /etc/systemd/system/$u; done
systemctl enable shippin-ai-status.timer
# Guard: the image must carry no vendor login state (only each user's AI home does). Installers may
# scaffold an empty config dir (e.g. ~/.grok); that is removed, but anything credential-like refuses.
rm -rf /root/.npm /root/.cache
for d in /root /etc/skel; do
  for n in .claude .claude.json .codex .grok .ai .config/claude; do
    [ -e "$d/$n" ] || continue
    if [ -n "$(find "$d/$n" \( -iname '*auth*' -o -iname '*credential*' -o -iname '*token*' -o -iname '*session*' -o -iname '*.key' \) -print -quit)" ]; then
      echo "refusing to publish: $d/$n holds login-like files" >&2; exit 1
    fi
    echo "removing install-time scaffolding $d/$n: $(find "$d/$n" -type f -printf '%P ' | head -c 300)" >&2
    rm -rf "${d:?}/$n"
  done
done
