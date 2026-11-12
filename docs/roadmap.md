# Roadmap

[What Shippin is](objective.md) defines the target, software for running a
commercial cloud provider, and its requirement IDs. This page says what exists
today, the order the rest gets built in, and what each stage must prove.

Updated 2026-11-11 from the owner's direction:
- Shippin is a custom commercial cloud.
- Its own Incus and Proxmox hosts sit in one control plane, with public clouds (AWS, Google Cloud, Azure, DigitalOcean, Fly.io) mixed in.
- Customers buy a VM and deploy services onto it with one click.
- They get a terminal anyone can use, and DevPulse syncs with their cloud workspace.
- Friends and family are the first customers.

## Where it stands

| Area | Today | Missing |
| --- | --- | --- |
| Install | One command or Docker Compose installs the panel, PostgreSQL 17, ClickHouse 26.8 LTS and (in Compose) Dragonfly, with generated credentials and a setup code. The panel's database role is not a superuser and cannot alter or drop the audit log. `/healthz` and the panel's dependency card report each part. A fresh install, an upgrade that keeps data, and a backup restore have been run. | ClickHouse holds no data until the metrics sampler exists. ClickHouse backup and restore are not done. |
| Settings in the panel | About eight settings can be overridden in the panel, including who can sign up. Hosts, health probes, provider images and image servers, workspace templates, team settings and the AgentHub connection are stored in the database and edited in the panel. | Sign-in providers, identity, mail, feature switches, every secret, workspace limits, updates and branding after setup are file or environment settings only, and need a restart. |
| Panel | One React panel (`web/`) on React Aria components. Every colour, size, radius and shadow comes from a strict token file, enforced in CI by `npm run tokens`. Text and control edges meet WCAG AA contrast by calculation. | No chart component and no light theme. Pages keep their old layout and navigation, and many forms use one-off inputs instead of the shared field. |
| Own hosts (Incus, Proxmox) | Hosts of both kinds are added and tested in the panel. Guests are created, started, stopped and deleted from it, with a required reason, an audit entry and a strict confirmation for deletes. This was verified in a browser on real Incus and real Proxmox VE 9. AgentHub review applies to power actions once connected. | One fleet view of both kinds with their resources side by side; regions; placement across hosts. Advanced host credentials can still be file paths or environment variable names. Clone, migrate and resize have not been exercised on real hosts. |
| Public clouds (HYB-01) | Nothing. `internal/hv` has Incus, Proxmox and a bare driver only. | A driver each for AWS, Google Cloud, Azure, DigitalOcean and Fly.io; cloud accounts with sealed credentials; regions. |
| Customer-brought servers (BYO-01) | The host agent and node enrollment exist for the provider's own hosts. | Not checked as a customer flow: a customer adding their own server to their organization. |
| Dashboards | Live point-in-time numbers: Proxmox guest CPU, memory, disk and network; host memory. | Incus guest metrics, host CPU and load, any stored history, range queries, live updates and charts. |
| Built-in services and recovery | The console shows PostgreSQL, ClickHouse and Dragonfly live. ClickHouse can be changed or turned off from it behind a strict confirmation. Database backups, download, the recovery key and a restore script work. Verified: wipe, restore, sign in, hosts answer. | PostgreSQL and Dragonfly connections are still set at install. No scheduled or off-machine backups. ClickHouse history is not backed up. |
| Access | Local accounts, passkeys, setup of the internal organization, staff roles, per-person catalog grants and team settings exist. A coverage tier's sign-in rule is enforced on every organization request. Development sign-ins exist, off unless an installation lists them. | One scoped staff-role authority with primary and secondary roles; current-session MFA everywhere. |
| Sign-up and invitations (LAUNCH-01) | **Customers > Sign-ups** sets who can sign up (invitation only, with a public waitlist, or anyone), live and audited. In waitlist mode the sign-in page links to `/waitlist`; people confirm their address from an emailed link, and staff invite them from the list. The invitation is emailed, or its one-time link is shown to copy. The waitlist refuses entries while it is closed or while mail is not set up. Staff and organization owners also invite people from the panel, and `/invite` claims an invitation. | An invitation that also creates the person's own organization on a chosen plan and coverage. Data export and full removal of a customer have not been proved. Mail itself is still a file setting. |
| Customers, plans and coverage | Customers, default workspaces and projects persist, with cross-tenant denial tests. Plans and coverage tiers are edited on **Plans & tiers**; each plan includes a tier, and a customer can be moved to another. Members see their coverage in Settings. | Plans that shape a workspace, quotas enforced from the plan, runnable projects, and metering. Prices are settings and are not needed to launch. |
| Customer machines (VM-01) | A VM-per-organization design and a first implementation ([VM-first tenants](reference/vm-first-tenants.md)), from October 2026. | Not verified on the current build. A customer ordering a VM from their plan in the panel, by size, image and region, does not exist. |
| One-click services (CAT-01) | Catalog, versioned deployment plans and an Incus OCI application executor (volume, container, start, host health probe) exist. A tenant's starter database runs on a shared PostgreSQL on the platform host. | No catalog entry for PostgreSQL, ClickHouse, Coolify or our products. Nothing deploys onto a customer's VM yet. The Incus executor has never run against a real host. It needs AgentHub with no alternative, does not re-run after a review passes, cannot act on an earlier generation's container, and ignores declared ports. Proxmox has no application path. |
| Terminal (TERM-01) | A browser terminal opens the workspace's Zellij session and reconnects to it. It can hand off to WezTerm, has guides beside it, and has AI app cards with exact install commands. | The plain-language layer: asking in words, seeing the commands before they run, errors explained, and one-press common jobs. The same terminal on a customer's VM. |
| DevPulse sync (SYNC-01) | Device-code sign-in and short-lived sessions for `shippin-bridge`. | A published client API for listing projects, syncing files both ways with conflicts shown, opening sessions and following runs; and a per-device list with revocation in the panel. |

