# Roadmap

[What Shippin is](objective.md) defines the target and its requirement IDs: a
self-hosted panel in this repository, and an enterprise module for running a
multi-tenant hosted cloud ([Editions and licensing](editions.md)). This page says
what exists today, the order the rest gets built in, and what each stage must prove.

Updated 2026-11-20: the immediate delivery priority is a reachable, claimable installation and a coherently grouped panel, followed by real Incus and Proxmox journeys. The [foundation delivery plan](foundation-delivery.md) defines P1–P7, ownership and observable acceptance gates. P1 and P2 use the existing owners and may precede the full enterprise extraction; broad context rewrites still follow the separation. A plan is not deployment evidence.

The 2026-11-19 edition decision remains: the public repository is the self-hosted panel, in the spirit of Coolify. Multi-tenant and commercial capabilities move to the private enterprise repository `shippinAI/shippin-enterprise`. Workspaces in the panel run on the owner's own hosts through the hypervisor's API.

Updated 2026-11-13: the hosted development space (SOLO-01 to COACH-01) comes first for solo developers, in the spirit of Lovable on their own subscriptions, and it teaches as it goes.

Updated 2026-11-11 from the owner's direction:
- Shippin is a custom commercial cloud.
- Its own Incus and Proxmox hosts sit in one control plane, with public clouds (AWS, Google Cloud, Azure, DigitalOcean, Fly.io) mixed in.
- Customers buy a VM and deploy services onto it with one click.
- They get a terminal anyone can use, and DevPulse syncs with their cloud workspace.
- Friends and family are the first customers.

Added 2026-11-24: the [hosting coverage plan](hosting-coverage.md) maps what WHMCS and cPanel cover to the requirements here, with the development space's modes, and places the gaps in this order of work. It proposes; this page and the contract decide.

## Every request, in one place

Updated 2026-11-19 (assistant skill library row) and 2026-11-15. Each request from the owner, the requirement it falls under in [What Shippin is](objective.md), and where it really stands. **Done** means seen working on the review deploy or a real system; **built** means tests pass but it has not been seen working for real; **designed** means a written design only.

