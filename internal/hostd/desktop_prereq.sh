#!/bin/bash
# Desktop prerequisites for a tenant VM. Idempotent. Does not install Claude, Codex, ChatGPT
# or Grok, and does not read vendor credentials. hostd runs this only inside tvm-<tenant>.
# SHIPPIN_ROOT prefixes every path so tests can run it against a fixture.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
R=${SHIPPIN_ROOT:-}
r() { printf '%s%s' "$R" "$1"; }
security_change=0
ini_changed=0

if [ -n "$R" ]; then
  sudo() { "$@"; }
fi

ini_set() {
  local file=$1 section=$2 key=$3 val=$4 tmp
  mkdir -p "$(dirname "$file")"
  [ -f "$file" ] || : >"$file"
  tmp=$(mktemp)
  awk -v section="$section" -v key="$key" -v val="$val" '
    BEGIN { seen=0; insec=0; found=0 }
    /^\[/ {
      if (insec && !seen) { print key "=" val; seen=1 }
      insec = ($0 == "[" section "]")
      if (insec) found=1
      print
      next
    }
    insec && $0 ~ "^[ \t]*" key "[ \t]*=" {
      if ($0 != key "=" val) print key "=" val
      else print
      seen=1
      next
    }
    { print }
    END {
      if (!found) { print ""; print "[" section "]"; print key "=" val }
      else if (insec && !seen) print key "=" val
    }
  ' "$file" >"$tmp"
  ini_changed=0
  if ! cmp -s "$file" "$tmp"; then
    mv "$tmp" "$file"
    ini_changed=1
  else
    rm -f "$tmp"
  fi
}

pam_line() {
  local file=$1 line=$2
  mkdir -p "$(dirname "$file")"
  touch "$file"
  grep -qxF "$line" "$file" && return 0
  printf '%s\n' "$line" >>"$file"
}

write_if_changed() {
  local file=$1 tmp
  mkdir -p "$(dirname "$file")"
  tmp=$(mktemp)
  cat >"$tmp"
  if [ -f "$file" ] && cmp -s "$file" "$tmp"; then rm -f "$tmp"; return 0; fi
  mv "$tmp" "$file"
}

# Cowork packages follow https://code.claude.com/docs/en/desktop-linux :
# amd64 wants qemu-system-x86 and ovmf; arm64 wants qemu-system-arm and qemu-efi-aarch64.
arch=$(dpkg --print-architecture 2>/dev/null || echo amd64)
case "$arch" in
  arm64) qemu_pkgs="qemu-system-arm qemu-efi-aarch64 virtiofsd" ;;
  *) qemu_pkgs="qemu-system-x86 ovmf virtiofsd" ;;
esac

apt-get update -qq
apt-get install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold \
  curl gnupg ca-certificates \
  xfce4 xfce4-terminal xrdp xorgxrdp \
  gnome-keyring dbus-user-session \
  $qemu_pkgs

# xrdp listens on loopback only. An existing 127.0.0.1 bind is left alone.
# A restart happens only when the listener or the disconnect policy had to change,
# so a VM that was set up by hand (address already loopback, KillDisconnected already
# false) is not bounced.
ini_set "$(r /etc/xrdp/xrdp.ini)" Globals address 127.0.0.1
if [ "$ini_changed" = 1 ]; then security_change=1; fi
ini_set "$(r /etc/xrdp/xrdp.ini)" Globals port 3389
if [ "$ini_changed" = 1 ]; then security_change=1; fi
# sesman.ini(5): KillDisconnected=false means a disconnect does not kill the session.
# That is what keeps Claude Desktop running when the browser tab closes.
# The man page says this applies to xorgxrdp sessions, which is why xorgxrdp is installed.
ini_set "$(r /etc/xrdp/sesman.ini)" Sessions KillDisconnected false
if [ "$ini_changed" = 1 ]; then security_change=1; fi
ini_set "$(r /etc/xrdp/sesman.ini)" Sessions DisconnectedTimeLimit 0
ini_set "$(r /etc/xrdp/sesman.ini)" Sessions IdleTimeLimit 0
if ! grep -q '^Policy=' "$(r /etc/xrdp/sesman.ini)"; then
  ini_set "$(r /etc/xrdp/sesman.ini)" Sessions Policy Default
fi

pam_line "$(r /etc/pam.d/xrdp-sesman)" 'auth optional pam_gnome_keyring.so'
pam_line "$(r /etc/pam.d/xrdp-sesman)" 'session optional pam_gnome_keyring.so auto_start'

write_if_changed "$(r /etc/systemd/logind.conf.d/shippin-desktop.conf)" <<'EOF'
[Login]
IdleAction=ignore
IdleActionSec=infinity
EOF

write_if_changed "$(r /etc/modules-load.d/vhost_vsock.conf)" <<'EOF'
vhost_vsock
EOF

write_if_changed "$(r /usr/local/bin/shippin-desktop-nudge)" <<'EOF'
#!/bin/bash
# Runs at XFCE login. Does not log anyone out. Disables the locker and DPMS.
xset s off -dpms 2>/dev/null || true
xfconf-query -c xfce4-screensaver -p /lock/enabled -n -t bool -s false 2>/dev/null || true
xfconf-query -c xfce4-power-manager -p /xfce4-power-manager/blank-on-ac -n -t int -s 0 2>/dev/null || true
xfconf-query -c xfce4-power-manager -p /xfce4-power-manager/dpms-enabled -n -t bool -s false 2>/dev/null || true
EOF
chmod 755 "$(r /usr/local/bin/shippin-desktop-nudge)" 2>/dev/null || true

write_if_changed "$(r /etc/xdg/autostart/shippin-no-lock.desktop)" <<'EOF'
[Desktop Entry]
Type=Application
Name=Shippin desktop nudge
Comment=Disable locker and DPMS so a remote desktop session stays unlocked
Exec=/usr/local/bin/shippin-desktop-nudge
OnlyShowIn=XFCE;
X-GNOME-Autostart-enabled=true
EOF

if command -v systemctl >/dev/null 2>&1; then
  systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target >/dev/null 2>&1 || true
  if ! systemctl is-active --quiet xrdp; then
    systemctl enable --now xrdp >/dev/null 2>&1 || true
  elif [ "$security_change" = 1 ]; then
    systemctl try-restart xrdp xrdp-sesman >/dev/null 2>&1 || true
  fi
fi
modprobe vhost_vsock >/dev/null 2>&1 || true

# kvm group for accounts in the per-org uid range (docs/reference/identity-local-accounts.md).
# This does not create accounts. Cowork needs the group for /dev/kvm and /dev/vhost-vsock.
if getent group kvm >/dev/null 2>&1 && [ -f "$(r /etc/passwd)" ]; then
  while IFS=: read -r name _ uid _ _ _ _; do
    case "$uid" in ''|*[!0-9]*) continue ;; esac
    if [ "$uid" -ge 10000 ] && [ "$uid" -le 59999 ]; then
      usermod -aG kvm "$name" >/dev/null 2>&1 || true
    fi
  done <"$(r /etc/passwd)"
fi

# Never rewrite a vendor apt source that is already there (the operator's hand install).
mkdir -p "$(r /var/lib/shippin)"
printf '%s\n' 'ready' >"$(r /var/lib/shippin/desktop-ready)"
echo "desktop prerequisites ok (xrdp security_change=$security_change)"
