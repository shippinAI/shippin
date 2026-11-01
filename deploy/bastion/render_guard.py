#!/usr/bin/env python3
"""Render a narrowly scoped bastion ingress guard; never apply it."""
import argparse
import ipaddress
import json
import pathlib
import re


def render(config):
    expected = {"edge_ipv4", "edge_interface", "ssh_interfaces", "ssh_port", "terminal_port"}
    if set(config) != expected:
        raise ValueError("configuration must contain exactly the documented guard fields")
    edge = ipaddress.IPv4Address(config["edge_ipv4"])
    if edge.is_unspecified or edge.is_multicast or edge.is_loopback:
        raise ValueError("edge_ipv4 must identify the edge, not a wildcard or loopback")
    if not isinstance(config["ssh_interfaces"], list):
        raise ValueError("ssh_interfaces must be an explicit list")
    interfaces = [config["edge_interface"], *config["ssh_interfaces"]]
    if not config["ssh_interfaces"] or any(
        not isinstance(v, str) or not re.fullmatch(r"[a-zA-Z0-9_.-]{1,15}", v)
        for v in interfaces
    ):
        raise ValueError("interface names must be explicit Linux interface names")
    if config["edge_interface"] in config["ssh_interfaces"]:
        raise ValueError("SSH must not be allowed from the platform-facing interface")
    ports = [config["ssh_port"], config["terminal_port"]]
    if any(type(v) is not int or not 1 <= v <= 65535 for v in ports) or len(set(ports)) != 2:
        raise ValueError("SSH and terminal ports must be distinct TCP ports")
    ssh_ifaces = ", ".join(json.dumps(v) for v in config["ssh_interfaces"])
    rules = f'''# This table owns only bastion SSH and terminal ingress.
# Do not add an established-flow bypass before the terminal rejection.
destroy table inet shippin_bastion_guard
table inet shippin_bastion_guard {{
  chain input {{
    type filter hook input priority -20; policy accept;
    tcp dport {ports[1]} iifname "{config['edge_interface']}" ip saddr {edge} counter accept
    tcp dport {ports[1]} counter reject with tcp reset
    tcp dport {ports[0]} iifname {{ {ssh_ifaces} }} counter accept
    tcp dport {ports[0]} counter reject with tcp reset
  }}
}}
'''
    service = '''[Unit]
Description=Shippin bastion ingress guard
Before=ssh.service bastion-web.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/sbin/nft --file /etc/nftables.d/shippin-bastion.nft
ExecReload=/usr/sbin/nft --file /etc/nftables.d/shippin-bastion.nft
NoNewPrivileges=yes
CapabilityBoundingSet=CAP_NET_ADMIN
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
'''
    dependency = '''[Unit]
Requires=shippin-bastion-guard.service
After=shippin-bastion-guard.service
'''
    return {"shippin-bastion.nft": rules, "shippin-bastion-guard.service": service,
            "10-shippin-bastion-guard.conf": dependency}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("config", type=pathlib.Path)
    parser.add_argument("output", type=pathlib.Path)
    args = parser.parse_args()
    files = render(json.loads(args.config.read_text(encoding="utf-8")))
    args.output.mkdir(parents=True, exist_ok=False)
    for name, content in files.items():
        (args.output / name).write_text(content, encoding="utf-8", newline="\n")


if __name__ == "__main__":
    main()
