#!/usr/bin/env python3
"""Run only in the disposable fixture. Real systemd/nft/sshd; Incus transport and ttyd mocked."""
import json
import hashlib
import os
from pathlib import Path
import subprocess
import sys
from fixture_setup import setup, run


def main():
    source = Path(sys.argv[1])
    peer, bundle, digest, ssh = setup(source)
    launcher = (source / 'guard_launcher.py').read_text()
    print('EVIDENCE ' + json.dumps({'source': str(source), 'bundle': str(bundle), 'manifest_sha256': digest,
          'source_sha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in source.glob('*.py')},
          'inline_launcher': launcher}), flush=True)
    def action(name, backup, fail=None, kill=None, okay=True):
        Path('/run/guard-test-reload-count').unlink(missing_ok=True)
        env = dict(os.environ)
        if fail:
            env['TEST_FAIL_AFTER'] = fail
        if kill:
            env['TEST_KILL_AFTER'] = kill
        print('RUN ' + json.dumps({'argv': ['python3', '-I', '-c', 'EVIDENCE.inline_launcher', digest,
                  str(bundle), name, str(backup)], 'fault_after': fail, 'kill_after': kill}), flush=True)
        result = subprocess.run(['python3', '-I', '-c', launcher, digest, str(bundle), name, str(backup)],
                                env=env, text=True, capture_output=True, timeout=45)
        if okay and result.returncode:
            raise RuntimeError(result.stdout + result.stderr)
        if not okay and result.returncode == 0:
            raise RuntimeError('expected operation failure')
        return result
    def clean(backup):
        state = json.loads((backup / 'prepared.json').read_text())
        for name in state['files']:
            p = Path(name)
            if p.exists() or p.with_name('.' + p.name + '.shippin-pending').exists():
                raise RuntimeError(f'owned file remains: {p}')
        if run('nft', '-j', 'list', 'tables').find('shippin_bastion_guard') >= 0:
            raise RuntimeError('guard table remains')
        run('systemctl', 'is-active', '--quiet', 'ssh.service', 'bastion-web.service')
        if 'shippin-bastion-guard' in run('systemctl', 'show', 'ssh.service', '-p', 'Requires'):
            raise RuntimeError('dependency remains')
        if Path('/etc/systemd/system/multi-user.target.wants/shippin-bastion-guard.service').exists():
            raise RuntimeError('enabled unit link remains')
        if Path('/etc/nftables.d/unrelated-test-file').read_text() != 'preserve me':
            raise RuntimeError('unrelated state changed')
    Path('/etc/nftables.d/unrelated-test-file').write_text('preserve me')
    live = None
    try:
        backup = Path('/root/backups/happy')
        action('check', backup)
        live = subprocess.Popen(ssh + ['echo READY; read reply; echo SURVIVED'],
                                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        if live.stdout.readline().strip() != 'READY':
            raise RuntimeError('could not open established SSH test session')
        action('apply', backup)
        action('verify', backup)
        run(*ssh, 'true')
        stdout, stderr = live.communicate('continue\n', timeout=5)
        if live.returncode or stdout.strip() != 'SURVIVED':
            raise RuntimeError('established SSH did not survive: ' + stderr)
        print('PASS check/apply/verify; established and fresh SSH survive', flush=True)
        rules = Path('/etc/nftables.d/shippin-bastion.nft')
        original = rules.read_text()
        rules.write_text('deliberately invalid nft syntax\n')
        result = subprocess.run(['systemctl', 'reload', 'shippin-bastion-guard.service'], capture_output=True)
        if result.returncode == 0:
            raise RuntimeError('malformed reload unexpectedly succeeded')
        rules.write_text(original)
        action('verify', backup)
        print('PASS malformed reload retains existing rules', flush=True)
        run('nft', 'destroy', 'table', 'inet', 'shippin_bastion_guard')
        action('verify', backup, okay=False)
        print('PASS verifier detects table loss; continuous fail-closed protection NOT established', flush=True)
        action('rollback', backup); action('rollback', backup)
        clean(backup)
        print('PASS actual rollback and repeat rollback preserve services/unrelated state', flush=True)
        for fault in ['files-installed', 'daemon-reload-1', 'enable']:
            backup = Path('/root/backups') / fault
            action('apply', backup, fail=fault, okay=False)
            action('rollback', backup); clean(backup)
            print(f'PASS actual rollback after lost result at {fault}', flush=True)
        for fault in ['dropins-removed', 'daemon-reload-1', 'guard-disabled',
                      'files-removed', 'daemon-reload-2', 'table-removed']:
            for mode in ['exception', 'kill']:
                backup = Path('/root/backups') / (mode + '-' + fault)
                action('apply', backup)
                action('rollback', backup, fail=fault if mode == 'exception' else None,
                       kill=fault if mode == 'kill' else None, okay=False)
                action('rollback', backup); clean(backup)
                print(f'PASS rollback retry after {mode} at {fault}', flush=True)
    finally:
        if live is not None and live.poll() is None:
            live.kill(); live.wait()
        peer.terminate(); peer.wait(timeout=5)


if __name__ == '__main__':
    main()
