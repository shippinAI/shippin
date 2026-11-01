#!/usr/bin/env python3
"""Gold-policy only. Invoke verified in-memory bytes through the ticket's inline launcher."""
import argparse
import json
import pathlib
import subprocess
from guard_files import atomic_json, protected, sync_dir
from guard_checks import inventory, require, rules, services


def main(bundle, digest):
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=['check', 'apply', 'rollback', 'verify'])
    p.add_argument('backup', type=pathlib.Path)
    args = p.parse_args()
    config = json.loads(bundle['runtime.json'])
    guard = json.loads(bundle['guard-config.json'])
    instance, project = config['instance'], config['project']
    def run(cmd):
        return subprocess.run(cmd, stdin=subprocess.DEVNULL, check=True,
                              capture_output=True, text=True, timeout=20).stdout
    def in_instance(name, cmd):
        return run(['incus', 'exec', '-T', name, '--project', project, '--', *cmd])
    inside = lambda cmd: in_instance(instance, cmd)
    peer = lambda cmd: in_instance(config['edge_instance'], cmd)
    paths = {
        '/etc/nftables.d/shippin-bastion.nft': 'shippin-bastion.nft',
        '/etc/systemd/system/shippin-bastion-guard.service': 'shippin-bastion-guard.service',
        '/etc/systemd/system/ssh.service.d/10-shippin-bastion-guard.conf': '10-shippin-bastion-guard.conf',
        '/etc/systemd/system/bastion-web.service.d/10-shippin-bastion-guard.conf': '10-shippin-bastion-guard.conf',
    }
    contents = {target: bundle[name].decode() for target, name in paths.items()}
    expected_rules = bundle['shippin-bastion.nft'].decode()
    def files(action, data):
        inside(['python3', '-I', '-c', bundle['guard_files.py'].decode(), action, json.dumps(data)])
    marker = args.backup / 'prepared.json'
    protected(args.backup.parent, directory=True)
    if args.action in ('check', 'apply'):
        require(not args.backup.exists() and not args.backup.is_symlink(), 'backup already exists')
        files('check', contents)
        tables = json.loads(inside(['nft', '-j', 'list', 'tables']))
        require(not any(x.get('table', {}).get('name') == 'shippin_bastion_guard'
                        for x in tables['nftables']), 'guard table already exists')
        inside(['python3', '-I', '-c',
                "import subprocess,sys; subprocess.run(['nft','--check','--file','-'],input=sys.argv[1],text=True,check=True)",
                expected_rules])
        inventory(inside, peer, config, guard)
        print('PASS: protected absent paths, kernel syntax, interfaces/IP/route/listeners/loader')
        if args.action == 'check':
            return
        args.backup.mkdir(mode=0o700)
        sync_dir(args.backup.parent)
        saved = {'state': 'prepared', 'files': contents, 'instance': instance, 'project': project,
                 'bundle_digest': digest, 'nft_before': inside(['nft', 'list', 'ruleset'])}
        atomic_json(marker, saved)  # durable intent before the first remote mutation
        files('install', contents)
        inside(['systemctl', 'daemon-reload'])
        inside(['systemctl', 'enable', '--now', 'shippin-bastion-guard.service'])
        rules(inside, expected_rules)
        services(inside)
        saved['state'] = 'applied'
        atomic_json(marker, saved)
        print('APPLIED: pending ticket verification; not policy confirmation')
    elif args.action == 'rollback':
        protected(marker)
        saved = json.loads(marker.read_text())
        require(saved['instance'] == instance and saved['project'] == project and
                saved['bundle_digest'] == digest and saved['files'] == contents, 'journal identity mismatch')
        if saved['state'] == 'rolled_back':
            print('Already rolled back')
            return
        files('validate', saved['files'])
        def phase(name, operation):
            saved.update(state='rolling_back', rollback_phase=name)
            atomic_json(marker, saved)
            operation()
            saved['completed_phase'] = name
            atomic_json(marker, saved)
        failure = None
        try:
            phase('remove_dependencies', lambda: files('remove', {
                k: v for k, v in saved['files'].items() if k.endswith('.conf')}))
            phase('reload_dependencies', lambda: inside(['systemctl', 'daemon-reload']))
            phase('disable_guard', lambda: inside(['python3', '-I', '-c',
                "import pathlib,subprocess; p=pathlib.Path('/etc/systemd/system/shippin-bastion-guard.service'); subprocess.run(['systemctl','disable','--now','shippin-bastion-guard.service'],check=True) if p.exists() else None"]))
            phase('remove_files', lambda: files('remove', saved['files']))
            phase('reload_units', lambda: inside(['systemctl', 'daemon-reload']))
            phase('remove_table', lambda: inside(['nft', 'destroy', 'table', 'inet', 'shippin_bastion_guard']))
        except BaseException as error:
            failure = error
        finally:
            # SIGKILL/power loss still requires the armed Gold retry/boot-revert path.
            for unit in ['ssh.service', 'bastion-web.service']:
                try:
                    inside(['systemctl', 'start', unit])
                except Exception as error:
                    print(f'RECOVERY REQUIRED: could not restore {unit}: {error}')
                    if failure is None:
                        failure = error
        if failure is not None:
            raise failure
        saved['state'] = 'rolled_back'
        atomic_json(marker, saved)
        print('ROLLED BACK: owned files/temporary files/table removed; original services running')
    else:
        inventory(inside, peer, config, guard)
        rules(inside, expected_rules)
        services(inside)
        target = f"http://{config['bastion_ipv4']}:{guard['terminal_port']}/"
        def probe(source):
            result = subprocess.run(['incus', 'exec', '-T', source, '--project', project, '--',
                'curl', '-sS', '--max-time', '3', '-o', '/dev/null', '-w', '%{http_code}', target],
                stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=8)
            return result.returncode, result.stdout
        require(probe(config['edge_instance']) == (0, '200'), 'edge cannot reach terminal')
        for source in config['deny_probe_instances']:
            rc, status = probe(source)
            require(rc != 0 and status == '000', f'non-edge {source} reached terminal')
        print('PASS: exact rules, services, inventory, edge allowed/non-edge denied; SSH proof still required')


if __name__ == '__main__':
    if 'BUNDLE' not in globals():
        raise SystemExit('Use the approved inline launcher; direct execution is prohibited')
    main(BUNDLE, BUNDLE_DIGEST)
