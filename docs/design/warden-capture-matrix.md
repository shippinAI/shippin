# Warden capture matrix

The console is the product. The tree in `third_party/warden-ct102` is an import of the older panel (2026-08-27), kept so this matrix can name every file. It is not a subsystem: its HTML, PAM login, env file, and YAML registries are not loaded. Each row says where the job lives in the console and how it was changed to fit.

Naming. Packages, routes, UI strings, and config keys say Shippin. The panel's product name does not appear in those places. Provenance is this matrix and the import commit.

Inventory count: 188 tracked files plus 28 screenshot rows absent from the current
tree (216 imported-source matrix rows), and five named source documents reviewed
separately from `shippin-mesh`. These counts describe source rows, not product
modules or deployable capabilities.

| Status | Count |
|---|---|
| ported | 6 |
| to-author | 1 |
| intentionally replaced | 177 |
| rejected-for-security | 32 |
| needs-review (additional named sources) | 0 |

`ported` means behavior was re-authored in a platform owner and has implementation evidence. `intentionally replaced` means a current platform capability owns the same concern; its API and behavior still need verification. `to-author` means the row names a target owner but a supported route is not complete. `rejected-for-security` means this imported implementation is excluded; a related product capability can remain open in the backlog for safe re-authoring. `needs-review` means the source is now available but a feature-level comparison and disposition are incomplete.

The reference tree is not served and is not a second panel. It can be removed only after every `to-author` and `needs-review` row is implemented or explicitly rejected and the import provenance is retained. Do not use the row count as proof that every behavior has been exercised.

## How alignment was applied

- Identity is Nomen (`internal/auth`). There is no second user table, PAM stack, or PVE user directory.
- A hypervisor API is columns on `servers` (migration `036_server_hypervisor.sql`), the same rows as enroll and hostd. There is no second registry and no `PROXMOX_*` config.
- The role is `roles.infra` (`op_infra`). Platform admin is not implied.
- Infrastructure reads and configuration remain available, but provider mutations fail closed with `503 agenthub_review_unavailable` until AgentHub integration is implemented. The local task board does not authorize provider changes.
- Secrets are env names or `secret_ref`. Token bytes are not columns.
- UI is the console shell: `PageHeader`, `Card`, `DataTable`, `EmptyState`, operator nav. No panel template is rendered.
- TLS verification stays on.

## Backlog items

From `BACKLOG.md` in the import. Checked items the console already covers are replaced. Open items are to-author or rejected.

| Item | Status | Console home | Alignment |
|---|---|---|---|
| Session timeout, idle, login rate limit | intentionally replaced | Nomen session | Not a second session store. |
| Toast notifications | intentionally replaced | console toast queue | Panel AJAX toasts are not loaded. |
| Dashboard card layout | intentionally replaced | operator health and Hosts | No second dashboard. |
| Responsive layout | intentionally replaced | console shell, including the phone nav | The panel hamburger is not ported. Hosts is on the phone nav when the caller has `infra.hosts`. |
| systemd timers next to cron | to-author | internal/jobs | Host timer editing is not the job runner. |
| DNS management | intentionally replaced | workspace domains; phase 2 node DNS | `/etc/hosts` editing is not ported. |
| Network configuration | ported (read) | phase 1 node network read | Writes are phase 3, not half-routed. |
| Process manager | rejected-for-security | none | Killing host processes from the panel is not a product feature. |
| Backup and restore | to-author | managed-services backups; phase 1 vzdump and PBS list/restore | File copy executors are not wired. Hypervisor backups are phase 1. |
| ACME | to-author | phase 3 node ACME | Not routed. |
| Disk and partition manager | to-author | phase 2 node disks | Local fstab edits are not ported. |
| Multi-server | intentionally replaced | servers registry | One registry, not an agent mode beside it. |
| Audit log | intentionally replaced | console audit | The panel file log is not written. |
| API tokens for the panel | rejected-for-security | console bearer session | A second token system for the panel API is not added. PVE tokens stay the hypervisor credential, named by env. |
| SSH key management | intentionally replaced | `/account/ssh-keys` | Host `authorized_keys` editing is rejected. |
| Config file editor | rejected-for-security | none | Arbitrary host config writes are the file manager problem. |
| Database overview | intentionally replaced | tenant database page | Not a probe of local MySQL sockets. |
| Email and webhook alerts | intentionally replaced | internal/mail; phase 3 PVE notifications | Panel webhooks are not copied. |
| Internationalisation | to-author | console copy, when the product takes it | No panel string catalog is imported. |
| Plugin system | rejected-for-security | none | Loading panel plugins is a second extension surface. |
| Favicon | intentionally replaced | console brand | The panel icon is not served. |
| Quicknav (Ctrl+K) | intentionally replaced | sidebar and phone nav | `static/quicknav.js` is not loaded. |
| CSV export | to-author | a console table action, if the product needs it | Not copied off the panel tables. |
| Confirm dangerous actions | intentionally replaced | explicit confirmation in the console; AgentHub review is pending | Browser `confirm()` is not added. Provider mutations currently fail closed until AgentHub integration is complete. |
| Connection indicator | ported | Hosts empty and error states | The page shows the hypervisor error. It does not invent an online status. |

