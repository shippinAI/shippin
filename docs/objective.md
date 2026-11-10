# What Shippin is

Shippin is an open cloud panel for your own servers. You install it once, add your
Incus and Proxmox hosts in the panel, and run a private cloud on them. The people
who run the infrastructure work in the **control plane**. Organizations that use it
are **tenants**: each gets workspaces shaped by its subscription, with projects,
apps and databases running on your hosts.

This public repository is the whole product. Anyone can install it and run it
privately. The business is commercial support and a managed, hosted Shippin that
runs this same code. Nothing about any one installation, including our own, is
built into it.

This document is the contract. [The roadmap](roadmap.md) says what exists today
and what comes next. Requirement IDs such as CFG-01 are stable; plans, commits and
evidence cite them.

## Rules for every change

**CFG-01 Everything happens in the panel.** Every action, every input and every
setting can be done and changed in the panel by someone allowed to. The bootstrap
file holds only what the panel needs before it can reach its own database: the
PostgreSQL, ClickHouse and cache addresses and credentials, the session key, the
listen address and TLS files. Everything else is stored in PostgreSQL, validated
before it is saved, takes effect without a restart, and is written to the audit
log with who, when and why. A setting without a panel form is unfinished work.

**ENG-01 Nothing is hardcoded.** No installation's hosts, addresses, domains,
tenants, credentials or product URLs appear in code, defaults, migrations or tests.
Choices such as image servers, base domains and providers are validated settings.
Provider-specific code stays in provider files, each concern has one owning
package, and new tables use the `shippin_` prefix.

**OPEN-01 One public product.** There is no private fork and no per-site
repository. An installation's values are data in its own database. Anything an
installation once scripted by hand becomes a configurable feature of the product.

**GOV-01 Every change carries a reason.** A change to infrastructure, access,
tenants or settings asks for a short note, and the audit log keeps it. Human review
by an outside system such as AgentHub is optional: once it is configured in the
panel, Shippin enforces it; until then, permitted people act directly and the
audit records it. No site's policy is built in.

**UI-01 One panel.** `web/` is the only panel. Every destination is within four
actions. Its look, accessibility and speed follow the
[design system](design-system.md) (UI-02, UI-03, PERF-01).

**Truth.** Implemented, tested, deployed and verified on a real provider are four
different states and are reported separately. A form, a manifest or a healthy
container is not a working feature.

## What a fresh install gives you

**CORE-01.** One command, or Docker Compose, installs and configures all of this:

| Part | What it holds |
| --- | --- |
| Panel | The Go control plane and the web panel, one process. |
| PostgreSQL | All state: organizations, people, roles, settings, hosts, workspaces, deployments, audit. |
| ClickHouse | Time series and events: host and guest metrics, activity, logs. Dashboards read it. |
| Dragonfly | Short-lived shared counters and caches. Nothing durable. |

**REL-01.** The installer generates every credential, shares no password, and
prints a one-time setup code. Setup creates the owner, the internal organization
and the branding. **REL-02.** Upgrades keep data and grants; backup, restore and
rollback are proved, not assumed.

## Who uses it

