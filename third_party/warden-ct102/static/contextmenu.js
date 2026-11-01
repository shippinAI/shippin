// Warden Context Menu
// Usage: ctxMenu.attach(selector, itemsCallback)
// itemsCallback(targetRow, event) should return [{label, action, icon?, danger?, disabled?}, 'separator', ...]

var ctxMenu = (function() {
    var menuEl = null;

    function init() {
        if (menuEl) return;
        menuEl = document.createElement('div');
        menuEl.className = 'ctx-menu';
        menuEl.style.display = 'none';
        document.body.appendChild(menuEl);

        document.addEventListener('click', function() { hide(); });
        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') hide();
        });
        window.addEventListener('scroll', function() { hide(); });
        window.addEventListener('resize', function() { hide(); });
    }

    function show(x, y, items) {
        init();
        menuEl.innerHTML = '';

        items.forEach(function(item) {
            if (item === 'separator') {
                var sep = document.createElement('div');
                sep.className = 'ctx-sep';
                menuEl.appendChild(sep);
                return;
            }
            var btn = document.createElement('button');
            btn.className = 'ctx-item';
            if (item.danger) btn.classList.add('ctx-danger');
            if (item.disabled) btn.classList.add('ctx-disabled');

            var html = '';
            if (item.icon) html += '<span class="ctx-icon">' + item.icon + '</span>';
            html += '<span>' + item.label + '</span>';
            btn.innerHTML = html;

            if (!item.disabled) {
                btn.addEventListener('click', function(e) {
                    e.stopPropagation();
                    hide();
                    item.action();
                });
            }
            menuEl.appendChild(btn);
        });

        // Position
        menuEl.style.display = '';
        var rect = menuEl.getBoundingClientRect();
        var vw = window.innerWidth;
        var vh = window.innerHeight;

        if (x + rect.width > vw) x = vw - rect.width - 8;
        if (y + rect.height > vh) y = vh - rect.height - 8;
        if (x < 0) x = 8;
        if (y < 0) y = 8;

        menuEl.style.left = x + 'px';
        menuEl.style.top = y + 'px';
    }

    function hide() {
        if (menuEl) menuEl.style.display = 'none';
    }

    function attach(selector, itemsCallback) {
        document.addEventListener('contextmenu', function(e) {
            var target = e.target.closest(selector);
            if (!target) return;
            e.preventDefault();
            var items = itemsCallback(target, e);
            if (items && items.length > 0) {
                show(e.clientX, e.clientY, items);
            }
        });
    }

    return { show: show, hide: hide, attach: attach };
})();
