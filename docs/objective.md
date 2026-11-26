# What Shippin is

Shippin is a self-hosted control panel. One installation runs on a server its owner already has, and from there manages:
- their Incus and Proxmox hosts;
- the virtual machines, apps and services on them;
- the development workspaces their team builds in;
- the people who operate it.

It is built for people who are not infrastructure engineers. Everything is plain language and guided, with the real machine one click away.

There are two editions ([Editions and licensing](editions.md)):

- **The panel** is this public repository. Anyone can install it and run their own infrastructure with it. Nothing about any installation is built in: hosts, domains and products are configuration in that installation's own database.
- **The enterprise edition** is a private module loaded into the panel. It adds what it takes to run a multi-tenant hosted cloud and sell it: customers, plans, coverage, sign-up, isolation between customers and the hosted development space.

This document is the contract for both. Part one is the panel. Part two is the enterprise module, which builds on part one and never changes it. [The roadmap](roadmap.md) says what exists today and the order of the rest. Requirement IDs such as CFG-01 are stable; plans, commits and evidence cite them. A requirement with a part in each edition is defined in part one and extended in part two.

# Part one: the panel

## What the owner gets

**VM-01 Machines on their own hosts.** The owner creates virtual machines and containers on any host added to the panel:
- they pick a host, a size and an image;
- each one gets a console, the friendly terminal (TERM-01), power actions, snapshots, rebuild and removal.

**CAT-01 One-click services.** Picking a catalog entry deploys it onto one of the owner's machines or hosts, in a few guided steps. This is the pattern of DigitalOcean's 1-Click Apps and Coolify.
- PostgreSQL and ClickHouse come first.
- Then a growing list of open-source applications. Products such as Nomen (identity), Vaultix (secrets) and Coolify are catalog entries like any other, with no special code path.
- Entries are data: they are added and updated without changing the panel's code.

**TERM-01 A terminal anyone can use.** Every machine and development workspace has a real shell in the browser that reconnects where the person left it. Over it is a plain-language layer:
- say what you want and see the exact commands before anything runs;
- errors and output explained in plain words;
- guided recipes for common jobs;
- one-press actions for start, logs, deploy, preview and back up;
- a confirmation before anything destructive.

The layer never hides the shell, and an engineer can ignore it. AI help runs on the person's own connection in their own runner (AI-01).

**SYNC-01 Work locally against the panel.** Shippin publishes a client API, so a desktop app can sign in with a device code and do this:
- list the person's organizations, workspaces and projects;
- sync a project's files both ways, with conflicts shown, not overwritten;
- open a terminal or session in a workspace;
- follow its runs, previews and deployments.

DevPulse is the first such client. The API is in the [OpenAPI contract](../api/openapi/) and open to any client. Each device's access is scoped, listed in the panel and revocable.

## Development workspaces

**SOLO-01 Usable by one person now.** A single developer installs the panel on their own server and builds in a workspace on it. The panel runs for solo use before it runs for teams: what a solo developer needs works end to end, and what they don't need stays out of their way.

**WS-01 A workspace on your own host.** A development workspace is a container on one of the installation's hosts, created through that host's own API (Incus first, then Proxmox). It needs nothing else installed on the host. It starts from a template, has its own terminal, editor and previews, and reaches only itself. Who may open it is checked on every connection.

**IDE-01 One focused workspace.** A workspace opens as one screen that brings together:
- a customized VS Code;
- tailored Zellij and WezTerm sessions, limited to the workspace;
- the person's AI apps, shown inside it.

It shows only what matters to the step the person is on, and brings the rest into focus when it is used. Nothing is hidden: the full editor and the real shell are one action away.

**GUIDE-01 Behind the scenes, made visible.** Interactive explanations and motion show what is happening as it happens:
- the files an AI changed;
- the commands that ran;
- a build, a deploy, a preview.

How much is explained is a setting each person chooses, from quiet to step by step. The explanations are driven by the same events as the actions (MCPL-01), so what they show is what actually ran.

**COACH-01 A specialist that teaches.** A dedicated specialist (a mixture of expert models) follows a person's events: prompt submissions, errors, builds and deploys.
- From them it judges the person's skill and level, responds when a trigger calls for it, and adjusts how much it explains.
- It keeps notes on what the person has learned and where they struggle.
- Those notes belong to the person: they can read, correct, export and delete them.
- Nothing about their subscriptions or credentials is part of the notes.

## Rules for every change

**CFG-01 Everything happens in the panel.** Anyone allowed to can do every action, and enter or change every input and setting, in the panel. The bootstrap file holds only what the panel needs before it can reach its own database:
- the PostgreSQL, ClickHouse and cache addresses and credentials;
- the session key;
- the listen address and TLS files.