## Reviewed implementation dispositions (2026-11-05)

The imported backup subsystem came from the Wisconsin CT 102 runtime capture. Its `PROVENANCE.md` says the backup files were present in the running tree but "exist in no git history"; the only source root (`shippin-mesh`) has no root license recorded in the checkout audit, and its dependency licenses/provenance are unresolved. Its nested `go.mod` declares `creack/pty`, `gorilla/websocket`, `pgx/v5`, `msteinert/pam`, and `yaml.v3`; those are not approved backup-specific dependencies. The backup implementation stores estate configuration under the local user config directory, contains estate-specific defaults, executes configurable tar/rsync commands, and includes an `rsync --delete` path. The imported HTTP handlers do not prove the canonical tenant, AgentHub, audit, generation or secret resolver boundaries. Therefore the seven backup implementation/test files are rejected as source code. This is not a rejection of backup/restore as a product capability: that remains open in the backlog and `managed-services.md`, to be authored as a provider- and deployment-scoped feature with secret references and restore verification.

The imported certificates, disks, logs and package screens are local host administration surfaces. Certificate scanning/renewal accesses fixed `/etc` paths and invokes certbot; disk inventory invokes `lsblk`, `smartctl`, `du` and a privileged SMART path; log reads include `/var/log/auth.log` and local journal commands; package actions invoke host `apt-get` install/remove/upgrade. These are not imported. The disk inventory data is independently re-authored from the configured Proxmox API through `internal/hv/proxmox/details.go`, with SMART skipped; the remaining Proxmox certificate, log, apt and fuller disk API items are not routed. The old tests are tied to those implementations and are not platform behavior evidence.

`BACKLOG.md` remains the sole open source row because it contains several requested capabilities still queued below. Its candidate code is not a runtime input. Thus the 216-row counts now distinguish an open backlog document from rejected source implementations and independently re-authored platform behavior.

## Imported files

| File | Status | Console home | How it was aligned |
|---|---|---|---|

