#!/bin/bash
# Fail when the public tree still contains site addresses or site paths.
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root"
patterns=(
  '192\.168\.1\.'
  '206\.109\.207\.'
  '23\.156\.136\.'
  '100\.84\.'
  '10\.20\.0\.'
  '104\.176\.44\.'
  '/root/homebase'
  '/home/hades'
  'rrflow\.ai'
  'probablydns\.com'
  'systemfreaks\.com'
  '01a14149-df75-780b-812f-8415b98628c7'
  '395263680805142530'
  '395268604699869186'
  'PROXMOX_VERIFY_TLS=false'
  'InsecureSkipVerify: true'
)
failed=0
while IFS= read -r file; do
  for pattern in "${patterns[@]}"; do
    if grep -nE "$pattern" "$file" >/dev/null; then
      echo "site literal ${pattern} in ${file}" >&2
      grep -nE "$pattern" "$file" >&dev/stderr || true
      failed=1
    fi
  done
done < <(git ls-files | grep -vE '(^|/)(check-public-tree\.sh|merge-from-shippin-console\.md)$' || true)
# Tailscale CGNAT addresses are site data. Documentation of the range in a comment is rejected too.
if git ls-files | xargs grep -nE '100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.' >/dev/null 2>&1; then
  echo "tailscale CGNAT address found" >&2
  git ls-files | xargs grep -nE '100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.' >&2 || true
  failed=1
fi
exit "$failed"
