#!/bin/bash
# Load per-tenant ZFS keys at boot (before incus) from the sops key store, via stdin only.
set -uo pipefail
K=/var/lib/shippin/site/secrets/zfs-keys.sops.yaml
zfs list -H -o name,shippin:tenant,keystatus -t filesystem -r tank/tenants | while read -r ds t ks; do
  [ "$t" = "-" ] || [ "$ks" != "unavailable" ] && continue
  if sops decrypt --extract "[\"$t\"]" "$K" | zfs load-key -L prompt "$ds"; then echo "unlocked $ds"; else echo "FAILED $ds"; fi
done
zfs mount -a || true