Everything else is stored in PostgreSQL, and:
- it is validated before it is saved;
- it takes effect without a restart;
- it goes in the audit log with who, when and why.

A setting without a panel form is unfinished work. Keys entered in the panel are saved to the installation's secret store, its Vaultix account connected in the panel with a machine identity, and never shown again ([design](settings.md)).

**ENG-01 Nothing is hardcoded.** No installation's hosts, addresses, domains, organizations, credentials or product URLs appear in code, defaults, migrations or tests.
- Choices such as image servers, base domains, regions and providers are validated settings.
- Provider-specific code stays in provider files, and each concern has one owning package.
- New tables use the `shippin_` prefix.

**OPEN-01 One public panel, one enterprise module.** This repository is the panel, published under FSL-1.1-ALv2: anyone may use and self-host it, except as a competing commercial service.
- There is no fork and no per-site repository.
- The enterprise edition is a separate private module that extends the panel only through its extension points ([Editions and licensing](editions.md)). It never copies or patches panel code. It is unlocked by a signed license key and runs managed or unmanaged.
- Multi-tenant and commercial code (customers, plans, coverage, sign-up, isolation between customers, billing) belongs to the module, never to this repository.
- An installation's values are data in its own database. Anything an installation once scripted by hand becomes a configurable feature of the product.

**GOV-01 Every change carries a reason.** A change to infrastructure, access, organizations or settings asks for a short note, and the audit log keeps it. Human review by an outside system such as AgentHub is optional:
- once it is configured in the panel, Shippin enforces it;
- until then, permitted people act directly and the audit records it.

No site's policy is built in.

**MCPL-01 One canonical action surface.** Every action a person can take is described once, as a file whose folder gives its ID and route ([MCPL](mcpl.md)).
- Routes, the registry and the panel's action map are generated; none is written by hand. The enterprise module's actions are generated the same way and merged in.
- Inputs, outputs, reasons, audit and errors are handled the same way for every action.
- A person in the panel, the API, and an AI acting for that person after a natural chat all run the same handler, permission check and audit. There is no second API for models.
- Plain code does the mechanical work, so that a model's tokens go to choices.

`TestMCPLConformance` enforces this, and its lists of gaps only shrink.

**UI-01 One panel.** `web/` is the only panel. Every destination is within four actions. Its look, accessibility and speed follow the [design system](design-system.md) (UI-02, UI-03, PERF-01).

**Truth.** Implemented, tested, deployed and verified on a real provider are four different states, and each is reported separately. A form, a manifest or a healthy container is not a working feature.

## Pre-release

**PRE-01.** Until the first release, there is one review deploy, and it is overwritten in place: work, overwrite, review.
- Nothing is built for upgrading older builds: no compatibility shims and no rollback machinery. The database schema may be re-based.
- A broken build is never rolled back. The deploy stays on the newest build, the error stays visible, and the fix is made in the source and deployed over it. No snapshot is taken to return to, and no earlier build is restored.
- CI runs the build, unit tests, the database tests against PostgreSQL 17, panel checks, and the secret and public-tree scans. The database tests run in the Checks workflow the Gate requires before main moves; a run that expected a database and did not have one fails instead of skipping them.
- Installers and ClickHouse tests run by hand when that code changes.
- Code that is not organized by context is rewritten, not worked around; see [the code map](architecture.md).

## What a fresh install gives you

**CORE-01.** One command, or Docker Compose, installs and configures all of this, on a plain Debian or Ubuntu machine or inside a guest on an Incus or Proxmox host:

| Part | What it holds |
| --- | --- |
| Panel | The Go control plane and the web panel, one process. |
| PostgreSQL | All state: organizations, people, roles, settings, hosts, workspaces, deployments, audit. |
| ClickHouse | Time series and events: host and guest metrics, activity, logs. Dashboards read it. |
| Dragonfly | Short-lived shared counters and caches. Nothing durable. |

**REL-01.** The installer generates every credential and shares no password. It prints no setup code: installing needs no step in a terminal beyond running it.

