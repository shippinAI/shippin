/* Quick-jump navigation palette (Ctrl+K) */
var QuickNav = (function() {
    var pages = [
        { name: 'Overview / Home', url: '/', keys: 'home control panel overview admin shell' },
        { name: 'Overview / Dashboard', url: '/dashboard', keys: 'dashboard stats overview gauges wardenclyffe infra internal customer connections' },
        { name: 'Overview / Agent Workspaces', url: '/agent-workspaces', keys: 'agent workspaces codex claude devstation capsule vscode cursor shared storage ssh stream tmux' },
        { name: 'Overview / Workspace Map', url: '/workspaces', keys: 'workspace map federation node wardenclyffe devpulse master-node mcp projects connected components' },
        { name: 'Deploy / Catalog', url: '/catalog', keys: 'catalog deployable blueprints template schema x-warden compose' },
        { name: 'Deploy / Deployments', url: '/deployments', keys: 'deployments active pending rollouts backups recovery drills migrations substrate cluster promotion automation' },
        { name: 'Deploy / Customers', url: '/customers', keys: 'customers tenant customer owned systems sla recoveries clyffe boundary' },
        { name: 'Deploy / Warden Internal', url: '/internal', keys: 'warden internal platform control plane shared services ops boundary' },
        { name: 'Operations / Services', url: '/services', keys: 'services systemd units' },
        { name: 'Operations / Packages', url: '/packages', keys: 'packages apt dnf pacman updates' },
        { name: 'Operations / Docker', url: '/docker', keys: 'docker containers images volumes' },
        { name: 'Operations / Files', url: '/files', keys: 'files filemanager browser editor' },
        { name: 'Observability / Logs', url: '/logs', keys: 'logs journal syslog' },
        { name: 'Operations / Cron', url: '/cron', keys: 'cron jobs schedule crontab' },
        { name: 'Operations / Backups', url: '/backups', keys: 'backups backup restore tar rsync warden-storage wardenop shippin estate full' },
        { name: 'Access / Users', url: '/users', keys: 'users groups accounts' },
        { name: 'Access / Firewall', url: '/firewall', keys: 'firewall ufw rules ports' },
        { name: 'Access / SSH Keys', url: '/sshkeys', keys: 'ssh keys authorized authorised pubkey' },
        { name: 'Infrastructure / DNS', url: '/dns', keys: 'dns hosts resolv nameserver resolver domain domains cloudflare powerdns caddy zones routes edge' },
        { name: 'Observability / Endpoints', url: '/endpoints', keys: 'endpoints monitor checks uptime port health' },
        { name: 'Settings / Terminal', url: '/terminal', keys: 'terminal shell console bash expert' },
        { name: 'Settings / System Info', url: '/sysinfo', keys: 'sysinfo neofetch hostname os kernel cpu ram memory specs hardware' },
        { name: 'Settings / Config', url: '/config', keys: 'config settings export import backup migrate' },
        { name: 'Settings / System', url: '/system', keys: 'system reboot shutdown poweroff' },
        { name: 'Infrastructure / Proxmox', url: '/proxmox', keys: 'proxmox pve vm container kvm lxc hypervisor infrastructure' },
        { name: 'Infrastructure / Runners', url: '/runners', keys: 'runners ci fleet build github actions nightly streak race achievements devpulse' },
        { name: 'Settings / Help', url: '/help', keys: 'help about info' }
    ];

    var overlay, input, list, selected = 0, filtered = pages;

    function build() {
        overlay = document.createElement('div');
        overlay.className = 'qn-overlay';
        overlay.innerHTML =
            '<div class="qn-modal">' +
                '<div class="qn-input-wrap">' +
                    '<span class="qn-icon">\u2315</span>' +
                    '<input class="qn-input" type="text" placeholder="Jump to page\u2026" autocomplete="off" />' +
                    '<kbd class="qn-kbd">esc</kbd>' +
                '</div>' +
                '<ul class="qn-list"></ul>' +
            '</div>';
        document.body.appendChild(overlay);
        input = overlay.querySelector('.qn-input');
        list = overlay.querySelector('.qn-list');

        overlay.addEventListener('mousedown', function(e) {
            if (e.target === overlay) close();
        });

        input.addEventListener('input', function() {
            filter(input.value);
        });

        input.addEventListener('keydown', function(e) {
            if (e.key === 'ArrowDown') {
                e.preventDefault();
                selected = Math.min(selected + 1, filtered.length - 1);
                updateActive();
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                selected = Math.max(selected - 1, 0);
                updateActive();
            } else if (e.key === 'Enter') {
                e.preventDefault();
                if (filtered[selected]) {
                    window.location.href = filtered[selected].url;
                }
            } else if (e.key === 'Escape') {
                close();
            }
        });
    }

    function filter(query) {
        var q = query.toLowerCase().trim();
        if (!q) {
            filtered = pages;
        } else {
            filtered = pages.filter(function(p) {
                return p.name.toLowerCase().indexOf(q) !== -1 ||
                       p.keys.indexOf(q) !== -1;
            });
        }
        selected = 0;
        render();
    }

    function render() {
        list.innerHTML = '';
        if (filtered.length === 0) {
            list.innerHTML = '<li class="qn-empty">No matches</li>';
            return;
        }
        filtered.forEach(function(p, i) {
            var li = document.createElement('li');
            li.className = 'qn-item' + (i === selected ? ' qn-active' : '');
            var current = window.location.pathname === p.url ||
                (p.url !== '/' && window.location.pathname.indexOf(p.url) === 0);
            li.innerHTML = '<span class="qn-name">' + p.name + '</span>' +
                (current ? '<span class="qn-current">current</span>' : '') +
                '<span class="qn-url">' + p.url + '</span>';
            li.addEventListener('mousedown', function(e) {
                e.preventDefault();
                window.location.href = p.url;
            });
            li.addEventListener('mouseenter', function() {
                selected = i;
                updateActive();
            });
            list.appendChild(li);
        });
    }

    function updateActive() {
        var items = list.querySelectorAll('.qn-item');
        items.forEach(function(el, i) {
            if (i === selected) {
                el.classList.add('qn-active');
                el.scrollIntoView({ block: 'nearest' });
            } else {
                el.classList.remove('qn-active');
            }
        });
    }

    function open() {
        if (!overlay) build();
        overlay.classList.add('qn-open');
        input.value = '';
        filtered = pages;
        selected = 0;
        render();
        setTimeout(function() { input.focus(); }, 10);
    }

    function close() {
        if (overlay) overlay.classList.remove('qn-open');
    }

    function isOpen() {
        return overlay && overlay.classList.contains('qn-open');
    }

    document.addEventListener('keydown', function(e) {
        if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
            e.preventDefault();
            if (isOpen()) {
                close();
            } else {
                open();
            }
        }
    });

    return { isOpen: isOpen };
})();