| `AGENTS.md` | intentionally replaced | shippin.cloud/console module | The nested module is not built or installed. The console process is the process. Agent instruction files in this tree are not console instructions. |
| `BACKLOG.md` | to-author | this matrix, backlog section | Open items are listed below and mapped to a console home. Checked items that the console already has are not re-built. |
| `CLAUDE.md` | intentionally replaced | shippin.cloud/console module | The nested module is not built or installed. The console process is the process. Agent instruction files in this tree are not console instructions. |
| `Makefile` | intentionally replaced | shippin.cloud/console module | The nested module is not built or installed. The console process is the process. Agent instruction files in this tree are not console instructions. |
| `PROVENANCE.md` | intentionally replaced | this matrix and the import commit | Provenance stays in the import commit and this matrix. The file is not a runtime input. |
| `agent_workspaces.go` | intentionally replaced | internal/api agent routes | Agent workspaces are the console agent identities, not a parallel registry. |
| `agent_workspaces_handlers.go` | intentionally replaced | internal/api agent routes | Agent workspaces are the console agent identities, not a parallel registry. |
| `agent_workspaces_test.go` | intentionally replaced | internal/api agent routes | Agent workspaces are the console agent identities, not a parallel registry. |
| `audit.go` | intentionally replaced | internal/api auditFirst and the operator audit log | The panel's local audit file is not written. Mutations use the console audit row. |
| `auth.go` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `auth_handlers.go` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `auth_pam_linux.go` | rejected-for-security | internal/auth (Nomen session) | Local PAM passwords are a second identity. The console session is Nomen. There is no PAM conversation and no password form. |
| `auth_pam_unsupported.go` | rejected-for-security | internal/auth (Nomen session) | Local PAM passwords are a second identity. The console session is Nomen. There is no PAM conversation and no password form. |
| `auth_test.go` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `backups.go` | rejected-for-security | managed-services backup design; no executor | The local JSON store, cron-like goroutine and arbitrary tar/rsync path execution are not imported. rsync uses --delete; destinations and source paths are host-local. The backup capability remains open in the managed-services backlog for a deployment-scoped, audited, secret_ref-based executor. |
| `backups_coverage.go` | ported | internal/backupcov | Coverage scoring and the event ring were rewritten without the the original site paths and without logging secret material. |
| `backups_estate.go` | rejected-for-security | managed-services backup design; no executor | Estate targets and defaults include installation-specific paths and names. The product will use explicitly enrolled Shippin resources and operator-configured destinations; no host path or estate default is imported. |
| `backups_events.go` | ported | internal/backupcov | Coverage scoring and the event ring were rewritten without the the original site paths and without logging secret material. |
| `backups_handlers.go` | rejected-for-security | managed-services backup design; no executor | The standalone panel handlers are not mounted in the product and do not establish the canonical tenant, AgentHub, audit or deployment-generation boundary. Re-author API and UI actions in their owners. |
| `backups_infisical.go` | rejected-for-security | managed-services backup design; Vaultix secret resolver | Infisical-specific calls and source configuration are not adopted. Product destinations use the configured Vaultix secret_ref contract and audited one-key resolution. |
| `backups_s3.go` | rejected-for-security | managed-services backup design; no transfer executor | The S3 transfer implementation is not adopted. Its dependencies, signing, endpoint semantics and security properties have not been approved for product use; implement a bounded provider with endpoint validation and credential references only after a separate review. |
| `backups_secrets.go` | rejected-for-security | internal/backupcov.Dest secret_ref | The panel wrote access keys into backups-secrets.json. Destinations store a sops:// or vaultix:// ref. The writer is not ported. |
| `backups_smb.go` | rejected-for-security | managed-services backup design; no transfer executor | The SMB transfer implementation and credential-file assumptions are not adopted. The product must use configured secret references and a bounded audited executor. |
| `backups_test.go` | rejected-for-security | no imported tests; platform-owned behavior tests required | These tests exercise the excluded standalone local-store implementation, including production-credential-dependent skips. They do not validate Shippin tenant isolation, AgentHub authorization, runtime execution or restore safety. |
| `catalog.go` | intentionally replaced | internal catalog (migrations 010, 011, 034) and admin catalog pages | The x-warden compose key is not a catalog format. The console catalog is the catalog. |
| `catalog_handlers.go` | intentionally replaced | internal catalog (migrations 010, 011, 034) and admin catalog pages | The x-warden compose key is not a catalog format. The console catalog is the catalog. |
| `catalog_test.go` | intentionally replaced | internal catalog (migrations 010, 011, 034) and admin catalog pages | The x-warden compose key is not a catalog format. The console catalog is the catalog. |
| `certs.go` | rejected-for-security | Proxmox certificate-info API is planned; TLS trust is owned by provider enrollment | Local filesystem scans include certificate directories and certbot renewal invokes host commands. No arbitrary host PEM discovery/renewal route is imported. PVE node certificate information and ACME remain separate provider backlog items. |
| `ci_runners.go` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `ci_runners_handlers.go` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `ci_runners_test.go` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `ci_stats.go` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `ci_stats_handlers.go` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `ci_stats_queries.go` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `ci_stats_queries_test.go` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `ci_stats_test.go` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `components/README.md` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/action_button.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/breadcrumb.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/confirm_dialog.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/connection_row.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/empty_state.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/entity_card.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/entity_list.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/metric_spark.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/section.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/service_row.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/shell.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/status_pill.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/toolbar.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `components/workspace_card.html` | intentionally replaced | web/src/lib/design | The HTML partials are not rendered. Buttons, cards, tables, empty states, and dialogs are the console design system. |
| `config.go` | rejected-for-security | servers.hv_* columns and roles.infra | The env file ships VERIFY_TLS=false and a token secret field. Those keys are not read. TLS verify stays on. token_env is a variable name. |
| `cron.go` | intentionally replaced | internal/jobs | Host crontab editing is not exposed. Scheduled work is the console job runner. |
| `cron_test.go` | intentionally replaced | internal/jobs | Host crontab editing is not exposed. Scheduled work is the console job runner. |
| `customers.go` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `customers_handlers.go` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `customers_handlers_test.go` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `deployments.go` | intentionally replaced | projects and internal/jobs | The panel pipeline is not a second deploy engine. Projects and jobs are the console path. |
| `deployments_handlers.go` | intentionally replaced | projects and internal/jobs | The panel pipeline is not a second deploy engine. Projects and jobs are the console path. |
| `deployments_pipeline.go` | intentionally replaced | projects and internal/jobs | The panel pipeline is not a second deploy engine. Projects and jobs are the console path. |
| `disks.go` | intentionally replaced | internal/hv/proxmox/details.go; existing infrastructure node detail | The old lsblk/smartctl/du host view is not imported. Proxmox node disk inventory is independently re-authored through /nodes/{node}/disks/list?skipsmart=1 and shown as read-only details; SMART probes and local directory usage remain excluded. Disk initialization and pool-specific details remain open. |
| `dns.go` | intentionally replaced | workspace domains; phase 2 node DNS | Editing /etc/hosts on the panel is not the product. Workspace domains exist. Node DNS is a later Proxmox read. |
| `docker.go` | intentionally replaced | catalog and managed service instances | The docker.sock admin page is not an API. Compose templates and managed services are the console feature. |
| `domain_registry.go` | intentionally replaced | workspace domains; phase 2 node DNS | Editing /etc/hosts on the panel is not the product. Workspace domains exist. Node DNS is a later Proxmox read. |
| `domain_registry_test.go` | intentionally replaced | workspace domains; phase 2 node DNS | Editing /etc/hosts on the panel is not the product. Workspace domains exist. Node DNS is a later Proxmox read. |
| `endpoints.go` | intentionally replaced | operator health and config probes | Local process widgets and the panel home are not a second dashboard. Operator health and the probe list are the console view. |
| `estate.go` | intentionally replaced | config, not compiled estate names | Hardcoded estate taxonomy is not a default. Groups, when authored, are configuration. |
| `filemanager.go` | rejected-for-security | phase 3 guest-agent file API in docs/design/proxmox-full-coverage.md | Unrestricted read/write of the panel host filesystem is not a product. Guest files, when authored, go through the hypervisor agent under approval. |
| `firewall.go` | intentionally replaced | internal/hv/proxmox FirewallRules; write in phase 2 | Host iptables editing is not ported. Phase 1 reads PVE firewall rules. Writes wait for the phase 2 approval verb. |
| `firewall_test.go` | intentionally replaced | internal/hv/proxmox FirewallRules; write in phase 2 | Host iptables editing is not ported. Phase 1 reads PVE firewall rules. Writes wait for the phase 2 approval verb. |
| `go.mod` | intentionally replaced | shippin.cloud/console module | The nested module is not built or installed. The console process is the process. Agent instruction files in this tree are not console instructions. |
| `go.sum` | intentionally replaced | shippin.cloud/console module | The nested module is not built or installed. The console process is the process. Agent instruction files in this tree are not console instructions. |
| `health_widgets.go` | intentionally replaced | operator health and config probes | Local process widgets and the panel home are not a second dashboard. Operator health and the probe list are the console view. |
| `help.go` | intentionally replaced | operator health and config probes | Local process widgets and the panel home are not a second dashboard. Operator health and the probe list are the console view. |
| `home.go` | intentionally replaced | operator health and config probes | Local process widgets and the panel home are not a second dashboard. Operator health and the probe list are the console view. |
| `infrastructure_graph.go` | intentionally replaced | operator health; no second graph | The panel graph is not mounted. A console view, if needed, reads the server registry. |
| `infrastructure_graph_handlers.go` | intentionally replaced | operator health; no second graph | The panel graph is not mounted. A console view, if needed, reads the server registry. |
| `install/phase1-install.sh` | rejected-for-security | console release path (not these scripts) | The scripts pin private addresses, use curl -k, and write a token secret into an env file. They are not invoked. Connection data lives on the server row. |
| `install/phase2-reverse-proxy.sh` | rejected-for-security | console release path (not these scripts) | The scripts pin private addresses, use curl -k, and write a token secret into an env file. They are not invoked. Connection data lives on the server row. |
| `install/rollout-warden.sh` | rejected-for-security | console release path (not these scripts) | The scripts pin private addresses, use curl -k, and write a token secret into an env file. They are not invoked. Connection data lives on the server row. |
| `install/warden-infisical-env-refresh.sh` | rejected-for-security | console release path (not these scripts) | The scripts pin private addresses, use curl -k, and write a token secret into an env file. They are not invoked. Connection data lives on the server row. |
| `internal/README.md` | intentionally replaced | console module version | No nested version package. The console build is the version. |
| `internal/client/README.md` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/client/client.go` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/client/policy.go` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/client/registry.go` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/client/registry_test.go` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/client/testdata/clients.yaml` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/client/testdata/clients/acme-corp/client.yaml` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/client/testdata/clients/acme-corp/policy.yaml` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/client/testdata/clients/example-tenant/client.yaml` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/client/testdata/clients/example-tenant/policy.yaml` | intentionally replaced | tenants, workspaces, catalog | Per-client YAML is not a second tenant store. Orgs, workspaces, and the catalog are the console data model. |
| `internal/host/README.md` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/connector.go` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/host.go` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/kinds/README.md` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/kinds/bare/connector.go` | ported | internal/hv/bare | A server with no hypervisor API returns ErrNoHypervisor. It does not pretend to be online. |
| `internal/host/kinds/proxmox/connector.go` | ported | internal/hv/proxmox, /v1/infra/servers, /operator/hosts | Re-authored as the Proxmox driver. Inventory, lifecycle, qemu and lxc snapshots, storage, network, firewall read, vzdump, and UPID waits. The page uses the console shell, not the HTML template. |
| `internal/host/kinds/wardenclyffe/connector.go` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/registry.go` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/registry_kinds_test.go` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/registry_test.go` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/testdata/hosts.yaml` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/testdata/hosts/server1/host.yaml` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/host/testdata/hosts/server2/host.yaml` | intentionally replaced | internal/servers, migration 036, hostd enroll | The YAML host registry and the wardenclyffe kind are not loaded. A hypervisor API is columns on the server row enroll and hostd already use. The kind name is not a driver. |
| `internal/version/version.go` | intentionally replaced | console module version | No nested version package. The console build is the version. |
| `logs.go` | rejected-for-security | future provider-scoped Proxmox syslog/journal read API | The standalone endpoint reads local journal and /var/log files including authentication logs. It is not a tenant/provider-scoped API and can expose secrets or personal data; no host log viewer is imported. Proxmox node log reads remain an explicit future item. |
| `logs_test.go` | rejected-for-security | no imported tests; provider-scoped tests required | Tests are coupled to the excluded local journal/file implementation and do not cover provider scoping, redaction, authorization or audit. |
| `main.go` | intentionally replaced | shippin.cloud/console module | The nested module is not built or installed. The console process is the process. Agent instruction files in this tree are not console instructions. |
| `main_test.go` | intentionally replaced | shippin.cloud/console module | The nested module is not built or installed. The console process is the process. Agent instruction files in this tree are not console instructions. |
| `managed_workspaces.go` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `managed_workspaces_handlers.go` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `managed_workspaces_normalize.go` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `managed_workspaces_test.go` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `network.go` | intentionally replaced | internal/hv/proxmox NodeNetworks | Local netplan writes are not ported. Phase 1 reads PVE bridges, bonds, and VLANs. |
| `notifications.go` | intentionally replaced | internal/mail; phase 3 PVE notification targets | Panel webhooks are not a second notifier. Console mail exists. PVE notification targets are phase 3 and not routed. |
| `oidc.go` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `oidc_client.go` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `oidc_handlers.go` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `oidc_test.go` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `packages.go` | rejected-for-security | future read-only Proxmox apt inventory; no host package mutation | This implementation supports host package discovery and apt-get install/remove/upgrade commands. Host package mutation is not a control-plane feature. PVE apt versions/repositories read APIs remain separate and unimplemented. |
| `platform_systems.go` | intentionally replaced | operator health and config probes | Local process widgets and the panel home are not a second dashboard. Operator health and the probe list are the console view. |
| `platform_systems_handlers.go` | intentionally replaced | operator health and config probes | Local process widgets and the panel home are not a second dashboard. Operator health and the probe list are the console view. |
| `platform_systems_test.go` | intentionally replaced | operator health and config probes | Local process widgets and the panel home are not a second dashboard. Operator health and the probe list are the console view. |
| `proxmox.go` | ported | internal/hv/proxmox, /v1/infra/servers, /operator/hosts | Re-authored as the Proxmox driver. Inventory, lifecycle, qemu and lxc snapshots, storage, network, firewall read, vzdump, and UPID waits. The page uses the console shell, not the HTML template. |
| `runners_page.go` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `screenshots/audit.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/certs.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/cron.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/ctx-cron.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/ctx-docker.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/ctx-files.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/ctx-services.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/ctx-users.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/dashboard.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/disks.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/dns.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/docker.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/endpoints.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/files.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/firewall.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/help.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/home.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/login.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/logs.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/network.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/packages-updates.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/packages.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/services.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/sshkeys.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/sysactions.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/system.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/terminal.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `screenshots/users.png` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `sshkeys.go` | rejected-for-security | Nomen team page and /v1/me/ssh-keys; phase 4 pool ACL | Host-local users and authorized_keys are a parallel directory. Shippin identities stay in Nomen. PVE /access/* is a projection of those roles, not a copied user editor. |
| `static/contextmenu.js` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `static/favicon.svg` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `static/media/login-background-poster.jpg` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `static/media/warden-login-brand.png` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `static/media/warden-login-brand.webp` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `static/modal.js` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `static/quicknav.js` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `static/style.css` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `static/toast.js` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `static/wardenui.css` | intentionally replaced | console shell and design system | Panel CSS, quicknav, toasts, and screenshots are not served. Navigation is operatorNav and tenantNav. |
| `supabase.go` | intentionally replaced | internal/db | A second Postgres pool for userdata is not opened. The console database is the store. |
| `sys_linux.go` | intentionally replaced | hypervisor node APIs (phase 1 network and services; phase 2 disks and status) | Local syscalls and shell-outs against the panel host are not copied. Node facts come from the driver. |
| `sys_unsupported.go` | intentionally replaced | hypervisor node APIs (phase 1 network and services; phase 2 disks and status) | Local syscalls and shell-outs against the panel host are not copied. Node facts come from the driver. |
| `sysactions.go` | rejected-for-security | phase 3 node power, approval verb power | Reboot and shutdown of the panel host from a button are not exposed. Node power is a later operator action with approval. |
| `sysinfo.go` | intentionally replaced | operator health and config probes | Local process widgets and the panel home are not a second dashboard. Operator health and the probe list are the console view. |
| `template_funcs.go` | intentionally replaced | shippin.cloud/console module | The nested module is not built or installed. The console process is the process. Agent instruction files in this tree are not console instructions. |
| `templates/agent_workspaces.html` | intentionally replaced | internal/api agent routes | Agent workspaces are the console agent identities, not a parallel registry. |
| `templates/audit.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/backups.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/catalog.html` | intentionally replaced | internal catalog (migrations 010, 011, 034) and admin catalog pages | The x-warden compose key is not a catalog format. The console catalog is the catalog. |
| `templates/certs.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/config.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/cron.html` | intentionally replaced | internal/jobs | Host crontab editing is not exposed. Scheduled work is the console job runner. |
| `templates/customer_detail.html` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `templates/customer_devstation.html` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `templates/customer_devstation_service.html` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `templates/customer_drill_stub.html` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `templates/customers.html` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `templates/deployments.html` | intentionally replaced | projects and internal/jobs | The panel pipeline is not a second deploy engine. Projects and jobs are the console path. |
| `templates/deployments_detail.html` | intentionally replaced | projects and internal/jobs | The panel pipeline is not a second deploy engine. Projects and jobs are the console path. |
| `templates/deployments_new.html` | intentionally replaced | projects and internal/jobs | The panel pipeline is not a second deploy engine. Projects and jobs are the console path. |
| `templates/disks.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/dns.html` | intentionally replaced | workspace domains; phase 2 node DNS | Editing /etc/hosts on the panel is not the product. Workspace domains exist. Node DNS is a later Proxmox read. |
| `templates/docker.html` | intentionally replaced | catalog and managed service instances | The docker.sock admin page is not an API. Compose templates and managed services are the console feature. |
| `templates/endpoints.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/filemanager.html` | rejected-for-security | phase 3 guest-agent file API in docs/design/proxmox-full-coverage.md | Unrestricted read/write of the panel host filesystem is not a product. Guest files, when authored, go through the hypervisor agent under approval. |
| `templates/firewall.html` | intentionally replaced | internal/hv/proxmox FirewallRules; write in phase 2 | Host iptables editing is not ported. Phase 1 reads PVE firewall rules. Writes wait for the phase 2 approval verb. |
| `templates/header.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/help.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/home.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/index.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/login.html` | intentionally replaced | internal/auth, sign-in page | Panel sessions, OIDC hand-roll, and the login page are not a second sign-in. The console already uses Nomen. |
| `templates/logs.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/network.html` | intentionally replaced | internal/hv/proxmox NodeNetworks | Local netplan writes are not ported. Phase 1 reads PVE bridges, bonds, and VLANs. |
| `templates/notifications.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/packages.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/proxmox.html` | ported | internal/hv/proxmox, /v1/infra/servers, /operator/hosts | Re-authored as the Proxmox driver. Inventory, lifecycle, qemu and lxc snapshots, storage, network, firewall read, vzdump, and UPID waits. The page uses the console shell, not the HTML template. |
| `templates/runners.html` | intentionally replaced | not a console surface yet; do not import the HTML | Runner fleet health stays out until it is designed against the console operator page. The panel handlers and template are not mounted. |
| `templates/services.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/sshkeys.html` | rejected-for-security | Nomen team page and /v1/me/ssh-keys; phase 4 pool ACL | Host-local users and authorized_keys are a parallel directory. Shippin identities stay in Nomen. PVE /access/* is a projection of those roles, not a copied user editor. |
| `templates/sysactions.html` | rejected-for-security | phase 3 node power, approval verb power | Reboot and shutdown of the panel host from a button are not exposed. Node power is a later operator action with approval. |
| `templates/sysinfo.html` | intentionally replaced | console React routes | The HTML page is not rendered. Where the job remains, it is a React route on the console shell. |
| `templates/terminal.html` | rejected-for-security | workspace terminal; phase 3 termproxy/vncproxy/spiceproxy | A host root PTY is not the console terminal. Proxmox consoles stay a later phase with a session broker, not a copied websocket page. |
| `templates/users.html` | rejected-for-security | Nomen team page and /v1/me/ssh-keys; phase 4 pool ACL | Host-local users and authorized_keys are a parallel directory. Shippin identities stay in Nomen. PVE /access/* is a projection of those roles, not a copied user editor. |
| `templates/workspaces.html` | intentionally replaced | tenants, workspaces, team | Customer YAML and the drill-down HTML are not a second CRM. Tenants and workspaces already model this. |
| `terminal.go` | rejected-for-security | workspace terminal; phase 3 termproxy/vncproxy/spiceproxy | A host root PTY is not the console terminal. Proxmox consoles stay a later phase with a session broker, not a copied websocket page. |
| `terminal_unsupported.go` | rejected-for-security | workspace terminal; phase 3 termproxy/vncproxy/spiceproxy | A host root PTY is not the console terminal. Proxmox consoles stay a later phase with a session broker, not a copied websocket page. |
| `users.go` | rejected-for-security | Nomen team page and /v1/me/ssh-keys; phase 4 pool ACL | Host-local users and authorized_keys are a parallel directory. Shippin identities stay in Nomen. PVE /access/* is a projection of those roles, not a copied user editor. |
| `users_test.go` | rejected-for-security | Nomen team page and /v1/me/ssh-keys; phase 4 pool ACL | Host-local users and authorized_keys are a parallel directory. Shippin identities stay in Nomen. PVE /access/* is a projection of those roles, not a copied user editor. |
| `warden.env.example` | rejected-for-security | servers.hv_* columns and roles.infra | The env file ships VERIFY_TLS=false and a token secret field. Those keys are not read. TLS verify stays on. token_env is a variable name. |
| `warden.service` | intentionally replaced | shippin.cloud/console module | The nested module is not built or installed. The console process is the process. Agent instruction files in this tree are not console instructions. |

## Additional source documents reviewed and dispositioned

The five named sources below are readable in the local `shippin-mesh` checkout at
`26cce35b` (the original matrix recorded GitHub 404 responses). They have now
been compared with current Shippin owners. These dispositions classify the
documents and do not close the separate capability rows above or establish
archive readiness. The checkout remains read-only with dirty `go.work.sum`; its
root license and dependency/asset provenance remain unresolved.

| Source | Status | Console home | How it was aligned |
|---|---|---|---|
| `docs/specs/02-proxmox-connector.md` | intentionally replaced | `internal/hv/proxmox`, `internal/servers`, infrastructure API and tests | The source defaults TLS verification off, uses singleton environment configuration and accepts UPIDs without polling. Shippin uses configured multi-server records, always-verifying TLS, provider-local lifecycle calls, UPID polling and read-back. Evidence is in `driver.go`, `phase1.go` and `driver_test.go` (`TestTLSRejectsUnknownCA`, task wait/read-back and failed-task cases). Remaining Proxmox coverage stays open in the capability ledger. |
| `docs/WARDENCLYFFE_MODULE_MAP.md` | intentionally replaced | `docs/source-of-truth.md`, `docs/design/service-contexts.md`, capability ledger | Its two-product Warden/Clyffe boundary conflicts with Shippin's one product and one `web/` panel. Reusable bounded-context and operator/customer authorization ideas are expressed in Shippin-owned boundaries; no second product, API or panel is adopted. Individual service, support and infrastructure behaviors remain tracked separately. |
| `docs/WARDENCLYFFE_NAMING_CONVENTIONS.md` | intentionally replaced | `docs/design/naming.md`, `docs/source-of-truth.md` | Historical Warden/Clyffe/mesh labels and schema names are not Shippin identifiers. Shippin Prox is the operator capability name and `proxmox` remains the provider kind. |
| `docs/WARDENCLYFFE_BACKEND_OPTIONS_2026_05.md` | intentionally replaced | `docs/design/managed-services.md`, deployment and persistence owners | Role separation is retained as planning context: PostgreSQL owns platform truth; managed PostgreSQL, Dragonfly, Redis and ClickHouse are separate customer-deployable engines with distinct contracts. No source version pin, topology, site assumption, alternate product database or dependency is adopted. Shippin currently records these service runtimes as design-only. |
| `docs/ai/WARDENCLYFFE_BASE_SKILL.md` and Warden agent skills | intentionally replaced | `AGENTS.md`, `.agents/skills/`, configured AgentHub policy | Persona prompts and site policy are not imported. Shippin repository rules and task skills govern development; AgentHub owns configurable human review. Only independently testable capabilities enter the source ledger after security and provenance review. |

This closes the five document-classification rows only. It does not resolve the
Mesh checkout's root license, dependency and asset rights, dirty local work, or
remaining runtime capabilities; the Mesh source remains ineligible for archive
readiness.
