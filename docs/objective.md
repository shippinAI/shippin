# Shippin product contract

Reconciled with the owner's requirements on 2026-11-07. This defines the target;
implementation status and execution order belong to [the roadmap](roadmap.md).
Older plans must be reconciled with this contract before coding.

Shippin is a self-hosted cloud development/hosting platform: one Go backend and
React panel in shippin-platform. Provide a Lovable-style build/run/publish/manage
experience using users' own supported AI subscriptions and remote resources.
Operators manage infrastructure and customer services. Branding is configurable.
Shippin Prox names Proxmox capability; Warden names are historical provenance.
## Ownership and hierarchy

```text
Installation
|-- Internal operator organization
|   |-- Staff: primary roles and scoped secondary roles
|   |-- Infrastructure: providers, fleets, nodes, capacity, allocations
|   `-- Platform services: PostgreSQL, Dragonfly, Nomen, Vaultix, ClickHouse...
`-- Tenant organizations (each organization is a tenant boundary)
    `-- Workspace(s): one default for most organizations, more when needed
        |-- Membership, AI/external connections, settings and resources
        `-- Multiple projects
            |-- Persistent code, repository and selected source material
            |-- Environments, runnable devspaces, apps and services
            |-- Tasks, remote sessions, logs, previews and deployments
            `-- Published URLs, custom domains, usage and lifecycle history
```

OWN-01: setup explicitly binds internal org, identity and infrastructure ownership.
IAM-01: UI-managed primary/secondary staff roles have scopes, expiry and revocation;
show effective access. IAM-02: tenant roles confer no staff access; staff roles confer
no customer-data or personal AI access. Support grants are scoped, temporary and
audited. Enforce all actions server-side and verify session MFA, not enrollment alone.
TEN-01: organization -> workspace(s) -> multiple projects. Onboarding opens one
default workspace; additional workspaces remain supported. Projects must open
persistent files/runtime, run, preview, and reopen after restart. Validate object
ancestry, membership and quotas. Migrate current compute workspaces as devspaces
without changing IDs/history silently; Incus projects remain separate provider objects.
INF-01: configure Incus/Proxmox, real capacity, allocations and placement under internal
infrastructure. INF-02: tenants see only their own usage/limits/resources; staff access
is scoped. Ownership differs from placement. Mark unknown/stale observations.
## Workspace AI subscriptions and external sources

AI-01: offer Activate Claude subscription in the workspace. It starts the user's
supported sign-in to unmodified Claude Code in an isolated owner runner. Bind the
resulting permission to the user and selected workspace. Preserve vendor-supported
sign-in choices; do not pool subscriptions, collect Claude session tokens centrally
or mount a user's credential volume into a shared project environment.

AI-02: dispatch a task to an authorized workspace/project runner, show its queue,
progress, logs, changed files, result and failure, and support cancellation and
restart recovery. Enforce project access, resource/concurrency limits and connection
revocation. Review changes before publication. CLI sign-in status alone does not
prove usable entitlement or successful execution. Native subscription sessions and
SDK/API credential modes are explicit; never silently switch to API billing or use
consumer subscription tokens as a Shippin API proxy.

AI-03: remote access opens the intended project through supported terminal/IDE,
SSH bridge or Claude remote connection. Access is scoped, expiring and revocable;
reconnection preserves authorized project state. An installation command alone
cannot establish connected status. Vendor eligibility must be checked at runtime.

EXT-01: Connect Google Drive through Google authorization. Select files/folders
and explicitly grant their use to a workspace/project. Use selected-file scope
where sufficient; record source IDs, versions, sync/import state and provenance.
Support disconnect/revocation. Imported material is never implicitly public or
organization-wide. Additional connectors follow the same ownership model.

PLAN-01: distinguish Shippin hosting/resource entitlements from AI-provider
subscriptions and limits. A hosting plan does not promise unlimited Claude usage.
Pricing and billing policy remain configurable product decisions.

## Deployment, services and publishing

DEP-01: automatic and advanced UI paths generate the same validated versioned
plan and use the same authorized executor. Advanced controls expose supported
source, dependencies, secrets, placement, capacity, storage, network and recovery.
DEP-02: choose workspace/project/environment, template/public-private Git/custom
source, configure, review, build/deploy, verify health, operate/update/back up/
restore/remove. Preserve source/build provenance, execution logs and recovery state.

SVC-01: PostgreSQL, Dragonfly, ClickHouse, Nomen, Vaultix and catalog additions declare
versions, prerequisites, inputs, dependencies and health. SVC-02: managed/external and
platform/customer ownership are explicit; detach never deletes external services.
SVC-03: verify each engine's lifecycle, rotation, backup/restore separately. Nomen owns
identity-provider records, Vaultix owns secrets, Shippin owns resource/deployment state.
PUB-01: Publish gives an actual healthy app an available name.shippin.app or
name.shippin.dev address for this operator's installation. Base domains and allowed
choices are configuration, not mandatory domains for every self-hosted installation.
Validate slug uniqueness/reservations, route ownership, TLS and deployment mapping.
Preview/private/production exposure is explicit; publication must be intentional.

PUB-02: custom domain flow is Add domain -> verify ownership -> provide exact DNS
records -> validate routing and HTTPS -> switch primary address. Recommend
Cloudflare with optional scoped automation; retain manual DNS for other providers.
Use CNAME where valid and appropriate apex records/flattening where required.
The same deployed app gets a new address; its files/data/server need not move.
Keep the working URL during failed verification or cutover and report DNS/TLS state.

PUB-03: project management exposes deployment versions, preview/production state,
URLs/domains, logs, health, resources, scaling and supported rollback/recovery.
NET-01: Cloudflare Tunnel, Tailscale and custom connectivity are UI-configured,
scoped and verified against real routes. Inventory alone is not deployment.
GOV-01: AgentHub owns configured human-review policy. Shippin records actual system,
action, decision and provenance. Hostnames/aliases are data, with no site-specific
Gold/Warden policy built into the product.

## Delivery and proof

REL-01: Docker setup includes owner credentials, internal org, branding and persistent
service choices; no shared password. Supply the one-time setup code to the operator.
REL-02: preserve data and grants on upgrade; prove backup/restore/rollback.
UI-01: one panel, destinations within four actions. SRC-01: methodical reviewed source
absorption with evidence before archive; preserve user work. ENG-01: canonical owners,
provider-specific files, validation/Biome/Lefthook/Zod consistency, new shippin_ tables,
no site hardcoding or sibling-runtime dependencies, explicit public/private interfaces.
Acceptance journey: create tenant/default workspace -> open two real projects ->
activate own Claude connection -> authorize selected Drive content -> dispatch and
remotely reconnect to the intended project -> run/preview -> publish Shippin HTTPS
URL -> verify custom domain -> deploy update -> recover failed update -> revoke
connections and prove isolation. Each step needs UI/API persistence, actual execution,
negative paths and release evidence. Implemented, tested, deployed and live-provider
verified are separate states. Forms, manifests and healthy containers alone do not pass.

Provider constraints checked 2026-11-07:
[Claude hosting/authentication](https://code.claude.com/docs/en/legal-and-compliance),
[Remote Control](https://code.claude.com/docs/en/remote-control),
[Drive scopes](https://developers.google.com/workspace/drive/api/guides/api-specific-auth).
These inform the target; they do not prove Shippin's integration works.
