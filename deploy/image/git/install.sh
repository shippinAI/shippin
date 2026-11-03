#!/bin/bash
# Runs as root inside the image builder (build-image.sh). GitHub App imports: a credential helper
# reading the workspace's tmpfs token, the gh CLI wrapper, and the token directory.
set -euo pipefail
HERE=$(dirname "$0")
install -m 0755 "$HERE/shippin-git-cred" /usr/local/sbin/shippin-git-cred
install -m 0644 "$HERE/shippin-git.sh" /etc/profile.d/shippin-git.sh
grep -q shippin-git.sh /etc/bash.bashrc || echo '[ -r /etc/profile.d/shippin-git.sh ] && . /etc/profile.d/shippin-git.sh' >> /etc/bash.bashrc
install -m 0644 "$HERE/shippin-git.tmpfiles" /etc/tmpfiles.d/shippin-git.conf
git config --system credential.https://github.com.helper /usr/local/sbin/shippin-git-cred
git config --system credential.https://github.com.useHttpPath false
