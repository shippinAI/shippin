# Warden Backlog

## Priority 1 — High Value, Moderate Effort

- [x] **Session Timeout & Security** — Configurable session expiry, idle timeout, brute-force rate limiting on login, failed login logging
- [x] **Notifications / Toast Messages** — Replace full page reloads with AJAX + toast notifications for actions (service restart, firewall rule add, etc.)
- [x] **Dashboard Customisation** — Collapsible/reorderable dashboard cards, remember layout in localStorage
- [ ] **Responsive Mobile Layout** — Hamburger menu, horizontal scroll on tables, improved small screen experience
- [ ] **Scheduled Tasks (systemd timers)** — List and manage systemd timers alongside cron jobs

## Priority 2 — Strong Features, More Effort

- [x] **DNS Management** — View/edit `/etc/hosts`, `/etc/resolv.conf`, manage local DNS entries
- [ ] **Network Configuration** — View/edit network interfaces, IP addresses, routes, DNS settings. Manage NetworkManager or netplan configs
- [ ] **Process Manager** — Full process list with search, kill/signal processes, sort by CPU/memory (expanded from dashboard's top 15)
- [x] **Backup & Restore** — Full estate backup tab for warden-storage and /home/wardenop, plus tar/rsync jobs and one-click restore
- [ ] **SSL / Let's Encrypt** — Built-in ACME client for automatic Let's Encrypt certificates, auto-renewal
- [ ] **Disk & Partition Manager** — View partitions, mount/unmount filesystems, view SMART status, manage fstab

## Priority 3 — Nice to Have

- [ ] **Multi-server Support** — Agent mode: manage multiple servers from one Warden instance
- [ ] **Audit Log** — Log all actions taken through Warden (who did what, when) to a local file
- [ ] **API Keys / Token Auth** — REST API authentication for scripting and automation
- [ ] **SSH Key Management** — View/add/remove authorised SSH keys per user
- [ ] **Config File Editor** — Quick access to common config files (nginx, apache, sshd, etc.) with syntax hints
- [ ] **Database Overview** — Detect MySQL/PostgreSQL/Redis, show status, connections, basic stats
- [ ] **Email / Webhook Alerts** — Trigger alerts on high CPU, disk full, service down, etc.
- [ ] **Internationalisation (i18n)** — Language support for non-English users
- [ ] **Plugin System** — Allow extending Warden with custom modules

## Quick Wins

- [x] **Favicon** — Add a proper favicon instead of browser default
- [x] **Search across nav** — Quick-jump to any page via keyboard shortcut (Ctrl+K)
- [ ] **Export tables to CSV** — Download button on services, packages, users, firewall tables
- [x] **Confirm dangerous actions** — Consistent modal confirmations instead of browser `confirm()` dialogs
- [ ] **Connection indicator** — Show when the server is unreachable (dashboard auto-refresh failure)
