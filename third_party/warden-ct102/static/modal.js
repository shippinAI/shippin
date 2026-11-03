// Warden Confirm Modal — replaces browser confirm() dialogs
var Modal = (function() {
    var overlay = null;

    function init() {
        if (overlay) return;
        overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        overlay.innerHTML =
            '<div class="modal-box">' +
                '<div class="modal-body"></div>' +
                '<div class="modal-btns">' +
                    '<button class="btn btn-dim modal-cancel">Cancel</button>' +
                    '<button class="btn modal-ok">Confirm</button>' +
                '</div>' +
            '</div>';
        document.body.appendChild(overlay);

        overlay.addEventListener('click', function(e) {
            if (e.target === overlay) resolve(false);
        });
        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape' && overlay.classList.contains('modal-open')) {
                resolve(false);
            }
        });
    }

    var _resolve = null;

    function resolve(val) {
        if (!_resolve) return;
        overlay.classList.remove('modal-open');
        var fn = _resolve;
        _resolve = null;
        fn(val);
    }

    /**
     * Modal.confirm(message, opts)
     * opts.danger  — use red confirm button (default false)
     * opts.ok      — confirm button text (default "Confirm")
     * opts.cancel  — cancel button text (default "Cancel")
     * Returns a Promise<boolean>
     */
    function confirm(message, opts) {
        init();
        opts = opts || {};
        var body = overlay.querySelector('.modal-body');
        var okBtn = overlay.querySelector('.modal-ok');
        var cancelBtn = overlay.querySelector('.modal-cancel');

        body.textContent = message;
        okBtn.textContent = opts.ok || 'Confirm';
        cancelBtn.textContent = opts.cancel || 'Cancel';

        okBtn.className = 'btn modal-ok ' + (opts.danger ? 'btn-danger' : 'btn-default');

        overlay.querySelector('.modal-cancel').onclick = function() { resolve(false); };
        overlay.querySelector('.modal-ok').onclick = function() { resolve(true); };

        overlay.classList.add('modal-open');
        okBtn.focus();

        return new Promise(function(res) { _resolve = res; });
    }

    /**
     * Modal.confirm with two choices (e.g. delete user + optionally remove home)
     * Returns Promise<{confirmed: bool, option: bool}>
     */
    function confirmWithOption(message, optionLabel) {
        init();
        var body = overlay.querySelector('.modal-body');
        var okBtn = overlay.querySelector('.modal-ok');
        var cancelBtn = overlay.querySelector('.modal-cancel');

        var id = 'modal-opt-' + Date.now();
        body.innerHTML = '<div style="margin-bottom:0.75rem">' + escHtml(message) + '</div>' +
            '<label class="modal-option"><input type="checkbox" id="' + id + '"> ' + escHtml(optionLabel) + '</label>';
        okBtn.textContent = 'Confirm';
        okBtn.className = 'btn modal-ok btn-danger';
        cancelBtn.textContent = 'Cancel';

        overlay.querySelector('.modal-cancel').onclick = function() { resolveOpt(false, false); };
        overlay.querySelector('.modal-ok').onclick = function() {
            var chk = document.getElementById(id);
            resolveOpt(true, chk ? chk.checked : false);
        };

        overlay.classList.add('modal-open');
        okBtn.focus();

        return new Promise(function(res) { _resolveOpt = res; });
    }

    var _resolveOpt = null;

    function resolveOpt(confirmed, option) {
        if (!_resolveOpt) return;
        overlay.classList.remove('modal-open');
        var fn = _resolveOpt;
        _resolveOpt = null;
        fn({ confirmed: confirmed, option: option });
    }

    function escHtml(s) {
        var d = document.createElement('div');
        d.textContent = s;
        return d.innerHTML;
    }

    // Intercept form submissions with data-confirm attribute
    document.addEventListener('submit', function(e) {
        var form = e.target;
        var msg = form.getAttribute('data-confirm');
        if (!msg) return;
        if (form.dataset.confirmed === 'true') {
            return; // allow through to toast.js
        }
        e.preventDefault();
        e.stopImmediatePropagation();
        var danger = form.hasAttribute('data-confirm-danger');
        confirm(msg, { danger: danger }).then(function(ok) {
            if (ok) {
                form.dataset.confirmed = 'true';
                form.requestSubmit ? form.requestSubmit() : form.submit();
            }
        });
    }, true);

    return { confirm: confirm, confirmWithOption: confirmWithOption };
})();
