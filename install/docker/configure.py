#!/usr/bin/env python3
"""Initialize missing local Compose credentials without changing existing settings."""
from pathlib import Path
import os
import secrets

root = Path(__file__).resolve().parents[2]
target = root / ".env"
existing = target.read_text(encoding="utf-8") if target.exists() else ""
keys = {line.split("=", 1)[0].strip() for line in existing.splitlines() if "=" in line and not line.lstrip().startswith("#")}
new = []
for key in ("SHIPPIN_DB_PASSWORD", "SHIPPIN_DRAGONFLY_PASSWORD"):
    if key not in keys and not os.environ.get(key):
        new.append(f"{key}={secrets.token_hex(32)}")
if new:
    with target.open("a", encoding="utf-8", newline="\n") as out:
        if existing and not existing.endswith("\n"):
            out.write("\n")
        out.write("\n".join(new) + "\n")
    target.chmod(0o600)
print("Local Compose credentials are configured. Existing settings were preserved.")
