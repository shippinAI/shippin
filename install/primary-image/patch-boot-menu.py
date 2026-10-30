#!/usr/bin/env python3
"""Modify only the two normal Debian installer choices, failing on drift."""
from pathlib import Path
import hashlib
import sys


def replace_exact(path: Path, old: str, new: str, count: int) -> None:
    text = path.read_text(encoding="utf-8")
    actual = text.count(old)
    if actual != count:
        raise SystemExit(f"unexpected boot menu {path}: expected {count} matches, got {actual}")
    path.write_text(text.replace(old, new, count), encoding="utf-8")


if len(sys.argv) != 2:
    raise SystemExit("usage: patch-boot-menu.py DIRECTORY")

directory = Path(sys.argv[1])
grub = directory / "grub.cfg"
grub_text = grub.read_text(encoding="utf-8")
marker = "    linux    /install.amd/vmlinuz vga=788 --- quiet "
if grub_text.count(marker) != 2:
    raise SystemExit("unexpected GRUB boot menu: normal entries changed")
replace_exact(grub, marker, marker.replace(" ---", " pkgsel/include=incus ---"), 2)
for name in ("txt.cfg", "gtk.cfg"):
    path = directory / name
    text = path.read_text(encoding="utf-8")
    if text.count(" --- quiet ") != 1:
        raise SystemExit(f"unexpected ISOLINUX boot menu: {name}")
    replace_exact(path, " --- quiet ", " pkgsel/include=incus --- quiet ", 1)

# Debian's optional media check includes grub.cfg. Keep the original manifest
# accurate for the modified file; MD5 here is Debian's media-check format, not
# an artifact authentication mechanism.
manifest = directory / "md5sum.txt"
lines = manifest.read_text(encoding="utf-8").splitlines(keepends=True)
target = "  ./boot/grub/grub.cfg\n"
matches = [index for index, line in enumerate(lines) if line.endswith(target)]
if len(matches) != 1:
    raise SystemExit("unexpected Debian media checksum manifest")
lines[matches[0]] = hashlib.md5(grub.read_bytes()).hexdigest() + target
manifest.write_text("".join(lines), encoding="utf-8")
