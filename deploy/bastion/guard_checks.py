"""Inventory and loaded-rule checks; authorization and SSH-session proof remain ticket gates."""
import json
import re


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def inventory(inside, peer, runtime, guard):
    links = {v['ifname']: v for v in json.loads(inside(['ip', '-j', 'link', 'show']))}
    for name in [guard['edge_interface'], *guard['ssh_interfaces']]:
        require(name in links and 'UP' in links[name]['flags'], f'interface missing/down: {name}')
    def addresses(executor, interface):
        rows = json.loads(executor(['ip', '-j', 'address', 'show', 'dev', interface]))
        return [a['local'] for row in rows for a in row['addr_info']]
    require(runtime['bastion_ipv4'] in addresses(inside, guard['edge_interface']), 'wrong bastion IP')
    require(guard['edge_ipv4'] in addresses(peer, guard['edge_interface']), 'wrong edge IP')
    route = json.loads(inside(['ip', '-j', 'route', 'get', guard['edge_ipv4']]))
    require(len(route) == 1 and route[0]['dev'] == guard['edge_interface'] and
            route[0].get('prefsrc') == runtime['bastion_ipv4'], 'unexpected edge route')
    loader = inside(['systemctl', 'show', 'nftables.service', '-p', 'ActiveState', '-p', 'UnitFileState'])
    require('ActiveState=inactive' in loader and 'UnitFileState=disabled' in loader,
            'another nftables loader is active/enabled; coordinate it before this change')
    sockets = inside(['ss', '-H', '-ltnp'])
    for unit, port in [('ssh.service', guard['ssh_port']), ('bastion-web.service', guard['terminal_port'])]:
        inside(['systemctl', 'is-active', '--quiet', unit])
        pid = inside(['systemctl', 'show', unit, '-p', 'MainPID', '--value']).strip()
        require(pid.isdecimal() and int(pid) > 0, f'no main PID: {unit}')
        exe = inside(['readlink', '-f', f'/proc/{pid}/exe']).strip()
        require(exe == runtime['listener_executables'][unit], f'unexpected executable: {unit}')
        rows = [row for row in sockets.splitlines() if row.split()[3].endswith(':' + str(port))]
        require(bool(rows) and all(re.search(r'pid=' + pid + r'[,)]', row) for row in rows),
                f'listener ownership mismatch: {unit}')


def rules(inside, expected):
    actual = inside(['nft', '-nn', '-y', '-s', 'list', 'table', 'inet', 'shippin_bastion_guard'])
    desired = '\n'.join(line for line in expected.splitlines()
                        if not line.startswith(('#', 'destroy ')))
    normalize = lambda value: re.sub(r'\s+', '', value)
    require(normalize(actual) == normalize(desired), 'loaded rules differ from approved rules')


def services(inside):
    inside(['systemctl', 'is-enabled', '--quiet', 'shippin-bastion-guard.service'])
    inside(['systemctl', 'is-active', '--quiet', 'shippin-bastion-guard.service'])
    for unit in ['ssh.service', 'bastion-web.service']:
        inside(['systemctl', 'is-active', '--quiet', unit])
        deps = inside(['systemctl', 'show', unit, '-p', 'Requires', '--value'])
        require('shippin-bastion-guard.service' in deps.split(), f'missing dependency: {unit}')
