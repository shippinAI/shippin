# What Shippin is

Shippin is the software for running a commercial cloud provider. One installation
runs the whole business:
- the servers and cloud accounts the workloads run on;
- the customers who sign up;
- what they buy: virtual machines, one-click services, development spaces and published apps;
- the support coverage that comes with each purchase;
- the people who operate it.

It is built for customers who are not infrastructure engineers. Everything a
customer does is plain language and guided, with the real machine one click away.

We run Shippin as a cloud business. Friends and family come first, then paying
customers. Anyone else can install the same public code and run their own cloud
with it. Nothing about our installation is built in. Our hosts, domains and
products are configuration in our own database.

This document is the contract. [The roadmap](roadmap.md) says what exists today
and the order of the rest. Requirement IDs such as CFG-01 are stable; plans,
commits and evidence cite them.

## The business

**PRV-01 A commercial cloud, end to end.** An installation is a cloud provider. It
offers what commercial providers offer:
- sign-up, with an invitation, a waitlist or open, as the operator chooses;
- organizations, members, roles and API tokens;
- a catalog of things to buy: machines, services, development spaces and addresses;
- regions to place them in;
- quotas and usage;
- support coverage;
- an audit trail.

The panel, the API and the command-line tools offer the same actions. A customer
never needs to know which hypervisor or cloud holds their workload unless their
plan exposes it.

**PRV-02 Mixed infrastructure behind one control plane.** Workloads run on any mix
of three kinds of capacity, all seen and managed in the same panel:
- the provider's own servers (Incus and Proxmox, in any combination);
- capacity rented from public clouds: AWS, Google Cloud, Azure, DigitalOcean and Fly.io;
- servers a customer brings, from bare metal to a VM in a cloud account of their own.

Who owns a resource is separate from where it runs (INF-01).

