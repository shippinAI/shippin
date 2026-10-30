#!/bin/sh
set -eu

# Development image only; see README.md before use.
version=13.7.0
iso="debian-${version}-amd64-netinst.iso"
url="https://cdimage.debian.org/debian-cd/${version}/amd64/iso-cd"
output=/output/shippin-primary-development.iso
test -d /output || { echo 'Mount an output directory at /output.' >&2; exit 1; }
test ! -e "$output" || { echo "Refusing to overwrite $output" >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

curl --fail --location --silent --show-error "$url/SHA256SUMS" -o "$work/SHA256SUMS"
curl --fail --location --silent --show-error "$url/SHA256SUMS.sign" -o "$work/SHA256SUMS.sign"
gpgv --status-fd 1 --keyring /usr/share/keyrings/debian-role-keys.pgp \
  "$work/SHA256SUMS.sign" "$work/SHA256SUMS" > "$work/gpg-status"
python3 - "$work/gpg-status" <<'PY'
from pathlib import Path
import sys
valid = {"10460DAD76165AD81FBC0CE9988021A964E6EA7D", "DF9B9C49EAA9298432589D76DA87E80D6294BE9B"}
lines = Path(sys.argv[1]).read_text().splitlines()
if not any(line.startswith("[GNUPG:] VALIDSIG ") and line.split()[2] in valid for line in lines):
    raise SystemExit("Checksum signature was not made by an expected Debian CD signing key")
PY
if [ -f "/input/$iso" ]; then
  cp "/input/$iso" "$work/$iso"
else
  curl --fail --location --silent --show-error "$url/$iso" -o "$work/$iso"
fi
(cd "$work" && grep "  $iso\$" SHA256SUMS | sha256sum --check --strict)
xorriso -osirrox on -indev "$work/$iso" \
  -extract /boot/grub/grub.cfg "$work/grub.cfg" \
  -extract /isolinux/txt.cfg "$work/txt.cfg" \
  -extract /isolinux/gtk.cfg "$work/gtk.cfg" \
  -extract /md5sum.txt "$work/md5sum.txt"
python3 /usr/local/lib/shippin-image/patch-boot-menu.py "$work"
xorriso -indev "$work/$iso" -outdev "$work/shippin-primary-development.iso" \
  -boot_image any replay \
  -map "$work/grub.cfg" /boot/grub/grub.cfg \
  -map "$work/txt.cfg" /isolinux/txt.cfg \
  -map "$work/gtk.cfg" /isolinux/gtk.cfg \
  -map "$work/md5sum.txt" /md5sum.txt
cp "$work/shippin-primary-development.iso" "$output.partial"
mv "$output.partial" "$output"
(cd /output && sha256sum shippin-primary-development.iso > shippin-primary-development.iso.sha256)
echo "Development ISO created: $output"
