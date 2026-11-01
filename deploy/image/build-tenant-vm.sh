#!/bin/bash
# Build the tenant VM image: Debian 13 cloud VM + Incus (Zabbly lts-6.0, key pinned) + btrfs-progs,
# no network listener. Published as $ALIAS (default shippin/tenant-vm). The builder is removed after.
set -euo pipefail
P="--project platform"; BASE=${BASE:-shippin/debian13-cloud-vm}; ALIAS=${ALIAS:-shippin/tenant-vm}
FPR=4EFC590696CB15B87C73A3AD82CC8797C838DCFD; B=tvm-builder-$$; HERE=$(dirname "$0")/tenant-vm
incus init -q "$BASE" $B --vm $P -c limits.cpu=4 -c limits.memory=4GiB -d root,size=10GiB </dev/null
incus start $B $P </dev/null
for i in $(seq 90); do incus exec $B $P -- true </dev/null 2>/dev/null && break; sleep 2; done
incus exec $B $P -- cloud-init status --wait </dev/null >/dev/null || true
# OVN networks run at MTU 1442; systemd-networkd ignores the DHCP MTU option unless UseMTU=yes,
# and a 1500 guest MTU silently drops large packets (seen as stalled apt/https downloads).
incus exec $B $P -- bash -euxc '
mkdir -p /etc/systemd/network/10-cloud-init-enp5s0.network.d
printf "[DHCPv4]\nUseMTU=yes\n" > /etc/systemd/network/10-cloud-init-enp5s0.network.d/mtu.conf
networkctl reload; networkctl reconfigure enp5s0; sleep 3; ip link show enp5s0' </dev/null
incus exec $B $P --env FPR=$FPR -- bash -euxc '
export DEBIAN_FRONTEND=noninteractive
apt-get -qq update && apt-get -qq install -y --no-install-recommends curl gpg ca-certificates btrfs-progs >/dev/null
mkdir -p /etc/apt/keyrings && curl -fsSL https://pkgs.zabbly.com/key.asc -o /tmp/z.asc
test "$(gpg --show-keys --with-colons /tmp/z.asc | awk -F: "/^fpr/{print \$10; exit}")" = "$FPR"
mv /tmp/z.asc /etc/apt/keyrings/zabbly.asc
printf "Enabled: yes\nTypes: deb\nURIs: https://pkgs.zabbly.com/incus/lts-6.0\nSuites: trixie\nComponents: main\nArchitectures: amd64\nSigned-By: /etc/apt/keyrings/zabbly.asc\n" > /etc/apt/sources.list.d/zabbly-incus-lts-6.0.sources
apt-get -qq update && apt-get -qq install -y --no-install-recommends incus-base >/dev/null
systemctl enable incus.socket
echo "net.ipv4.ip_forward=1" > /etc/sysctl.d/90-shippin.conf
cloud-init clean --logs --machine-id; apt-get clean; rm -rf /var/lib/apt/lists/*' </dev/null
incus file push "$HERE/shippin-tvm-init" $B/usr/local/sbin/shippin-tvm-init $P --mode 0755
incus stop $B $P </dev/null
incus image alias delete $ALIAS </dev/null 2>/dev/null || true  # keep the old image, move the alias
incus publish $B $P --alias $ALIAS description="Shippin tenant VM (Debian 13, Incus lts-6.0, btrfs)" </dev/null
incus delete $B $P </dev/null
incus image list $ALIAS </dev/null
