#!/bin/bash
# Read-only status inside a tenant VM. Prints one JSON object.
# Does not execute vendor CLIs and does not read credential files or keyrings.
set -euo pipefail
R=${SHIPPIN_ROOT:-}
r() { printf '%s%s' "$R" "$1"; }

ver() {
  local pkg=$1 line
  line=$(dpkg-query -W -f '${Package} ${Version}\n' "$pkg" 2>/dev/null || true)
  case "$line" in
    "$pkg "[A-Za-z0-9.+~:-]*) printf '%s' "${line#"$pkg "}" ;;
  esac
}

has_bin() {
  local p
  for p in "$@"; do
    if [ -x "$p" ]; then return 0; fi
  done
  return 1
}

json_user() {
  local name=$1 uid=$2 home=$3 base cld=false cdesk=false codex=false gpt=false
  base=$(r "$home")
  if has_bin "$base/.local/bin/claude" "$(r /usr/bin/claude)" "$(r /usr/local/bin/claude)"; then cld=true; fi
  if has_bin "$(r /usr/bin/claude-desktop)" "$(r /usr/local/bin/claude-desktop)" "$base/.local/bin/claude-desktop"; then cdesk=true; fi
  if has_bin "$(r /usr/bin/codex)" "$(r /usr/local/bin/codex)" "$base/.local/bin/codex"; then codex=true; fi
  if has_bin "$(r /usr/bin/chatgpt)" "$(r /usr/local/bin/chatgpt)" "$base/.local/bin/chatgpt"; then gpt=true; fi
  printf '{"user":"%s","uid":%s,"claude":%s,"claude_desktop":%s,"codex":%s,"chatgpt":%s}' \
    "$name" "$uid" "$cld" "$cdesk" "$codex" "$gpt"
}

users=''
if [ -f "$(r /etc/passwd)" ]; then
  while IFS=: read -r name _ uid _ _ home _; do
    case "$name" in ''|*[!a-z0-9_-]*) continue ;; esac
    case "$uid" in ''|*[!0-9]*) continue ;; esac
    if [ "$uid" -ge 10000 ] && [ "$uid" -le 59999 ]; then
      [ -n "$users" ] && users="$users,"
      users="$users$(json_user "$name" "$uid" "$home")"
    fi
  done <"$(r /etc/passwd)"
fi

xrdp=false
ini=$(r /etc/xrdp/xrdp.ini)
if [ -f "$ini" ] && grep -q '^address=127.0.0.1$' "$ini" && grep -q '^port=3389$' "$ini"; then
  if command -v ss >/dev/null 2>&1; then
    if ss -H -ltn 2>/dev/null | grep -q '127.0.0.1:3389'; then xrdp=true; fi
  else
    xrdp=true
  fi
fi

grok=false
for d in "$(r /usr/share/applications)"; do
  [ -d "$d" ] || continue
  for f in "$d"/*[Gg]rok* "$d"/*[Gg]rok*.desktop; do
    [ -e "$f" ] && grok=true
  done
done

kvm=false
[ -e "$(r /dev/kvm)" ] && kvm=true

cdv=$(ver claude-desktop)
ccv=$(ver claude-code)
gpv=$(ver chatgpt)
printf '{"packages":{"claude-desktop":"%s","claude-code":"%s","chatgpt":"%s"},"users":[%s],"xrdp_loopback":%s,"grok_desktop":%s,"kvm":%s}\n' \
  "$cdv" "$ccv" "$gpv" "$users" "$xrdp" "$grok" "$kvm"
