#!/usr/bin/env python3
"""Behavioral proof: run as root ONLY inside `unshare --net`; no host links touched."""
import os
import socket
import subprocess
import time
from render_guard import render


def run(*args, input=None):
    return subprocess.run(args, input=input, text=True, capture_output=True,
                          check=True, timeout=10).stdout


def main():
    own = os.readlink("/proc/self/ns/net")
    if os.geteuid() != 0 or own == os.readlink("/proc/1/ns/net"):
        raise SystemExit("Refusing: run with sudo unshare --net; never in the host namespace")
    peer = subprocess.Popen(["unshare", "--net", "sleep", "60"])
    listeners = []
    existing = None
    try:
        for _ in range(100):
            if os.readlink(f"/proc/{peer.pid}/ns/net") != own:
                break
            time.sleep(.02)
        else:
            raise RuntimeError("peer namespace was not created")
        ns = ["nsenter", f"--net=/proc/{peer.pid}/ns/net"]
        run("ip", "link", "set", "lo", "up")
        run(*ns, "ip", "link", "set", "lo", "up")
        for iface, other, local, remote in [
            ("eth0", "p0", "192.0.2.2/24", "192.0.2.10/24"),
            ("tailscale0", "p1", "198.51.100.2/24", "198.51.100.10/24")]:
            run("ip", "link", "add", iface, "type", "veth", "peer", "name", other)
            run("ip", "link", "set", other, "netns", str(peer.pid))
            run("ip", "addr", "add", local, "dev", iface)
            run("ip", "link", "set", iface, "up")
            run(*ns, "ip", "addr", "add", remote, "dev", other)
            run(*ns, "ip", "link", "set", other, "up")
        run(*ns, "ip", "addr", "add", "192.0.2.50/24", "dev", "p0")
        run("ip", "-6", "addr", "add", "2001:db8::2/64", "dev", "eth0", "nodad")
        run(*ns, "ip", "-6", "addr", "add", "2001:db8::10/64", "dev", "p0", "nodad")
        run("ip", "-6", "addr", "add", "2001:db8:1::2/64", "dev", "tailscale0", "nodad")
        run(*ns, "ip", "-6", "addr", "add", "2001:db8:1::10/64", "dev", "p1", "nodad")
        for family, host, port in [(socket.AF_INET, "0.0.0.0", 7681),
                                   (socket.AF_INET, "0.0.0.0", 22),
                                   (socket.AF_INET6, "::", 7681),
                                   (socket.AF_INET6, "::", 22),
                                   (socket.AF_INET, "0.0.0.0", 9090)]:
            listener = socket.socket(family, socket.SOCK_STREAM)
            if family == socket.AF_INET6:
                listener.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
            listener.bind((host, port))
            listener.listen(32)
            listeners.append(listener)
        client = """import socket,sys
s=socket.socket(socket.AF_INET6 if ':' in sys.argv[1] else socket.AF_INET,socket.SOCK_STREAM)
s.settimeout(1)
if sys.argv[3] != '-': s.bind((sys.argv[3],0))
try: s.connect((sys.argv[1],int(sys.argv[2]))); print('ALLOW')
except OSError: print('DENY')
finally: s.close()
"""
        def probe(host, port, source="-", local=False):
            return run(*([] if local else ns), "python3", "-c", client, host, str(port), source).strip()
        assert probe("192.0.2.2", 7681, "192.0.2.50") == "ALLOW"
        persistent = """import socket,sys
s=socket.socket(); s.settimeout(2); s.bind(('192.0.2.50',0)); s.connect(('192.0.2.2',7681))
print('CONNECTED',flush=True); sys.stdin.readline()
try: s.sendall(b'x'); s.recv(1); print('OPEN')
except OSError: print('BLOCKED')
finally: s.close()
"""
        existing = subprocess.Popen(ns + ["python3", "-c", persistent], stdin=subprocess.PIPE,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        assert existing.stdout.readline().strip() == "CONNECTED"
        rules = render({"edge_ipv4": "192.0.2.10", "edge_interface": "eth0",
                        "ssh_interfaces": ["lo", "tailscale0"], "ssh_port": 22,
                        "terminal_port": 7681})["shippin-bastion.nft"]
        run("nft", "--check", "--file", "-", input=rules)
        run("nft", "--file", "-", input=rules)
        output, errors = existing.communicate("probe\n", timeout=4)
        assert existing.returncode == 0 and output.strip() == "BLOCKED", errors
        print("PASS existing non-edge TCP flow rejected on its next packet (not idle WebSocket revocation)")
        cases = [("edge terminal", "192.0.2.2", 7681, "192.0.2.10", False, "ALLOW"),
                 ("non-edge terminal", "192.0.2.2", 7681, "192.0.2.50", False, "DENY"),
                 ("platform SSH", "192.0.2.2", 22, "192.0.2.10", False, "DENY"),
                 ("tailnet SSH", "198.51.100.2", 22, "-", False, "ALLOW"),
                 ("loopback SSH", "127.0.0.1", 22, "-", True, "ALLOW"),
                 ("IPv6 terminal", "2001:db8::2", 7681, "-", False, "DENY"),
                 ("IPv6 platform SSH", "2001:db8::2", 22, "-", False, "DENY"),
                 ("IPv6 tailnet SSH", "2001:db8:1::2", 22, "-", False, "ALLOW"),
                 ("IPv6 loopback SSH", "::1", 22, "-", True, "ALLOW"),
                 ("unrelated port", "192.0.2.2", 9090, "192.0.2.50", False, "ALLOW")]
        for label, host, port, source, local, expected in cases:
            actual = probe(host, port, source, local)
            assert actual == expected, f"{label}: expected {expected}, got {actual}"
            print(f"PASS {label}: {actual}")
        run("nft", "--file", "-", input=rules)
        assert probe("192.0.2.2", 7681, "192.0.2.50") == "DENY"
        print("PASS atomic reload retains rejection")
        run("nft", "delete", "table", "inet", "shippin_bastion_guard")
        assert probe("192.0.2.2", 7681, "192.0.2.50") == "ALLOW"
        print("PASS nft-table removal restores baseline (not installer rollback)")
    finally:
        if existing is not None and existing.poll() is None:
            existing.terminate()
            existing.wait(timeout=5)
        for listener in listeners:
            listener.close()
        peer.terminate()
        peer.wait(timeout=5)


if __name__ == "__main__":
    main()
