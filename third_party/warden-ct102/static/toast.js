/* Toast notification system & AJAX form handler */
var Toast = (function() {
    var container;

    function init() {
        if (container) return;
        container = document.createElement('div');
        container.className = 'toast-container';
        document.body.appendChild(container);
    }

    function show(message, type, duration) {
        init();
        type = type || 'info';
        duration = duration || 4000;

        var toast = document.createElement('div');
        toast.className = 'toast toast-' + type;

        var icon = type === 'ok' ? '\u2713' : type === 'err' ? '\u2717' : '\u24d8';
        toast.innerHTML = '<span class="toast-icon">' + icon + '</span>' +
            '<span class="toast-msg">' + escapeHtml(message) + '</span>' +
            '<button class="toast-close">\u00d7</button>';

        toast.querySelector('.toast-close').onclick = function() { dismiss(toast); };

        container.appendChild(toast);
        // Trigger reflow for animation
        toast.offsetHeight;
        toast.classList.add('toast-in');

        if (duration > 0) {
            setTimeout(function() { dismiss(toast); }, duration);
        }
        return toast;
    }

    function dismiss(toast) {
        if (toast.classList.contains('toast-out')) return;
        toast.classList.remove('toast-in');
        toast.classList.add('toast-out');
        setTimeout(function() {
            if (toast.parentNode) toast.parentNode.removeChild(toast);
        }, 300);
    }

    function escapeHtml(s) {
        var d = document.createElement('div');
        d.appendChild(document.createTextNode(s));
        return d.innerHTML;
    }

    return { show: show, init: init };
})();

/* Show any ?msg= query param as a toast on page load */
(function() {
    var params = new URLSearchParams(window.location.search);
    var msg = params.get('msg');
    if (msg) {
        var type = msg.indexOf('success') !== -1 || msg.indexOf('initiated') !== -1 ? 'ok' : 'err';
        Toast.show(msg, type);
        // Clean URL without reload
        var url = new URL(window.location);
        url.searchParams.delete('msg');
        history.replaceState(null, '', url.toString());
    }
})();