## Order of work

Code is rewritten into its contexts as each stage reaches them, in the order in
the [code map](architecture.md). The first milestone is the friends-and-family
launch: the [acceptance journey](objective.md#acceptance-journey-friends-and-family)
on our own Incus and Proxmox hosts. Public clouds and billing come after it.

### Milestone 1: friends and family

| Stage | Result someone can see | Proof required |
| --- | --- | --- |
| 0. Clarity and foundation | This contract and roadmap; tidy docs; the [design system](design-system.md) with strict tokens enforced by a check. Done, and kept current. | The token check runs in CI; every page renders on the token scale. |
| 1. Core install | Fresh installs include ClickHouse beside PostgreSQL and Dragonfly, configured with generated credentials; the panel shows all three healthy. Done. | A clean install overwrites the review deploy; a stopped dependency shows as down. |
| 2. Every setting in the panel | Each configuration section has a validated panel form. It is stored in PostgreSQL, audited with a reason and live without a restart; sign-up mode is done. Secrets are sealed in the database. The file only bootstraps and seeds a first run. | Change each setting in the panel and observe its effect without a restart; invalid values are refused; the audit log shows who, when and why. |
| 3. Fleet and dashboards | Incus and Proxmox hosts in one fleet view with their resources side by side, grouped into regions. Live and historical CPU, memory, disk and network for every host and guest. | Real Incus and Proxmox in the local test rig; samples land in ClickHouse; charts show gaps as gaps. |
| 4. Control plane access | Owner and staff with primary and scoped secondary roles, an effective-access view, and support grants. | Denial tests for every entry point; current-session MFA. |
| 5. Customers, invitations and plans | One invitation creates a customer on a chosen plan and coverage, and the panel shows who accepted. The plan shapes the workspace and quotas are enforced. Customer data can be exported and removed. | Invite, accept, isolate, refuse over quota, export, remove, and prove nothing is left. |
| 6. Customer machines | A customer orders a VM by size, image and region. It is placed on an Incus or Proxmox host and has a console, power actions, snapshots, rebuild and removal. | On both real hypervisors; isolation between two customers; removal leaves nothing. |
| 7. One-click services | PostgreSQL and ClickHouse, then Coolify and our products (Nomen, Wardenclyffe, ShippinTech, Devagotchi), deploy onto a customer's VM or the provider's capacity, guided or advanced, through one executor. | Provision, health, credentials, update, backup, restore and removal for each entry separately. |
| 8. Terminal anyone can use | The plain-language layer over the real shell, on workspaces and customer VMs. | A non-engineer completes set jobs (start, logs, deploy, back up) without typing a command; destructive actions are confirmed; the shell stays reachable. |
| 9. Workspace AI and sources | Activate one's own Claude subscription, authorize Drive content, dispatch work, reconnect remotely. | Owner-runner isolation, revocation, task recovery, real sessions. |
| 10. DevPulse sync | The client API and per-device access. DevPulse signs in, lists projects, syncs files both ways, and opens a session. | Two-way sync with a forced conflict; a revoked device is refused at once. |
| 11. Build and publish | Deploy from Git; publish an HTTPS address; connect a custom domain; manage the live app. | Route, TLS and DNS verified; safe cutover and rollback. |
| Launch | Invite friends and family onto our own hosts. | The whole acceptance journey, run end to end on the review deploy and then on our hosts, with evidence. |

### Milestone 2: commercial cloud

| Stage | Result someone can see | Proof required |
| --- | --- | --- |
| 12. Public clouds | Cloud accounts and drivers, one at a time: DigitalOcean, Fly.io, AWS, Google Cloud and Azure. Their machines appear in the fleet beside our hosts, and placement can use them. | Each driver on a real account: create, observe, power, remove, and billing-safe cleanup. One cloud does not close another. |
| 13. Customer-brought capacity | A customer adds their own server or cloud account, and workloads deploy onto it for that customer only. | Isolation from other customers; detaching leaves their machine intact. |
| 14. Metering and billing | Usage per customer, then invoicing and payment collection as an integration. | Metered usage matches what ran; no charge without a record. |
| 15. Release | Clean install, upgrade, backup, restore and support runbooks. Heavy CI returns here. | Installer and upgrade CI plus real-provider journeys. |

A stage closes only when its proof passes. Incus does not close Proxmox,
PostgreSQL does not close another engine, and one cloud does not close another.

## How work is recorded

Each packet does the following:
- cites requirement IDs;
- inspects the whole relevant source;
- records one user journey, its failure paths, the commands run and what was observed, in the local governance session ([governance/README.md](../governance/README.md)).

Report implemented, tested, deployed and live-provider verified separately.
Capabilities from older panels are tracked in the
[absorption ledger](absorption/capability-absorption.md).