| Request | Requirement | Status |
| --- | --- | --- |
| A Lovable-like hosted space where a solo developer builds on their own AI subscriptions (enterprise) | SOLO-01, DEV-01, AI-01 | **Not started as a product.** Pieces exist: workspaces with Zellij, AI app cards, the AI-01 runner design. No workspace has run on the current build. |
| The infrastructure under that space: where each space runs, its limits, its cost | VM-01, PLAN-01, INF-01 | **Not started.** Workspaces are Incus containers placed by hostd; nothing yet proves one runs here, sizes it from a plan, or stops it when idle. |
| A free tier that is cheap: VM-like spaces without a VM's weight | VM-01, PLAN-01 | **Not started.** The approach: system containers, idle spaces stopped, copy-on-write images, shared databases, limits from the plan. |
| One workspace screen: VS Code, the Zellij terminal, the person's AI apps | IDE-01 | **Not started.** Unverified: Claude desktop and ChatGPT desktop run on macOS and Windows, not in a Linux space; the command-line agents do. |
| Shippin.AI as the first sign-in, through Nomen, that the developer can extend to their own apps | IDP-01 | **Not started.** Sign-in to the panel exists; sign-in for a customer's own apps (one client per app) is not in the contract yet. |
| Instructions, hooks and governance that onboard a developer after sign-in | GUIDE-01 | **Not started.** |
| shippin.bot: knows the page and the person's permissions, runs known flows without the model, its own repo | GUIDE-01, COACH-01 | **Not started.** Waiting on the repo's name and visibility. |
| shippin.hub: connects Shippin to Lovable and Replit with connectors kept in Vaultix | SYNC-01, EXT-01 | **Not started.** Device sign-in for `shippin-bridge` exists. |
| A free self-hosted edition for one owner, separate from the multi-tenant cloud | OPEN-01, SOLO-01 | **Decided 2026-11-19:** this repository is the self-hosted panel; the multi-tenant cloud is the enterprise module in `shippinAI/shippin-enterprise` ([Editions and licensing](editions.md)). **Not built:** the multi-tenant code is still here; see [Separation, first](#separation-first). |
| Workspaces on the owner's own hosts, created and opened through the hypervisor's API | WS-01, TERM-01 | **Not started.** Workspaces today run only through `hostd`'s per-customer isolation, which moves to the enterprise module. |
| Vaultix, Wardenclyffe and Nomen offered as services | CAT-01 | **Not started.** Stage 7. |
| Every setting edited in the panel, keys saved to the installation's Vaultix | CFG-01 | **Done** for build runners, the job queue and the GitHub App, on the review deploy and against a real Vaultix. The owner's Vaultix account is not connected yet. The other sections are next in stage 2. |
| The control plane runs on the owner's own Incus or Proxmox: installed anywhere once, it deploys itself as a guest on an attached host and manages both kinds from there; nothing about one installation is in the code | INF-01, OPEN-01, CORE-01 | **Not started.** Installing anywhere and attaching both kinds work today. Self-deployment onto a host, moving between hosts, and running on several hosts are not built. |
| Placement: the panel chooses the host for a workload across Incus and Proxmox | INF-01, VM-01 | **Not started.** Guests are created on the host the operator picks. |
| Add a server: the panel works out Incus or Proxmox from the address and the operator confirms | INF-03 | **Done** on the review deploy against real Incus and Proxmox VE 9.2. The dialog has not been seen in a browser. |
| Dashboards: CPU, memory, disk and network, live and over time, stored in ClickHouse | OBS-01 | **Done for samples:** real Incus and Proxmox hosts are sampled into ClickHouse every 30 seconds and read back with gaps kept. **Built** for the charts and fleet view (not seen in a browser). Incus does not report host CPU or load; the Proxmox rig's CPU reads 0, not yet explained. |
| PostgreSQL and ClickHouse built and deployed properly | CORE-01, CAT-01 | **Done** for the panel's own (install, health, backup). **Not started** for deploying them onto a customer's machine (stage 7). |
| Infrastructure VMs assigned to an account that staff can change | VM-01, INF-01 | **Partly.** A machine per organization can be enabled, started and stopped. Reassigning and resizing do not exist; proving them needs a host that can run VMs. |
| The workspace terminal: the enhanced Zellij session, for that workspace only | TERM-01, IDE-01 | **Built:** follows the window size, reconnects by itself, records how each session ended, refuses anyone outside the workspace. Not run on a real workspace. |
| A shell on the server itself, in the control plane, as granted privileged access | INF-03, IAM-01 | **Designed:** [Host shell](host-shell.md), with the grant dialog and limited impersonation. Five decisions are the owner's. |
| CI: main only moves on green, failures are loud and make the AI fix them, tests are not weakened to pass | ENG-01 | **Done.** Pushes go to `queue/main`; the Gate promoted the first one to main by itself. The failure alarm has not been triggered for real yet. |
| A license that lets people self-host but not compete | OPEN-01 | **Done:** FSL-1.1-ALv2, licensor shippinAI. A contributor agreement is not set up. |
| An enterprise edition, managed and unmanaged, in its own repository | OPEN-01 | **Designed:** [Editions and licensing](editions.md), a private module over this panel with a signed license key; not a fork. The repository is `shippinAI/shippin-enterprise` (decided 2026-11-19). Extension points, the key check and the repository itself are not built. |
| A personal lexicon and prompt recipes from the owner's own chats | (tooling, later COACH-01) | **Done** as tooling (`governance/lexicon.mjs`). 16 terms and 14 recipes wait for the owner to confirm them. |
| A reviewed skill library assistants load by intent | GUIDE-01, COACH-01 | **Built** in [`.agents/library/`](../.agents/library/README.md): the tree, the linter, and the flat export. No assistant has loaded it, and shippin.bot is still not started. |

## Where it stands

| Area | Today | Missing |
| --- | --- | --- |
| MCPL (MCPL-01) | Actions are files, `internal/<domain>/<entity>/<verb>.go`. Their IDs and routes, the registry, the panel's typed calls and the OpenAPI document are generated, again inside the image build, and `mcplgen new` starts a new action with its test. One runner handles input, reason, transaction, audit and errors. Actions mount for staff, members, signed-in people and the public. Every change emits an event, streamed to the people who could have run it. MCP at `/v1/mcp` is an entrance that leads to one boundary per domain, each with its knowledge and its actions as tools, and AI clients sign in with access keys made under **Account > AI access**. An entity's reactions and services are declared in its folder too. Coverage tiers, plans, catalog services, customers, an organization's coverage, SSH keys and access keys are in that shape. So are a server's list and read, and its guests: list, read, create, delete, start, stop, shut down and restart. 300 legacy routes are listed, and the list only shrinks. | Every other entity moved into that shape; UI actions. See [MCPL](mcpl.md). |
| Hosted development space (SOLO-01, DEV-01, IDE-01, GUIDE-01, COACH-01) | Workspaces with Zellij, a WezTerm hand-off, AI app cards and the AI-01 subscription runner design. MCPL events exist to drive explanations and a coach. | The single focused workspace screen with a customized VS Code; AI apps shown inside it; explanations and motion driven by events, with a per-person explanation level; the specialist that follows events, judges skill and keeps the person's notes. |
| Install | One command or Docker Compose installs the panel, PostgreSQL 17, ClickHouse 26.8 LTS and (in Compose) Dragonfly, with generated credentials and no setup code: the first person to open it creates the owner, or the install names the owner up front (REL-03, 2026-11-25). The panel's database role is not a superuser and cannot alter or drop the audit log. `/healthz` and the panel's dependency card report each part. A fresh install, an upgrade that keeps data, and a backup restore have been run. | ClickHouse backup and restore are not done. |
| Settings in the panel | About eight settings can be overridden in the panel, including who can sign up. Hosts, health probes, provider images and image servers, workspace templates, team settings and the AgentHub connection are stored in the database and edited in the panel. | Sign-in providers, identity, mail, feature switches, every secret, workspace limits, updates and branding after setup are file or environment settings only, and need a restart. |
| Panel | One React panel (`web/`) on React Aria components. Every colour, size, radius and shadow comes from a strict token file, enforced in CI by `npm run tokens`. Text and control edges meet WCAG AA contrast by calculation. | No chart component and no light theme. Pages keep their old layout and navigation, and many forms use one-off inputs instead of the shared field. |
| Own hosts (Incus, Proxmox) | Hosts of both kinds are added and tested in the panel; "Add host" first detects the kind, address and certificate from the address alone, checked against real Incus and Proxmox VE 9.2 rigs. Guests are created, started, stopped and deleted from it, with a required reason, an audit entry and a strict confirmation for deletes. This was verified in a browser on real Incus and real Proxmox VE 9. AgentHub review applies to power actions once connected. | One fleet view of both kinds with their resources side by side; regions; placement across hosts. Advanced host credentials can still be file paths or environment variable names. Clone, migrate and resize have not been exercised on real hosts. |
| Public clouds (HYB-01) | Nothing. `internal/hv` has Incus, Proxmox and a bare driver only. | A driver each for AWS, Google Cloud, Azure, DigitalOcean and Fly.io; cloud accounts with sealed credentials; regions. |
| Customer-brought servers (BYO-01) | The host agent and node enrollment exist for the provider's own hosts. | Not checked as a customer flow: a customer adding their own server to their organization. |
| Dashboards | Live point-in-time numbers: Proxmox guest CPU, memory, disk and network; host memory. A [sampler](metrics.md) writes every attached host's and guest's CPU, memory, disk and network to ClickHouse on the schedule in **Settings > Metrics**, one panel process at a time, and a staff range read returns them in steps with missing time as nulls. Tested against a real ClickHouse and PostgreSQL, and against recorded Incus and Proxmox answers. | Proof on real Incus and Proxmox hosts; Incus host CPU, load, disk and network; live updates and charts. |
| Built-in services and recovery | The console shows PostgreSQL, ClickHouse and Dragonfly live. ClickHouse can be changed or turned off from it behind a strict confirmation. Database backups, download, the recovery key and a restore script work. Verified: wipe, restore, sign in, hosts answer. | PostgreSQL and Dragonfly connections are still set at install. No scheduled or off-machine backups. ClickHouse history is not backed up. |
| Access | Setup creates a built-in `setup` account that an installer, a person or an AI assistant, finishes setting up with. A banner on every page then walks the owner through taking it over: set its password (other sessions of it signed out) and create their own owner account. Local accounts, passkeys, setup of the internal organization, staff roles, per-person catalog grants and team settings exist. A coverage tier's sign-in rule is enforced on every organization request. Development sign-ins exist, off unless an installation lists them. | One scoped staff-role authority with primary and secondary roles; current-session MFA everywhere. |
| Sign-up and invitations (LAUNCH-01) | **Customers > Sign-ups** sets who can sign up (invitation only, with a public waitlist, or anyone), live and audited. In waitlist mode the sign-in page links to `/waitlist`; people confirm their address from an emailed link, and staff invite them from the list. The invitation is emailed, or its one-time link is shown to copy. The waitlist refuses entries while it is closed or while mail is not set up. Staff and organization owners also invite people from the panel, and `/invite` claims an invitation. | An invitation that also creates the person's own organization on a chosen plan and coverage. Data export and full removal of a customer have not been proved. Mail itself is still a file setting. |
| Customers, plans and coverage | Customers, default workspaces and projects persist, with cross-tenant denial tests. Plans and coverage tiers are edited on **Plans & tiers**; each plan includes a tier, and a customer can be moved to another. Members see their coverage in Settings. | Plans that shape a workspace, quotas enforced from the plan, runnable projects, and metering. Prices are settings and are not needed to launch. |
| Customer machines (VM-01) | A VM-per-organization design and a first implementation ([VM-first tenants](reference/vm-first-tenants.md)), from October 2026. | Not verified on the current build. A customer ordering a VM from their plan in the panel, by size, image and region, does not exist. |
| One-click services (CAT-01) | Catalog, versioned deployment plans and an Incus OCI application executor (volume, container, start, host health probe) exist. A tenant's starter database runs on a shared PostgreSQL on the platform host. | No catalog entry for PostgreSQL, ClickHouse, Coolify or our products. Nothing deploys onto a customer's VM yet. The Incus executor has never run against a real host. It needs AgentHub with no alternative, does not re-run after a review passes, cannot act on an earlier generation's container, and ignores declared ports. Proxmox has no application path. |
| Terminal (TERM-01) | A browser terminal opens the workspace's Zellij session, follows the pane's size, and reconnects to it by itself after a drop; open and close are audited with how long it ran and why it ended. Tested against a local PTY, not yet on a real workspace. It can hand off to WezTerm, has guides beside it, and has AI app cards with exact install commands. | The plain-language layer: asking in words, seeing the commands before they run, errors explained, and one-press common jobs. The same terminal on a customer's VM. |
| Build runners (RUN-01) | Staff offer runner sizes; an organization assigns its GitHub repositories to workspaces and projects; GitHub's job events are recorded per organization, waiting or refused against the plan's sizes and minutes. A machine is started per job within the plan's jobs-at-once limit, fair between organizations, never twice, and removed when the job ends or after the time limit; staff list and remove runner machines. The organization's Build runners page and staff's Runner sizes page are in the panel. Tested against fakes, and on the local Incus rig with a container (it cannot run VMs). PR #20 retains its original history while keeping the current runtime; [source disposition](runner-service.md) records superseded wiring and open profile/gateway gaps. | A runner image and a real job end to end on a VM, which needs a GitHub App and the runner release; staff's runner machines page; cancelling jobs and keeping logs; Proxmox, clouds and customers' own servers; the catalog entry for a customer's own runner. |
| DevPulse sync (SYNC-01) | Device-code sign-in and short-lived sessions for `shippin-bridge`. | A published client API for listing projects, syncing files both ways with conflicts shown, opening sessions and following runs; and a per-device list with revocation in the panel. |

## Order of work

The [foundation packets](foundation-delivery.md#delivery-packets) govern immediate delivery:
P1 reaches and claims an installation; P2 makes context and navigation coherent.
The enterprise separation remains the prerequisite for broad context rewrites.
Code is then rewritten into its contexts as each
stage reaches them, in the order in the [code map](architecture.md). The first milestone is the friends-and-family
launch: the [acceptance journey](objective.md#acceptance-journey-friends-and-family)
on our own Incus and Proxmox hosts. Public clouds and billing come after it.

### Separation, first

The 2026-11-19 separation direction, qualified by the P1/P2 foundation priority above: enterprise code leaves before broad context rewrites. Each step is closed only by its proof, and every commit leaves both editions working. Enterprise code moves with its tests; it is not rewritten on the way.

| Step | Result someone can see | Proof required |
| --- | --- | --- |
| S0. The split is the contract | [Editions and licensing](editions.md), [What Shippin is](objective.md) and the [code map](architecture.md) say what belongs to each edition. | Every requirement ID belongs to the panel, the module, or both, with each part stated. |
| S1. Extension points | `app.Main`, the module registry in `internal/ext` (actions, settings sections, permissions, job kinds, migrations), the caller, organization lifecycle, limits and placement policies, and panel pages from folders, a module's included (F1 in the [panel framework](web-framework.md)). No behaviour changes. | A test module registers one of each, and the tests see it work end to end. The panel with no module behaves as before. |
| S2. The enterprise repository | `shippinAI/shippin-enterprise` builds an enterprise image from the panel, pinned at a commit, plus its module. | The enterprise image runs the same panel; its CI passes. |
| S3. The code moves | One area at a time, enterprise first, then removed here: sign-up and acquisition; plans, coverage and customers; access to customers; `hostd` and per-customer isolation, tenant databases and hosted Vaultix; then a new database baseline (PRE-01) in which the installation's own team is an ordinary organization. | After each area, both review deploys are seen in a browser: the panel without it, the enterprise build doing what it did before, as Admin, Staff and Customer. |
| S4. It stays separate | A check in CI fails on enterprise packages, tables, routes or permissions in this repository. | The check fails on a planted example and passes on main. |

The [panel framework](web-framework.md) track continues in this order: F5 rot checks, F4 lexicon, F2 tokens from one source, F3 components and their gallery. It does not block P1/P2 or reaching S2. Each step is checked and seen on the review deploy; the framework document distinguishes working machinery from planned tooling.

The [builds and releases](release.md) track runs beside it, decided by the owner on 2026-11-20: a private edge build from every green `main`, and public tagged releases. The order: R1 build info and R2 Go checks first; R3 licenses before S2; then R4 edge and R5 releases; R6 updates after Phase 4.

Gaps checked in the code on 2026-11-19, and where each is closed:

| Gap | Found | Closed by |
| --- | --- | --- |
| Limits are read in two places | `hostd` keeps its own `plans.<name>` sizes (quota, CPU, memory, disks, VM) in its config file beside the catalog's plans, and reads them in eleven places. | Build-runner limits come through the limits policy (S1). hostd's sizes are the per-customer silo's, so they move with hostd in S3, where the enterprise module's limits policy supplies them and hostd's own plan list goes. |
| No row-level security; four overlapping organization tables | `tenants`, `orgs`, `identity_orgs` and `tenant_teams`; no table has a row policy. | S3's new baseline: one organizations table, and a row policy on every table scoped to an organization, with a test that fails on one without. |
| 300 routes outside the action schema | `internal/api/testdata/mcpl-undescribed.txt`, which may only shrink. | Each group as its work comes up; workspaces and guests with D1. |
| Tenant database backups are a stub | A backup is recorded as `pending_backend`; restore always refuses. | Real dumps to storage the owner configures, then restore, after D1. |
| One managed service kind | `internal/services/managed` provisions only `rrflow-state`. | Stage 7: PostgreSQL first, then S3, DNS and mail, through the same managed-service model. |
| No metering, invoicing or payment | The billing permissions are reserved placeholders. | Stage 14, in the enterprise edition: the owner decided on 2026-11-20 that the hosting layer (customers, plans, billing, provisioning, sign-up) is licensed to hosting companies with a signed key, like WHMCS. |

### The hosted development space

The 2026-11-13 direction puts the development space first, after the separation. These steps come before the stages below, in this order, each closed only by its proof. Steps marked enterprise are built in the enterprise module.

| Step | Result someone can see | Proof required |
| --- | --- | --- |
| D1. A workspace runs on your own host | A workspace is created as a container on an Incus host added in the panel, through the Incus API, and opened in the browser terminal (WS-01). | On the local Incus rig: create, open the Zellij terminal, type, resize, reconnect, stop, remove; another person is refused. |
| D2. Sign up for a hosted space (enterprise) | A solo developer signs up through the installation's sign-in provider and lands in their own hosted space. | A new person, one sign-in, a space that is theirs; a second person cannot see it. |
| D3. One screen | VS Code, the terminal and an AI panel in one focused screen (IDE-01). | Edit, run and see the change from the one screen. |
| D4. Their own AI | Claude Code and Codex signed in with the person's own subscription in their runner (AI-01). | Their sign-in, never pooled; revoking it stops it. |
| D5. Preview and publish | What they built is previewed, then published at an address (PUB-01). | A change reaches the live address; rollback works. |
| D6. Cheap spaces (enterprise) | Spaces sized by plan, stopped when idle and woken on use; the cost of an idle free space is measured (VM-01, PLAN-01). | Density and idle cost measured on a real host. |
| D7. Guided | Explanations from events and shippin.bot on each page (GUIDE-01, COACH-01). | A first-time person completes the first build with its help. |

### Milestone 1: friends and family

Stages 5 and 6 and the launch are enterprise work; the others are the panel.

| Stage | Result someone can see | Proof required |
| --- | --- | --- |
| 0. Clarity and foundation | This contract and roadmap; tidy docs; the [design system](design-system.md) with strict tokens enforced by a check. Done, and kept current. | The token check runs in CI; every page renders on the token scale. |
| 1. Core install | Fresh installs include ClickHouse beside PostgreSQL and Dragonfly, configured with generated credentials; the panel shows all three healthy. Done. | A clean install overwrites the review deploy; a stopped dependency shows as down. |
| 2. Every setting in the panel | Each configuration section has a validated panel form. It is stored in PostgreSQL, audited with a reason and live without a restart. Done: sign-up mode; the settings store, the Vaultix secret store and their panel pages; build runners, the job queue and the GitHub App. Keys are saved to the installation's Vaultix account, connected in the panel with a machine identity ([design](settings.md)). The file only bootstraps and seeds a first run. | Change each setting in the panel and observe its effect without a restart; invalid values are refused; the audit log shows who, when and why. A key entered in the panel is in Vaultix and in no database row, response, log or audit entry. |
| 3. Fleet and dashboards | Incus and Proxmox hosts in one fleet view with their resources side by side, grouped into regions. Live and historical CPU, memory, disk and network for every host and guest. | Real Incus and Proxmox in the local test rig; samples land in ClickHouse; charts show gaps as gaps. |
| 4. Control plane access | Owner and staff with primary and scoped secondary roles, an effective-access view, and support grants. | Denial tests for every entry point; current-session MFA. |
| 5. Customers, invitations and plans | One invitation creates a customer on a chosen plan and coverage, and the panel shows who accepted. The plan shapes the workspace and quotas are enforced. Customer data can be exported and removed. | Invite, accept, isolate, refuse over quota, export, remove, and prove nothing is left. |
| 6. Customer machines | A customer orders a VM by size, image and region. It is placed on an Incus or Proxmox host and has a console, power actions, snapshots, rebuild and removal. | On both real hypervisors; isolation between two customers; removal leaves nothing. |
| 7. One-click services | PostgreSQL and ClickHouse, then Coolify and our products (Nomen, Wardenclyffe, ShippinTech, Devagotchi), deploy onto a customer's VM or the provider's capacity, guided or advanced, through one executor. | Provision, health, credentials, update, backup, restore and removal for each entry separately. |
| 8. Terminal anyone can use | The plain-language layer over the real shell, on workspaces and customer VMs. | A non-engineer completes set jobs (start, logs, deploy, back up) without typing a command; destructive actions are confirmed; the shell stays reachable. |
| 9. Workspace AI and sources | Activate one's own Claude subscription, authorize Drive content, dispatch work, reconnect remotely. | Owner-runner isolation, revocation, task recovery, real sessions. |
| 10. DevPulse sync | The client API and per-device access. DevPulse signs in, lists projects, syncs files both ways, and opens a session. | Two-way sync with a forced conflict; a revoked device is refused at once. |
| 11. Build and publish | Deploy from Git; publish an HTTPS address; connect a custom domain; manage the live app. Build runners (RUN-01): an organization connects GitHub, and its workflow jobs run on single-use runner VMs within its plan's limits. | Route, TLS and DNS verified; safe cutover and rollback. Two organizations' jobs run at once on separate runner VMs, neither sees the other's, a job over the plan's limit waits, and every runner is gone after its job. |
| Launch | Invite friends and family onto our own hosts. | The whole acceptance journey, run end to end on the review deploy and then on our hosts, with evidence. |

### Milestone 2: commercial cloud

Stage 12's drivers are the panel's; customer cloud accounts and stages 13 and 14 are enterprise work.

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