/* AJAX form submission — intercepts all POST forms to API endpoints */
(function() {
    document.addEventListener('submit', function(e) {
        var form = e.target;
        if (form.method.toLowerCase() !== 'post') return;

        // Let modal.js handle unconfirmed data-confirm forms
        if (form.hasAttribute('data-confirm') && form.dataset.confirmed !== 'true') return;
        form.dataset.confirmed = '';

        var action = form.getAttribute('action') || '';

        // Only intercept API endpoints and sudo toggle
        if (action.indexOf('/api/') === -1 && action.indexOf('/sudo/toggle') === -1) return;

        // Skip forms that have their own AJAX handlers (packages progress bar)
        if (action.indexOf('/api/packages/action') !== -1) return;

        // Skip file uploads (multipart forms)
        if (form.enctype === 'multipart/form-data') return;

        e.preventDefault();

        // Prevent double-submit
        if (form.dataset.submitting === 'true') return;
        form.dataset.submitting = 'true';

        var btn = form.querySelector('button[type="submit"]');
        if (btn) {
            btn.disabled = true;
            btn.dataset.origText = btn.textContent;
            btn.innerHTML = '<span class="btn-spinner"></span>';
        }

        var data = new FormData(form);
        var body = new URLSearchParams(data).toString();

        fetch(action, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded',
                'Accept': 'application/json'
            },
            body: body
        }).then(function(resp) {
            if (resp.redirected) {
                // For sudo toggle or other redirects, reload the page
                window.location.reload();
                return null;
            }
            return resp.json();
        }).then(function(result) {
            if (!result) return;
            Toast.show(result.message, result.success ? 'ok' : 'err');
            if (action.indexOf('/sudo/toggle') !== -1 && result.success) {
                updateSudoUI(result.enabled);
            } else if (result.success && action.indexOf('/api/services/action') !== -1) {
                refreshServices();
            } else if (result.success && action.indexOf('/api/docker/action') !== -1) {
                refreshDocker();
            } else if (result.success && action.indexOf('/api/packages/action') !== -1) {
                setTimeout(function() { window.location.reload(); }, 1500);
            } else if (result.success && action.indexOf('/api/system/action') !== -1) {
                // System actions — no refresh needed
            } else if (result.success && action.indexOf('/api/files') !== -1) {
                refreshFiles();
            }
        }).catch(function(err) {
            Toast.show('Request failed: ' + err.message, 'err');
        }).finally(function() {
            form.dataset.submitting = '';
            if (btn) {
                btn.disabled = false;
                btn.textContent = btn.dataset.origText;
            }
        });
    });

    /* Sudo UI update — toggle banner and checkbox without reload */
    function updateSudoUI(enabled) {
        var banner = document.querySelector('.sudo-banner');
        if (enabled && !banner) {
            banner = document.createElement('div');
            banner.className = 'sudo-banner';
            banner.textContent = 'Elevated privileges active \u2014 commands will run with sudo';
            var main = document.querySelector('main');
            if (main) main.parentNode.insertBefore(banner, main);
        } else if (!enabled && banner) {
            banner.parentNode.removeChild(banner);
        }
        var cb = document.querySelector('.sudo-toggle input[type="checkbox"]');
        if (cb) cb.checked = enabled;
    }

    /* Inline refresh helpers — reload just the data, not the whole page */

    function refreshServices() {
        var filter = '';
        var filterInput = document.querySelector('.filter-input');
        if (filterInput) filter = filterInput.value;
        fetch('/api/services?filter=' + encodeURIComponent(filter), {
            headers: { 'Accept': 'application/json' }
        }).then(function(r) { return r.json(); })
        .then(function(units) { updateServicesTable(units); })
        .catch(function() { /* Silently fail — toast already shown */ });
    }

    function updateServicesTable(units) {
        var tbody = document.querySelector('table tbody');
        if (!tbody) return;
        tbody.innerHTML = '';
        units.forEach(function(u) {
            var tr = document.createElement('tr');
            tr.setAttribute('data-unit', u.name);
            tr.setAttribute('data-running', u.running ? 'true' : 'false');
            tr.setAttribute('data-enabled', u.enabled);
            var stateClass = u.running ? 'state-up' : 'state-down';
            var runText = u.running ? 'Yes' : 'No';
            var runClass = u.running ? 'running-yes' : 'running-no';

            var btns = '';
            if (u.running) {
                btns += actionForm('/api/services/action', u.name, 'restart', 'Restart', 'btn-warn', 'Restart ' + escHtml(u.name) + '?');
                btns += actionForm('/api/services/action', u.name, 'stop', 'Stop', 'btn-danger', 'Stop ' + escHtml(u.name) + '?');
            } else {
                btns += actionForm('/api/services/action', u.name, 'start', 'Start', 'btn-ok', '');
            }
            if (u.enabled === 'enabled' || u.enabled === 'enabled-runtime') {
                btns += actionForm('/api/services/action', u.name, 'disable', 'Disable', 'btn-dim', 'Disable ' + escHtml(u.name) + ' at boot?');
            } else if (u.enabled !== 'static') {
                btns += actionForm('/api/services/action', u.name, 'enable', 'Enable', 'btn-default', '');
            }

            tr.innerHTML = '<td><strong>' + escHtml(u.name) + '</strong></td>' +
                '<td>' + escHtml(u.description) + '</td>' +
                '<td><span class="' + stateClass + '">' + escHtml(u.active) + '</span></td>' +
                '<td>' + escHtml(u.enabled) + '</td>' +
                '<td><span class="' + runClass + '">' + runText + '</span></td>' +
                '<td class="actions-cell"><div class="action-btns">' + btns + '</div></td>';
            tbody.appendChild(tr);
        });
    }

    function actionForm(url, unit, action, label, cls, confirmMsg) {
        var danger = cls.indexOf('btn-danger') !== -1;
        return '<form method="post" action="' + url + '" class="inline-form"' +
            (confirmMsg ? ' data-confirm="' + confirmMsg + '"' + (danger ? ' data-confirm-danger' : '') : '') + '>' +
            '<input type="hidden" name="unit" value="' + escHtml(unit) + '">' +
            '<input type="hidden" name="action" value="' + action + '">' +
            '<button type="submit" class="btn btn-sm ' + cls + '">' + label + '</button></form>';
    }

    function refreshDocker() {
        // Docker has multiple tabs with different data — simplest to reload
        setTimeout(function() { window.location.reload(); }, 1000);
    }

    function refreshFiles() {
        // File manager — reload to reflect changes
        setTimeout(function() { window.location.reload(); }, 500);
    }

    function escHtml(s) {
        var d = document.createElement('div');
        d.appendChild(document.createTextNode(s));
        return d.innerHTML;
    }
})();
