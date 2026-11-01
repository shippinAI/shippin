"""Isolated container fixtures: systemd is real; terminal is an HTTP test double."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
from render_guard import render


def run(*cmd, **kwargs):
    return subprocess.run(cmd, check=True, capture_output=True, text=True, timeout=20, **kwargs).stdout


def setup(source):
    if not Path('/run/.containerenv').exists() or not Path('/root/guard-test-only').exists():
        raise RuntimeError('disposable Podman test container required')
    peer = subprocess.Popen(['unshare', '--net', 'sleep', '1800'])
    own = os.readlink('/proc/self/ns/net')
    for _ in range(100):
        if os.readlink(f'/proc/{peer.pid}/ns/net') != own:
            break
        time.sleep(.02)
    else:
        raise RuntimeError('network namespace unavailable')
    Path('/run/guard-test-peer').write_text(str(peer.pid))
    ns = ['nsenter', f'--net=/proc/{peer.pid}/ns/net']
    run(*ns, 'ip', 'link', 'set', 'lo', 'up')
    for iface, other, local, remote in [
        ('eth0', 'p0', '192.0.2.2/24', '192.0.2.10/24'),
        ('tailscale0', 'p1', '198.51.100.2/24', '198.51.100.10/24')]:
        run('ip', 'link', 'add', iface, 'type', 'veth', 'peer', 'name', other)
        run('ip', 'link', 'set', other, 'netns', str(peer.pid))
        run('ip', 'addr', 'add', local, 'dev', iface)
        run('ip', 'link', 'set', iface, 'up')
        run(*ns, 'ip', 'link', 'set', other, 'name', iface)
        run(*ns, 'ip', 'addr', 'add', remote, 'dev', iface)
        run(*ns, 'ip', 'link', 'set', iface, 'up')
    for ip in ['192.0.2.50/24', '192.0.2.90/24']:
        run(*ns, 'ip', 'addr', 'add', ip, 'dev', 'eth0')
    Path('/etc/systemd/system/bastion-web.service').write_text('''[Unit]
Description=Isolated HTTP test double (not ttyd)
[Service]
ExecStart=/usr/bin/python3 -m http.server 7681 --bind 192.0.2.2
[Install]
WantedBy=multi-user.target
''')
    run('systemctl', 'daemon-reload')
    run('systemctl', 'enable', '--now', 'bastion-web.service')
    run('systemctl', 'disable', 'nftables.service')
    shutil.copyfile(source / 'tests/incus_fixture.py', '/usr/local/bin/incus')
    Path('/usr/local/bin/incus').chmod(0o755)
    guard = {'edge_ipv4': '192.0.2.10', 'edge_interface': 'eth0',
             'ssh_interfaces': ['lo', 'tailscale0'], 'ssh_port': 22, 'terminal_port': 7681}
    runtime = {'instance': 'bastion', 'project': 'fixture', 'bastion_ipv4': '192.0.2.2',
               'edge_instance': 'edge', 'deny_probe_instances': ['console', 'mcp'],
               'listener_executables': {'ssh.service': '/usr/sbin/sshd',
                                        'bastion-web.service': str(Path('/usr/bin/python3').resolve())}}
    bundle = Path('/root/bundle'); bundle.mkdir()
    for name in ['guard_change', 'guard_checks', 'guard_files']:
        shutil.copyfile(source / (name + '.py'), bundle / (name + '.py'))
    for name, content in {**render(guard), 'guard-config.json': json.dumps(guard),
                          'runtime.json': json.dumps(runtime)}.items():
        (bundle / name).write_text(content)
    manifest = ''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n'
                       for p in sorted(bundle.iterdir()))
    (bundle / 'SHA256SUMS').write_text(manifest)
    Path('/root/backups').mkdir()
    # Test-only ephemeral key, confined to this networkless container.
    Path('/root/.ssh').mkdir(mode=0o700, exist_ok=True)
    run('ssh-keygen', '-q', '-t', 'ed25519', '-N', '', '-f', '/root/.ssh/test_key')
    shutil.copyfile('/root/.ssh/test_key.pub', '/root/.ssh/authorized_keys')
    key = Path('/etc/ssh/ssh_host_ed25519_key.pub').read_text().split()[:2]
    Path('/root/.ssh/known_hosts').write_text('198.51.100.2 ' + ' '.join(key) + '\n')
    ssh = ns + ['ssh', '-i', '/root/.ssh/test_key', '-o', 'BatchMode=yes',
                '-o', 'StrictHostKeyChecking=yes', '-o', 'ConnectTimeout=3', 'root@198.51.100.2']
    return peer, bundle, hashlib.sha256(manifest.encode()).hexdigest(), ssh
