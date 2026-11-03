#!/bin/bash
# One-time (idempotent): scalable tenant networking (07b).
# - shippin-transit: host bridge (NAT) used as the OVN uplink for tenant networks, so a tenant's
#   OVN router IP comes from 172.31.0.0/16 instead of the LAN (room for ~65k tenants).
# - ws-hub: OVN network on that uplink in the platform project; every tenant net peers with it.
# - edge gets eth1 on ws-hub (198.51.100.10), a route to 10.128.0.0/9 (tenant subnets), IP
#   forwarding and an nft table (/etc/shippin/ports.nft, managed by shippin-hostd) that DNATs
#   shared-endpoint ports (SSH, published ports) to workspaces.
set -euo pipefail
P="--project platform"; TR=${TRANSIT:-shippin-transit}; HUB=${HUB:-ws-hub}
HUBGW=${HUB_GW:?set HUB_GW}; HUBIP=${HUB_IP:?set HUB_IP}; TCIDR=${TENANT_CIDR:?set TENANT_CIDR}
incus network show $TR >/dev/null 2>&1 || incus network create $TR --type=bridge \
  ipv4.address=172.31.0.1/16 ipv4.nat=true ipv6.address=none \
  ipv4.dhcp.ranges=172.31.0.2-172.31.0.250 ipv4.ovn.ranges=172.31.1.0-172.31.255.254 </dev/null
incus network show $HUB $P >/dev/null 2>&1 || incus network create $HUB $P --type=ovn \
  network=$TR ipv4.address=$HUBGW/24 ipv4.nat=true ipv6.address=none </dev/null
incus config device show edge $P | grep -q '^eth1:' || \
  incus config device add edge eth1 nic network=$HUB name=eth1 ipv4.address=$HUBIP $P </dev/null
incus exec edge $P -- sh -euc "
printf '[Match]\nName=eth1\n\n[Network]\nAddress=$HUBIP/24\nDHCP=no\n\n[Route]\nDestination=$TCIDR\nGateway=$HUBGW\n' > /etc/systemd/network/eth1.network
networkctl reload
echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/90-shippin.conf; sysctl -q -p /etc/sysctl.d/90-shippin.conf
command -v nft >/dev/null || (apt-get -qq update && DEBIAN_FRONTEND=noninteractive apt-get -qq install -y nftables >/dev/null)
mkdir -p /etc/shippin
[ -f /etc/shippin/ports.nft ] || printf 'table ip shippin {\n}\n' > /etc/shippin/ports.nft
printf '#!/usr/sbin/nft -f\ninclude \"/etc/shippin/ports.nft\"\n' > /etc/nftables.conf
systemctl enable -q nftables; nft -f /etc/nftables.conf
ip route | grep -E 'eth1'; sysctl net.ipv4.ip_forward
" </dev/null
mkdir -p /var/lib/shippin && chmod 700 /var/lib/shippin
