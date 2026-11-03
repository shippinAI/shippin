#!/usr/bin/python3
"""TEST ONLY: transport adapter for an isolated systemd container; never install on Gold."""
import os
import json
from pathlib import Path
import subprocess
import sys

args = sys.argv[1:]
if args[:2] != ['exec', '-T'] or args[3:6] != ['--project', 'fixture', '--']:
    raise SystemExit('unexpected test transport command')
instance, command = args[2], args[6:]
if instance not in ('bastion', 'edge', 'console', 'mcp'):
    raise SystemExit('unknown fixture instance')
if instance != 'bastion':
    if instance in ('console', 'mcp') and command[0] == 'curl':
        command += ['--interface', '192.0.2.50' if instance == 'console' else '192.0.2.90']
    pid = Path('/run/guard-test-peer').read_text().strip()
    command = ['nsenter', f'--net=/proc/{pid}/ns/net', *command]
result = subprocess.run(command)
label = None
if args[6:8] == ['systemctl', 'daemon-reload']:
    counter = Path('/run/guard-test-reload-count')
    count = int(counter.read_text()) + 1 if counter.exists() else 1
    counter.write_text(str(count))
    label = 'daemon-reload-' + str(count)
elif args[6:8] == ['systemctl', 'enable']:
    label = 'enable'
elif len(args) > 10 and args[6:9] == ['python3', '-I', '-c'] and args[10] == 'install':
    label = 'files-installed'
elif len(args) > 10 and args[6:9] == ['python3', '-I', '-c'] and args[10] == 'remove':
    label = 'dropins-removed' if all(k.endswith('.conf') for k in json.loads(args[11])) else 'files-removed'
elif len(args) == 10 and args[6:9] == ['python3', '-I', '-c'] and "'disable','--now'" in args[9]:
    label = 'guard-disabled'
elif args[6:9] == ['nft', 'destroy', 'table']:
    label = 'table-removed'
if label and os.environ.get('TEST_KILL_AFTER') == label:
    os.kill(os.getppid(), 9)  # true controller death, so its finally block cannot run
if label and os.environ.get('TEST_FAIL_AFTER') == label:
    raise SystemExit(99)  # result is lost after the real side effect
raise SystemExit(result.returncode)