**PRV-03 From self-serve to managed.** Small customers serve themselves. Larger
ones buy coverage up to *Managed*: we operate their workloads remotely, deploy
custom builds for them, consult, and train their team. Coverage tiers sit beside
plans ([plans](plans.md#coverage)), and both are rows the operator edits in the
panel.

**PRV-04 What it is not.** Shippin is not a panel for one site's servers, and not
a wrapper around one cloud. It competes where cloud development environments
(Coder, Codespaces, Gitpod) meet app platforms (Coolify, Render, Railway): it gives
people who are not engineers what those give engineering teams, on infrastructure
the provider runs or the customer brings.

**LAUNCH-01 Friends and family first.** The first customers are people the
operator invites. An invitation names the plan and coverage. It creates the
person's account and their own organization on that plan, and the panel shows who
accepted. Sign-up stays invite-only until the operator opens a waitlist or public
sign-up in the panel. Early customers get the same isolation, quotas, backups,
data export and removal as paying ones. Nothing is relaxed because they are friends.

## What a customer gets

**VM-01 Their own machine.** A customer orders a virtual machine from their plan:
- they pick a size, an image and a region, and the provider decides which host or cloud holds it;
- it gets a console, the friendly terminal (TERM-01), power actions, snapshots, rebuild and removal;
- each organization's workloads run inside a VM boundary, never directly on a shared host kernel ([VM-first tenants](reference/vm-first-tenants.md)).

**CAT-01 One-click services onto that machine.** Picking a catalog entry deploys it
onto the customer's VM, or onto the provider's capacity, in a few guided steps.
This is the pattern of DigitalOcean's 1-Click Apps and Coolify.
- PostgreSQL and ClickHouse come first.
- Then Coolify, Nomen (identity), Wardenclyffe, Vaultix (secrets), ShippinTech,
  Devagotchi, Dragonfly, and a growing list of open-source applications.
- Our own products are catalog entries like any other, with no special code path.
- Entries are data: they are added and updated without changing the panel's code.

**TERM-01 A terminal anyone can use.** Every machine and development space has a
real shell in the browser that reconnects where the customer left it. Over it is
a plain-language layer:
- say what you want and see the exact commands before anything runs;
- errors and output explained in plain words;
- guided recipes for common jobs;
- one-press actions for start, logs, deploy, preview and back up;
- a confirmation before anything destructive.

The layer never hides the shell, and an engineer can ignore it. AI help runs on the
customer's own connection in their own runner (AI-01).

**SYNC-01 Work locally against the cloud.** Shippin publishes a client API, so a
desktop app can sign in with a device code and do this:
- list the customer's organizations, workspaces and projects;
- sync a project's files both ways, with conflicts shown, not overwritten;
- open a terminal or session in the cloud workspace;
- follow its runs, previews and deployments.

DevPulse is the first such client. The API is in the [OpenAPI contract](../api/openapi/)
and open to any client. Each device's access is scoped, listed in the panel and
revocable.

**Development spaces, AI and publishing:** AI-01 to AI-03, EXT-01, PUB-01 to
PUB-03 and NET-01 below.

## Rules for every change

**CFG-01 Everything happens in the panel.** Anyone allowed to can do every action,
and enter or change every input and setting, in the panel. The bootstrap file
holds only what the panel needs before it can reach its own database:
- the PostgreSQL, ClickHouse and cache addresses and credentials;
- the session key;
- the listen address and TLS files.

Everything else is stored in PostgreSQL. It is validated before it is saved, takes
effect without a restart, and goes in the audit log with who, when and why. A
setting without a panel form is unfinished work.

**ENG-01 Nothing is hardcoded.** No installation's hosts, addresses, domains,
tenants, credentials or product URLs appear in code, defaults, migrations or tests.
Choices such as image servers, base domains, regions and providers are validated
settings. Provider-specific code stays in provider files, and each concern has one
owning package. New tables use the `shippin_` prefix.

**OPEN-01 One public product.** There is no private fork and no per-site
repository. An installation's values are data in its own database. Anything an
installation once scripted by hand becomes a configurable feature of the product.

**GOV-01 Every change carries a reason.** A change to infrastructure, access,
tenants or settings asks for a short note, and the audit log keeps it. Human review
by an outside system such as AgentHub is optional:
- once it is configured in the panel, Shippin enforces it;
- until then, permitted people act directly and the audit records it.

No site's policy is built in.

**MCPL-01 One canonical action surface.** Every action a person can take is described
once ([MCPL](mcpl.md)): ID, route, access, risk, reason and summary. Routes, inputs,
outputs and errors follow one set of rules. A person in the panel, the API and an AI
acting for that person after a natural chat all run the same handler, permission
check and audit. There is no second API for models. Plain code does the mechanical
work so that a model's tokens go to choices. `TestMCPLConformance` enforces this,
and its lists of gaps only shrink.

**UI-01 One panel.** `web/` is the only panel. Every destination is within four
actions. Its look, accessibility and speed follow the
[design system](design-system.md) (UI-02, UI-03, PERF-01).

**Truth.** Implemented, tested, deployed and verified on a real provider are four
different states, and each is reported separately. A form, a manifest or a healthy
container is not a working feature.

## Pre-release

**PRE-01.** Until the first release, there is one review deploy, and it is
overwritten in place: work, overwrite, review.
- Nothing is built for upgrading older builds: no compatibility shims and no rollback machinery.
- CI stays fast: builds, unit tests, panel checks, and the secret and public-tree scans.
- Installers and database tests run by hand when that code changes.
- Code that is not organized by context is rewritten, not worked around; see [the code map](architecture.md).

## What a fresh install gives you

**CORE-01.** One command, or Docker Compose, installs and configures all of this:

| Part | What it holds |
| --- | --- |
| Panel | The Go control plane and the web panel, one process. |
| PostgreSQL | All state: organizations, people, roles, settings, hosts, workspaces, deployments, audit. |
| ClickHouse | Time series and events: host and guest metrics, activity, logs. Dashboards read it. |
| Dragonfly | Short-lived shared counters and caches. Nothing durable. |

**REL-01.** The installer generates every credential, shares no password, and
prints a one-time setup code. **REL-03.** The code creates the built-in setup
account, the internal organization and the branding. Whoever installs, a person
or an AI assistant, finishes setting up as that account. Until the owner takes
it over, every page asks them to:
1. set its password, which signs every other session of it out;
2. create their own owner account.

**REL-02.** From the first release, upgrades keep data and
grants, and backup, restore and rollback are proved, not assumed.

## Who uses it

```text
Installation: one cloud provider
|-- Control plane: the provider's own organization
|   |-- Staff: a primary role and scoped secondary roles, expiring, audited
|   |-- Capacity: Incus and Proxmox hosts, public cloud accounts, customer-brought servers
|   |   `-- Regions, fleets, live resources, placement
|   |-- Catalog, plans, coverage tiers, sign-up and invitations
|   |-- Customers, settings and the audit log
|   `-- Services the provider runs for itself
`-- Customers: each organization is one boundary
    |-- Machines it ordered, and services deployed onto them
    `-- Workspaces, shaped by its plan
        |-- Members, devices, AI connections, external sources, settings
        `-- Projects
            |-- Code, repository and selected source material
            |-- Development spaces, apps, databases and services
            |-- Terminal, tasks, sessions, logs, previews and deployments
            `-- Published addresses, custom domains, usage and history
```

### The control plane

- **OWN-01** Setup binds the internal organization, its identity and its
  infrastructure explicitly.
- **IAM-01** Staff roles are managed in the panel: one primary role plus scoped
  secondary roles, with expiry and revocation. The panel shows each person's
  effective access and where it comes from.
- **IAM-02** Tenant roles give no staff access. Staff roles give no access to
  customer data or personal AI connections. Support access is scoped, temporary
  and audited. The server enforces every action and checks multi-factor sign-in
  on the current session, not just enrollment.
- **IDP-01** People sign in with local accounts, passkeys, or any OIDC provider
  configured in the panel. Nomen is one such provider, not a requirement.

### Customers

- **TEN-01** An organization has one default workspace and may add more. A
  workspace holds many projects. A project keeps its files and runtime, runs,
  previews and reopens after a restart. Every request checks ancestry,
  membership and quotas.
- **SUB-01** A customer's plan shapes its workspace: which features, how much
  capacity, which services and which limits. Plans are created in the panel.
- **INF-02** Customers see only their own usage, limits and resources. Staff access
  to a customer is scoped.

## Infrastructure

- **INF-01** Staff add capacity in the panel and see its real resources in one
  fleet view: Incus and Proxmox hosts side by side, public cloud accounts by region,
  and servers customers bring. Each needs its address, credentials and trust. Staff
  place workloads and allocate capacity. Who owns a resource is separate from where
  it runs. Stale or unknown observations are labelled as such.
- **INF-03** Guests are created, started, stopped, snapshotted and removed from the
  panel. Image servers, templates, storage and networks are provider settings in
  the panel.
- **HYB-01** Each public cloud is a provider driver beside Incus and Proxmox, in
  its own package under `internal/hv/`:
  - AWS EC2, Google Compute Engine, Azure Virtual Machines, DigitalOcean Droplets
    and Fly.io Machines;
  - it creates and manages machines through that cloud's own API, with the account's
    credentials sealed in the database;
  - each reports what it can do, so the panel offers only what the provider supports.

  A cloud account belongs either to the provider or to a customer. A customer's
  account is used only for that customer.
- **BYO-01** A customer or the provider can add a server they already have, such as
  bare metal or a VM elsewhere, by installing the host agent on it. Workloads and
  services deploy onto it the same way, and detaching it never deletes what is on it.
- **OBS-01** Every host and guest has a dashboard with live and historical CPU,
  memory, disk and network. The panel samples each provider on a schedule set in
  the panel, stores the samples in ClickHouse and draws them immediately. Retention
  is a setting. Missing data is shown as missing, never as zero.

## Deployable services

CAT-01 above is the catalog. Each entry also follows these rules:

- **SVC-01** Each entry declares its versions, prerequisites, inputs, dependencies
  and health check.
- **SVC-02** Managed or external, and platform-owned or customer-owned, are always
  explicit. Detaching an external service never deletes it.
- **SVC-03** Each engine's lifecycle, credential rotation, backup and restore is
  verified on its own. Nomen owns identity records, Vaultix owns secrets, Shippin
  owns resource and deployment state.
- **DEP-01** The guided path and the advanced path produce the same validated,
  versioned plan and run through the same executor. Advanced settings cover
  source, dependencies, secrets, placement, capacity, storage, network and
  recovery.
- **DEP-02** A deployment has these steps:
  1. Choose the workspace, project and environment, or the customer's machine.
  2. Choose a template, a Git repository or a custom source.
  3. Configure and review.
  4. Deploy and verify health.
  5. Then operate, update, back up, restore or remove it.

  Source and build provenance, logs and recovery state are kept.

## Workspaces and AI

- **AI-01** A workspace offers *Activate Claude subscription*. It starts the user's
  own supported sign-in to unmodified Claude Code in an isolated runner owned by
  that user, bound to that user and workspace. Subscriptions are never pooled,
  session tokens are never collected centrally, and one user's credentials are
  never mounted into a shared environment.
- **AI-02** Work can be dispatched to an authorized runner, with its queue,
  progress, logs, changed files, result and failure visible, plus cancellation and
  recovery after a restart. Project access, resource limits and revocation are
  enforced, and changes are reviewed before publication. Native subscription
  sessions and API credentials are separate, explicit modes; one never silently
  becomes the other.
- **AI-03** Remote access opens the intended project through a terminal, IDE, SSH
  bridge or Claude remote connection. Access is scoped, expiring and revocable,
  and reconnecting keeps the authorized project state. Vendor eligibility is
  checked at runtime.
- **EXT-01** Google Drive connects through Google authorization:
  - the user selects files or folders and grants them to a workspace or project;
  - Shippin records their IDs, versions, sync state and provenance, and supports revocation;
  - imported material is never public or organization-wide by default.

  Further connectors follow the same model.

## Publishing

- **PUB-01** Publish gives a healthy app an address under one of the
  installation's base domains. Base domains and allowed choices are settings.
  Names are unique, routes are owned, TLS is verified, and preview, private and
  production exposure are explicit.
- **PUB-02** A custom domain is connected in these steps:
  1. Add the domain and verify ownership.
  2. Show the exact DNS records.
  3. Check routing and HTTPS.
  4. Switch the primary address.

  Cloudflare automation is optional and scoped, and manual DNS always works. The old
  address keeps working until the new one is verified.
- **PUB-03** Each project shows its deployment versions, preview and production
  state, addresses, logs, health, resources, scaling and rollback.
- **NET-01** Cloudflare Tunnel, Tailscale and custom connectivity are configured in
  the panel, scoped, and verified against real routes.

## Selling it

- **PLAN-01** Hosting plans (capacity, services, features) are separate from
  AI-provider subscriptions. A plan never promises unlimited AI usage. Prices and
  billing policy are settings in the panel. A plan works before it has a price.
- **COV-01** Coverage tiers (support hours, response targets, who operates the
  workloads, consulting, custom deployments, remote operations, training and the
  members' sign-in rule) are set beside plans in the panel. Each plan includes one,
  and a customer can be moved to a higher one.
- **BILL-01** Metering of what each customer uses comes before invoicing. Payment
  collection is a later integration and is not needed for the friends-and-family
  launch.
- Self-hosting is open (OPEN-01). Revenue comes from our own cloud, its coverage,
  and the consulting, custom deployments and training at the managed end.

## Engineering

- **SRC-01** Capabilities from older Shippin, Warden and Proxmox panels are absorbed
  by re-authoring them as configurable features under their owning package, with
  provenance, a security review and tests. Nothing is bulk-copied, and user work is
  preserved. The [absorption ledger](absorption/capability-absorption.md) tracks it.
- **ENG-02** Input is validated the same way in the panel and on the server. Every
  API route is in the [OpenAPI contract](../api/openapi/). Checks in
  [CONTRIBUTING.md](../CONTRIBUTING.md) run before every push.

## Acceptance journey: friends and family

The launch is accepted when one person can do all of this on a clean install, with
nothing done outside the panel:

**Operator**
1. Install Shippin.
2. Add an Incus host and a Proxmox host, and see both in one fleet view with live and historical resources.
3. Create a plan with its coverage.
4. Invite a friend onto it.

**Friend**
1. Accept the invitation and land in their own organization.
2. Order a VM.
3. Deploy PostgreSQL and one of our products onto it with one click.
4. Open the friendly terminal and do a job in plain language.
5. Activate their own Claude connection in a workspace.
6. Sync a project with DevPulse.
7. Publish an HTTPS address and connect a custom domain.

**Operator**
1. Revoke the friend's devices.
2. Export the friend's data and remove the organization.
3. Prove that nothing of theirs is left and that no other customer could see it.

Each step needs the panel and API to persist it, real execution, failure paths and
recorded evidence. Public clouds (HYB-01) join the journey after the launch.

Provider constraints checked 2026-11-07:
- [Claude hosting and authentication](https://code.claude.com/docs/en/legal-and-compliance)
- [Remote Control](https://code.claude.com/docs/en/remote-control)
- [Drive scopes](https://developers.google.com/workspace/drive/api/guides/api-specific-auth)

They inform the target; they do not prove Shippin's integration works.