```text
Installation
|-- Control plane: the internal operator organization
|   |-- Staff: a primary role and scoped secondary roles, expiring, audited
|   |-- Infrastructure: providers, hosts, fleets, capacity, placement
|   |-- Plans, subscriptions, tenants, settings and the audit log
|   `-- Services the installation runs for itself
`-- Tenants: each organization is one boundary
    `-- Workspaces, shaped by the tenant's subscription
        |-- Members, AI connections, external sources, settings
        `-- Projects
            |-- Code, repository and selected source material
            |-- Devspaces, apps, databases and services
            |-- Tasks, sessions, logs, previews and deployments
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

### Tenants

- **TEN-01** An organization has one default workspace and may add more. A
  workspace holds many projects. A project keeps its files and runtime, runs,
  previews and reopens after a restart. Every request checks ancestry,
  membership and quotas.
- **SUB-01** A tenant's subscription shapes its workspace: which features, how
  much capacity, which services and which limits. Plans are created and priced in
  the panel.
- **INF-02** Tenants see only their own usage, limits and resources. Staff access
  to a tenant is scoped.

## Infrastructure

- **INF-01** Staff add Incus and Proxmox hosts in the panel, with address,
  credentials and trust, and see real capacity. They place tenant workloads and
  allocate capacity. Who owns a resource is separate from where it runs. Stale or
  unknown observations are labelled as such.
- **INF-03** Guests are created, started, stopped, snapshotted and removed from the
  panel. Image servers, templates, storage and networks are provider settings in
  the panel.
- **OBS-01** Every host and guest has a dashboard with live and historical CPU,
  memory, disk and network. The panel samples each provider on a schedule set in
  the panel, stores the samples in ClickHouse and draws them immediately. Retention
  is a setting. Missing data is shown as missing, never as zero.

## Deployable services

- **CAT-01** A catalog of turnkey services that deploy onto the installation's
  hosts in a few steps. PostgreSQL and ClickHouse come first; then Nomen
  (identity), Wardenclyffe, Vaultix (secrets), Dragonfly, and a growing list of
  open-source community applications. Catalog entries are data: they are added
  and updated without changing the panel's code.
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
- **DEP-02** A deployment goes: choose workspace, project and environment; choose
  a template, a Git repository or a custom source; configure; review; deploy;
  verify health; then operate, update, back up, restore or remove. Source and build
  provenance, logs and recovery state are kept.

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
- **EXT-01** Google Drive connects through Google authorization. The user selects
  files or folders and grants them to a workspace or project; Shippin records
  their IDs, versions, sync state and provenance, and supports revocation.
  Imported material is never public or organization-wide by default. Further
  connectors follow the same model.

## Publishing

- **PUB-01** Publish gives a healthy app an address under one of the
  installation's base domains. Base domains and allowed choices are settings.
  Names are unique, routes are owned, TLS is verified, and preview, private and
  production exposure are explicit.
- **PUB-02** A custom domain goes: add domain, verify ownership, show the exact DNS
  records, check routing and HTTPS, then switch the primary address. Cloudflare
  automation is optional and scoped; manual DNS always works. The old address keeps
  working until the new one is verified.
- **PUB-03** Each project shows its deployment versions, preview and production
  state, addresses, logs, health, resources, scaling and rollback.
- **NET-01** Cloudflare Tunnel, Tailscale and custom connectivity are configured in
  the panel, scoped, and verified against real routes.

## Selling it

- **PLAN-01** Hosting plans (capacity, services, features) are separate from
  AI-provider subscriptions. A plan never promises unlimited AI usage. Prices and
  billing policy are set in the panel.
- Self-hosting is open. Revenue comes from commercial support and the managed,
  hosted Shippin (OPEN-01).

## Engineering

- **SRC-01** Capabilities from older Shippin, Warden and Proxmox panels are absorbed
  by re-authoring them as configurable features under their owning package, with
  provenance, a security review and tests. Nothing is bulk-copied, and user work is
  preserved. The [absorption ledger](absorption/capability-absorption.md) tracks it.
- **ENG-02** Input is validated the same way in the panel and on the server. Every
  API route is in the [OpenAPI contract](../api/openapi/). Checks in
  [CONTRIBUTING.md](../CONTRIBUTING.md) run before every push.

## Acceptance journey

Install on a clean machine; add an Incus host and a Proxmox host; see their live
dashboards; create a plan and a tenant; open its default workspace and two real
projects; deploy PostgreSQL and ClickHouse for it; activate the user's own Claude
connection; authorize selected Drive content; dispatch work and reconnect
remotely; run and preview; publish an HTTPS address; verify a custom domain;
deploy an update; recover a failed update; revoke connections and prove isolation.
Each step needs the panel and API to persist it, real execution, failure paths
and recorded evidence.

Provider constraints checked 2026-11-07:
[Claude hosting and authentication](https://code.claude.com/docs/en/legal-and-compliance),
[Remote Control](https://code.claude.com/docs/en/remote-control),
[Drive scopes](https://developers.google.com/workspace/drive/api/guides/api-specific-auth).
They inform the target; they do not prove Shippin's integration works.
