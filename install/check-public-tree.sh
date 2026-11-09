#!/bin/bash
# Fail when the public tree holds what should not be public. A site's own literals (its addresses, paths,
# domains and ids) cannot be listed here, because that would publish them, so a site passes them in:
#
#   SHIPPIN_SITE_PATTERNS=/path/to/patterns.txt bash install/check-public-tree.sh
#
# One extended regular expression per line, with # for comments. A line starting "shipped " applies only to
# code that ships, so tests and documentation may still name it.
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root"
patterns=(
  'PROXMOX_VERIFY_TLS=false'
  'InsecureSkipVerify: true'
)
shipped=()
if [ -n "${SHIPPIN_SITE_PATTERNS:-}" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ''|'#'*) ;;
      'shipped '*) shipped+=("${line#shipped }") ;;
      *) patterns+=("$line") ;;
    esac
  done < "$SHIPPIN_SITE_PATTERNS"
else
  echo "SHIPPIN_SITE_PATTERNS is not set: generic checks only" >&2
fi
failed=0
while IFS= read -r file; do
  for pattern in "${patterns[@]}"; do
    if grep -nE "$pattern" "$file" >&2; then
      echo "${file}: holds a literal that must not be public" >&2
      failed=1
    fi
  done
done < <(git ls-files | grep -vE '(^|/)check-public-tree\.sh$' || true)
# A named domain or server in shipped code is a default someone else inherits. Configuration names them.
# An applied migration is history, and stays as it ran.
if [ "${#shipped[@]}" -gt 0 ]; then
  while IFS= read -r file; do
    for pattern in "${shipped[@]}"; do
      if grep -nE "$pattern" "$file" >&2; then
        echo "${file}: names something shipped code must not; move it to configuration" >&2
        failed=1
      fi
    done
  done < <(git ls-files 'internal/*' 'cmd/*' 'web/src/*' 'install/*' 'deploy/*' 'config/*' \
    | grep -vE '(_test\.go|\.test\.tsx?|check-public-tree\.sh|a2ui-types\.ts|shippin\.example\.json)$|^internal/db/migrations/' || true)
fi
# Tailscale CGNAT addresses are site data. Documentation of the range in a comment is rejected too.
if git ls-files | grep -vE '(^|/)check-public-tree\.sh$' | xargs grep -nE '100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.' >&2 2>/dev/null; then
  echo "tailscale CGNAT address found" >&2
  failed=1
fi
exit "$failed"