**REL-03.** The installation is owned by a person from the start, with no setup code (the owner's decision, 2026-11-25, accepting the risk below for a simple install). Either:
1. the installation names its owner up front (`SHIPPIN_OWNER_EMAIL` with `SHIPPIN_OWNER_PASSWORD` or `SHIPPIN_OWNER_PASSWORD_FILE`), and the panel creates that owner when it first starts, so nobody has to open it to finish; or
2. while no owner exists, the first person to open the panel creates the owner account, with the installation's organization, and is signed in and shown their recovery codes once.

The first claim wins and every later one is refused; the claim and its address are audited. The accepted risk: anyone who reaches the panel's address before its owner does can claim it. The first way leaves no such window.

**REL-02.** From the first release, upgrades keep data and grants, and backup, restore and rollback are proved, not assumed.

## Who uses it

```text
Installation: one owner
|-- The installation's organization
|   |-- Owner and staff: a primary role and scoped secondary roles, expiring, audited
|   |-- Hosts: Incus and Proxmox, and servers the owner brings
|   |   `-- Fleets, guests, storage, networks, metrics
|   |-- Catalog, templates, settings and the audit log
|   `-- Services the installation runs for itself
`-- Organizations: the installation's own, and any the owner adds for their teams
    `-- Workspaces
        |-- Members, devices, AI connections, external sources, settings
        `-- Projects
            |-- Code, repository and selected source material
            |-- Development workspaces, apps, databases and services
            |-- Terminal, tasks, sessions, logs, previews and deployments
            `-- Published addresses, custom domains and history
```

### The control plane

- **OWN-01** Setup binds the installation's organization, its identity and its infrastructure explicitly.
- **IAM-01** Staff roles are managed in the panel: one primary role plus scoped secondary roles, with expiry and revocation. The panel shows each person's effective access and where it comes from.
- **IAM-02** Organization roles give no staff access. Staff roles give no access to personal AI connections. The server enforces every action, and checks multi-factor sign-in on the current session, not just enrollment.
- **IDP-01** People sign in with local accounts, passkeys, or any OIDC provider configured in the panel. Nomen is one such provider, not a requirement.

### Organizations

- **TEN-01** An organization has one default workspace and may add more.
  - A workspace holds many projects.
  - A project keeps its files and runtime, runs, previews, and reopens after a restart.
  - Every request checks ancestry and membership.

## Infrastructure

- **INF-01** Staff add hosts in the panel and see their real resources in one fleet view, with Incus and Proxmox hosts side by side.
  - Each host needs its address, credentials and trust.
  - Staff place workloads on them.
  - Stale or unknown observations are labelled as such.
- **INF-03** Guests are created, started, stopped, snapshotted and removed from the panel. Image servers, templates, storage and networks are provider settings in the panel.
- **HYB-01** Each public cloud is a provider driver beside Incus and Proxmox, in its own package under `internal/hv/`:
  - AWS EC2, Google Compute Engine, Azure Virtual Machines, DigitalOcean Droplets and Fly.io Machines;
  - it creates and manages machines through that cloud's own API, with the account's credentials sealed in the database;
  - each reports what it can do, so the panel offers only what the provider supports.
- **BYO-01** The owner can add a server they already have, such as bare metal or a VM elsewhere, by installing the host agent on it from a command the panel gives them. Workloads and services deploy onto it the same way. Detaching it never deletes what is on it.
- **OBS-01** Every host and guest has a dashboard with live and historical CPU, memory, disk and network.
  - The panel samples each provider on a schedule set in the panel, stores the samples in ClickHouse and draws them immediately.
  - Retention is a setting.
  - Missing data is shown as missing, never as zero.

## Deployable services

CAT-01 above is the catalog. Each entry also follows these rules:

- **SVC-01** Each entry declares its versions, prerequisites, inputs, dependencies and health check.
- **SVC-02** Managed or external is always explicit. Detaching an external service never deletes it.
- **SVC-03** Each engine's lifecycle, credential rotation, backup and restore is verified on its own. Nomen owns identity records, Vaultix owns secrets, and Shippin owns resource and deployment state.
- **DEP-01** The guided path and the advanced path produce the same validated, versioned plan, and run through the same executor. Advanced settings cover source, dependencies, secrets, placement, capacity, storage, network and recovery.
- **DEP-02** A deployment has these steps:
  1. Choose the workspace, project and environment, or a machine.
  2. Choose a template, a Git repository or a custom source.
  3. Configure and review.
  4. Deploy and verify health.
  5. Then operate, update, back up, restore or remove it.

  Source and build provenance, logs and recovery state are kept.

### Build runners

- **RUN-01** Build runners for the installation's GitHub repositories:
  - An organization connects its GitHub account or organization to Shippin, and assigns repositories to its workspaces and projects. Disconnecting removes Shippin's access and never touches the repositories.
  - A workflow job that asks for a Shippin runner by size gets a fresh machine for that job alone, on the installation's hosts. It is removed when the job ends. One workspace's jobs never see another's files or secrets.
  - Sizes, how many jobs run at once and the time limit are settings in the panel.
  - Runner machine images are provider settings in the panel, versioned, with their provenance kept.
  - A self-hosted runner can also be deployed from the catalog (CAT-01).

  Every operation is an MCPL action, and GitHub's events arrive at one declared route that is checked before anything is read. The design is in [Build runners](runners.md).

## Workspaces and AI

- **AI-01** A workspace offers *Activate Claude subscription*. It starts the user's own supported sign-in to unmodified Claude Code, in an isolated runner owned by that user and bound to that user and workspace.
  - Subscriptions are never pooled.
  - Session tokens are never collected centrally.
  - One user's credentials are never mounted into a shared environment.
- **AI-02** Work can be dispatched to an authorized runner.
  - Its queue, progress, logs, changed files, result and failure are visible, with cancellation and recovery after a restart.
  - Project access, resource limits and revocation are enforced, and changes are reviewed before publication.
  - Native subscription sessions and API credentials are separate, explicit modes; one never silently becomes the other.
- **AI-03** Remote access opens the intended project through a terminal, IDE, SSH bridge or Claude remote connection. Access is scoped, expiring and revocable, and reconnecting keeps the authorized project state. Vendor eligibility is checked at runtime.
- **EXT-01** Google Drive connects through Google authorization:
  - the user selects files or folders and grants them to a workspace or project;
  - Shippin records their IDs, versions, sync state and provenance, and supports revocation;
  - imported material is never public or organization-wide by default.

  Further connectors follow the same model.

## Publishing

- **PUB-01** Publish gives a healthy app an address under one of the installation's base domains. Base domains and allowed choices are settings. Names are unique, routes are owned, TLS is verified, and preview, private and production exposure are explicit.
- **PUB-02** A custom domain is connected in these steps:
  1. Add the domain and verify ownership.
  2. Show the exact DNS records.
  3. Check routing and HTTPS.
  4. Switch the primary address.

  Cloudflare automation is optional and scoped, and manual DNS always works. The old address keeps working until the new one is verified.
- **PUB-03** Each project shows its deployment versions, preview and production state, addresses, logs, health, resources, scaling and rollback.
- **NET-01** Cloudflare Tunnel, Tailscale and custom connectivity are configured in the panel, scoped, and verified against real routes.

## Engineering

- **SRC-01** Capabilities from older Shippin, Warden and Proxmox panels are absorbed by re-authoring them as configurable features under their owning package, with provenance, a security review and tests. Nothing is bulk-copied, and user work is preserved. The [absorption ledger](absorption/capability-absorption.md) tracks it.
- **ENG-02** Input is validated the same way in the panel and on the server. Every API route is in the [OpenAPI contract](../api/openapi/). Checks in [CONTRIBUTING.md](../CONTRIBUTING.md) run before every push.

## Acceptance journey: self-hosting

The panel is accepted when one person can do all of this on a clean install, with nothing done outside the panel:

1. Install Shippin inside a guest on an Incus host, and separately on a Proxmox host.
2. Add both hosts, and see them in one fleet view with live and historical resources.
3. Create a VM on each, and deploy PostgreSQL onto one with one click.
4. Invite a teammate.
5. Create a workspace on their own host, open its terminal, and do a job in plain language.
6. Activate their own Claude connection in the workspace.
7. Sync a project with DevPulse.
8. Publish an HTTPS address and connect a custom domain.
9. Revoke the teammate's devices and remove their access, and prove they can no longer reach anything.

Each step needs the panel and API to persist it, real execution, failure paths and recorded evidence.

# Part two: the enterprise module

Everything here is built in `shippinAI/shippin-enterprise` and plugged into the panel through its extension points ([Editions and licensing](editions.md)). None of it is in this repository.

## The business

**PRV-01 A commercial cloud, end to end.** An enterprise installation is a cloud provider. It offers what commercial providers offer:
- sign-up, with an invitation, a waitlist or open, as the operator chooses;
- customer organizations, members, roles and API tokens;
- a catalog of things to buy: machines, services, development spaces and addresses;
- regions to place them in;
- quotas and usage;
- support coverage;
- an audit trail.

The panel, the API and the command-line tools offer the same actions. A customer never needs to know which hypervisor or cloud holds their workload, unless their plan exposes it.

**PRV-02 Mixed infrastructure behind one control plane.** Workloads run on any mix of three kinds of capacity, all seen and managed in the same panel:
- the provider's own servers (Incus and Proxmox, in any combination);
- capacity rented from public clouds (HYB-01);
- servers a customer brings, from bare metal to a VM in a cloud account of their own.

Who owns a resource is separate from where it runs.

**PRV-03 From self-serve to managed.** Small customers serve themselves. Larger ones buy coverage up to *Managed*: the provider operates their workloads remotely, deploys custom builds for them, consults, and trains their team. Coverage tiers sit beside plans, and both are rows the operator edits in the panel.

**PRV-04 What it is.** The enterprise edition competes where cloud development environments (Coder, Codespaces, Gitpod) meet app platforms (Render, Railway): it gives people who are not engineers what those give engineering teams, on infrastructure the provider runs or the customer brings.

**LAUNCH-01 Friends and family first.** The first customers are people the operator invites.
- An invitation names the plan and coverage. It creates the person's account and their own organization on that plan, and the panel shows who accepted.
- Sign-up stays invite-only until the operator opens a waitlist or public sign-up in the panel.
- Early customers get the same isolation, quotas, backups, data export and removal as paying ones. Nothing is relaxed because they are friends.

## What a customer gets

- **VM-01, for customers.** A customer orders a machine from their plan: they pick a size, an image and a region, and the provider decides which host or cloud holds it. Each customer's workloads run inside a VM boundary, never directly on a shared host kernel ([VM-first tenants](reference/vm-first-tenants.md)).
- **DEV-01 Build with your own subscriptions, in a space the provider hosts.** A customer signs up and gets a development space hosted for them.
  - They bring the AI they already pay for (Claude, ChatGPT and others), under AI-01: their sign-in, their runner, never pooled or collected.
  - They describe what they want, see it built, preview it and publish it (PUB-01).
  - The space is sized by their plan and isolated from every other customer: encrypted storage, a private network and its own boundary.
- **SUB-01** A customer's plan shapes their space: which features, how much capacity, which services and which limits. Plans are created in the panel.
- **INF-02** Customers see only their own usage, limits and resources.
- **TEN-01, for customers.** Every request also checks the customer's quotas.
- **RUN-01, for customers.** A runner never serves two customers, and runs inside their VM boundary. The plan sets the sizes offered, how many jobs run at once and the runner minutes included. Usage is recorded per organization, workspace and project, ready for metering. Customers may run jobs on their own servers or cloud accounts.

## Operating customers

- **IAM-02, for customers.** Staff roles give no access to customer data. Support access to a customer is granted by the customer or by break-glass, and it is scoped, temporary and audited. A coverage tier can require a stronger sign-in from a customer's members.
- **INF-01, for customers.** Staff allocate capacity to customers and assign hosts to them. Who owns a resource is separate from where it runs.
- **HYB-01 and BYO-01, for customers.** A cloud account or server belongs either to the provider or to a customer. A customer's own capacity is used only for that customer.
- **SVC-02, for customers.** Platform-owned or customer-owned is always explicit.

## Selling it

- **PLAN-01** Hosting plans (capacity, services, features) are separate from AI-provider subscriptions.
  - A plan never promises unlimited AI usage.
  - Prices and billing policy are settings in the panel.
  - A plan works before it has a price.
- **COV-01** Coverage tiers are set beside plans in the panel. They cover:
  - support hours and response targets;
  - who operates the workloads;
  - consulting, custom deployments, remote operations and training;
  - the members' sign-in rule.

  Each plan includes one, and a customer can be moved to a higher one.
- **BILL-01** Metering of what each customer uses comes before invoicing. Payment collection is a later integration and is not needed for the friends-and-family launch.
- Revenue comes from a hosted cloud run on the enterprise edition, its coverage, enterprise licenses (managed and unmanaged), commercial licenses for competing use, and the consulting, custom deployments and training at the managed end.

## Acceptance journey: friends and family

The enterprise edition is accepted when one person can do all of this on a clean enterprise install, with nothing done outside the panel.

**Operator**
1. Install the enterprise edition.
2. Add an Incus host and a Proxmox host.
3. Create a plan with its coverage.
4. Invite a friend onto it.

**Friend**
1. Accept the invitation and land in their own organization.
2. Order a VM.
3. Deploy PostgreSQL onto it with one click.
4. Open their hosted development space and do a job in plain language.
5. Activate their own Claude connection in it.
6. Publish an HTTPS address.

**Operator**
1. Revoke the friend's devices.
2. Export the friend's data and remove the organization.
3. Prove that nothing of theirs is left, and that no other customer could see it.

Public clouds (HYB-01) join the journey after the launch.

Provider constraints checked 2026-11-07:
- [Claude hosting and authentication](https://code.claude.com/docs/en/legal-and-compliance)
- [Remote Control](https://code.claude.com/docs/en/remote-control)
- [Drive scopes](https://developers.google.com/workspace/drive/api/guides/api-specific-auth)

They inform the target; they do not prove Shippin's integration works.
