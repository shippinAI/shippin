# Roadmap

[What Shippin is](objective.md) defines the target and its requirement IDs. This
page says what exists today, in what order the rest gets built, and what each
stage must prove. Updated 2026-11-10 from the owner's direction: a clear product
definition, a rewritten panel on strict design tokens, PostgreSQL and ClickHouse
in every install, every setting in the panel, Incus and Proxmox with real
dashboards, then tenants, subscriptions and a turnkey catalog.

## Where it stands

| Area | Today | Missing |
| --- | --- | --- |
| Install | One command or Docker Compose installs the panel, PostgreSQL 17, ClickHouse 26.8 LTS and (in Compose) Dragonfly with generated credentials and a setup code. The panel's database role is not a superuser and cannot alter or drop the audit log. `/healthz` and the panel's dependency card report each. Fresh install, upgrade with data kept, and restore of a backup have been run. | ClickHouse holds no data until the metrics sampler exists. ClickHouse backup and restore are not done. |
| Settings in the panel | About seven settings can be overridden in the panel. Hosts, health probes, provider images and image servers, workspace templates, team settings and the AgentHub connection are stored in the database and edited in the panel. | Sign-in providers, identity, mail, feature switches, every secret, workspace limits, updates and branding after setup are file or environment only and need a restart. |
| Panel | One React panel (`web/`) on React Aria components. Every colour, size, radius and shadow comes from a strict token file, enforced in CI by `npm run tokens`; text and control edges meet WCAG AA contrast by calculation. | No chart component and no light theme. Pages keep their old layout and navigation, and many forms use one-off inputs instead of the shared field. |
| Infrastructure | Incus and Proxmox hosts are added, tested and listed in the panel; guests are listed; the Incus driver creates guests from configured image servers (verified on real Incus 6.0.4). | The panel refuses every guest creation and other provider change in code, whatever is configured; power actions need an AgentHub review. Host credentials are file paths or environment variable names on the panel's machine. |
| Dashboards | Live point-in-time numbers: Proxmox guest CPU, memory, disk and network; host memory. | Incus guest metrics, host CPU and load, any stored history, range queries, live updates and charts. |
| Access | Local accounts, passkeys, setup of the internal organization, staff roles and team settings exist. | One scoped staff-role authority with primary and secondary roles; current-session MFA everywhere. |
| Tenants | Customers, default workspaces and projects persist with cross-tenant denial tests. | Plans that shape a workspace, quotas, and runnable projects. |
| Services | Catalog, versioned deployment plans and an Incus OCI application executor (volume, container, start, host health probe) exist. A tenant's starter database runs on a shared PostgreSQL on the platform host. | No catalog entry for PostgreSQL, ClickHouse or anything else. The Incus executor has never run against a real host, needs AgentHub with no alternative, does not re-run after a review passes, cannot act on an earlier generation's container, and ignores declared ports. Proxmox has no application path. |

## Order of work

Code is rewritten into its contexts as each stage reaches them, in the order in the [code map](architecture.md).

| Stage | Result someone can see | Proof required |
| --- | --- | --- |
| 0. Clarity and foundation | This contract and roadmap; tidy docs; the [design system](design-system.md) with strict tokens enforced by a check. | The token check runs in CI; every page renders on the token scale. |
| 1. Core install | Fresh installs include ClickHouse beside PostgreSQL and Dragonfly, configured with generated credentials; the panel shows all three healthy. | A clean install overwrites the review deploy; a stopped dependency shows as down. |
| 2. Every setting in the panel | Each configuration section has a validated panel form, stored in PostgreSQL, audited with a reason, live without a restart. Secrets are sealed in the database. The file only bootstraps and seeds a first run. | Change each setting in the panel and observe its effect without a restart; invalid values are refused; the audit log shows who, when and why. |
| 3. Infrastructure and dashboards | Add Incus and Proxmox hosts with uploaded credentials; create and manage guests; live and historical CPU, memory, disk and network for every host and guest. | Real Incus and Proxmox in CI or the local test rig; samples land in ClickHouse; charts show gaps as gaps. |
| 4. Control plane access | Owner and staff with primary and scoped secondary roles, effective-access view, support grants. | Denial tests for every entry point, current-session MFA. |
| 5. Tenants and subscriptions | Plans created in the panel; a subscription opens a workspace shaped by its plan; quotas enforced. | Tenant isolation tests; quota refusals; usage visible to the tenant only. |
| 6. First services | PostgreSQL and ClickHouse deploy for a tenant or the operator from the catalog, guided or advanced, through one executor. | Provision, health, credentials, update, backup, restore and removal for each engine separately. |
| 7. Workspace AI and sources | Activate one's own Claude subscription, authorize Drive content, dispatch work, reconnect remotely. | Owner-runner isolation, revocation, task recovery, real sessions. |
| 8. Catalog | Nomen, Wardenclyffe, Vaultix, Dragonfly and open-source community applications as turnkey entries. | Each entry verified on its own; no blanket parity claim. |
| 9. Build and publish | Deploy from Git; publish an HTTPS address; connect a custom domain; manage the live app. | Route, TLS and DNS verified; safe cutover and rollback. |
| 10. Release | Clean install, upgrade, backup, restore and support runbooks. Heavy CI returns here. | Installer and upgrade CI plus real-provider journeys. |

A stage closes only when its proof passes. Incus does not close Proxmox, and
PostgreSQL does not close another engine.

## How work is recorded

Each packet cites requirement IDs, inspects the whole relevant source, records one
user journey, its failure paths, the commands run and what was observed, in the
local governance session ([governance/README.md](../governance/README.md)). Report
implemented, tested, deployed and live-provider verified separately.
Capabilities from older panels are tracked in the
[absorption ledger](absorption/capability-absorption.md).
