# Shippin delivery roadmap

[The product contract](objective.md) governs the target and stable requirement IDs.
Updated 2026-11-07 for internal operator ownership, staff roles, tenant workspaces,
workspace AI subscriptions, Google Drive, runnable projects and app publishing.
Reconcile older plans with this order; historical evidence is not current delivery.

## Intended tenant journey

Sign in -> open the organization's default workspace -> create/open multiple
projects -> activate the user's Claude connection for that workspace -> connect
selected Google Drive material -> dispatch work or connect remotely -> run/preview
an actual app -> publish name.shippin.app or name.shippin.dev -> connect a custom
domain -> manage deployments, services, resources, updates and recovery.

One organization is one tenant boundary. Most have one workspace; additional
workspaces are supported. Connections retain owner identity and explicit workspace/
project grants. Publishing base domains are operator configuration. Cloudflare is
the recommended DNS integration, with a manual path for other DNS providers.

## Current evidence and release blockers

| Area | Observed state | Remaining work |
|---|---|---|
| Source ownership | Go, web/, providers and partial deployment code exist here. | Source-family closure remains open; no folder is declared archive-ready. |
| Installation foundation | Docker fresh-install HTTP tests pass for password setup/rotation, internal organization, branding, customer creation, workspaces, projects and persistence across restart. The existing local installation was upgraded with its owner/data preserved; PostgreSQL and Dragonfly report healthy. The prior database backup restored successfully in isolation. | Current CI must pass before release closure. Browser walkthrough could not run because the browser host was unavailable and the native window remained minimized. Dragonfly packaging is not managed customer-service provisioning. |
| Staff authority | staff_roles/local-IdP grants coexist with separate staff_assignments/ops_* grants. | Reconcile one assignment authority with primary/secondary scopes and effective-access provenance. |
| Staff/customer separation | Bootstrap and migration bind the internal Operations registry, local identity organization and system tenant explicitly. Invitations and role assignment use that link; existing customer memberships are preserved. | Reconcile scoped staff roles and prove current-session MFA, infrastructure access and customer-data denials across all entry points. |
| Internal infrastructure | Internal ownership is persisted and shown in the operator panel. | Provider capacity placement and complete resource ownership enforcement remain open. |
| Tenant hierarchy | Local customer creation with an explicit owner creates its default workspace. Additional workspaces and projects persist; API tests cover cross-tenant denial, member write denial and deletion guards. Compute UI is colocated under features/devspaces; grouping UI under features/workspaces. | Workspace grants/quotas and runnable compute remain open. Pending projects are not deployed applications. |
| AI connections | Subscription status, isolated owner-runner and bridge pieces exist. | Verify the workspace-scoped grant/dispatch/remote flow. Existing docs record blocked legacy dispatch; do not present activation alone as completion. |
| Google Drive and publication | These are explicit product requirements. | No end-to-end Google Drive, Shippin-domain or custom-domain workflow was verified in this packet. |
| Providers/services | Inventory, bounded guest actions and partial Incus execution exist. | Real provider acceptance, Proxmox app execution and complete managed-service lifecycle remain open. |

## Ordered delivery

| Stage | User-visible result | Owners and acceptance gate |
|---|---|---|
| 1. Internal organization/access | Owner sets up the internal org and assigns primary/secondary staff access. | Existing identity/org/staff DB/API and team UI: explicit bindings, scoped grants, no implicit tenant access, denial tests and migration proof. Complete pending password/install work here. |
| 2. Tenant workspace hierarchy | A tenant opens its default workspace, adds another if needed, and opens multiple projects. | Existing tenant/org/project/compute owners and panel: grouping-workspace/devspace migration, persistent project state, quota ancestry and cross-tenant denial. |
| 3. Infrastructure/runtime | Staff registers Incus/Proxmox hosts and allocates capacity; a project runs and exposes its real preview. | Server/provider/host-agent owners: credential/TLS failures, inventory freshness, isolated placement, restart recovery and live provider verification. |
| 4. Workspace AI/Drive/remote work | Activate own Claude connection, authorize selected Drive content, dispatch to a chosen project and reconnect remotely. | Existing subs/bridge/jobs/project owners and connection UI: supported native sign-in, owner-runner isolation, workspace grants, task logs/cancel/recovery, Drive provenance/revocation, actual remote session. Explicit SDK/API credential mode where required. |
| 5. First complete service | Tenant and operator can each deploy an appropriately owned PostgreSQL instance using automatic or advanced configuration. | Catalog/deployments/managed/jobs/provider/UI owners: same validated plan from both paths; provision, health, credential isolation, update, backup, verified restore and removal. External detach never deletes data. |
| 6. Service family | Dragonfly, Nomen, Vaultix, ClickHouse and other services have deploy/connect/configure flows. | Engine/service adapters under existing owners: per-service requirements, dependency ordering, ownership, rotation, resources and recovery. No blanket parity claim. |
| 7. Build/publish/manage | Deploy Git/custom sources; publish a Shippin HTTPS URL; connect a custom domain; manage the live app. | Source/builder/deployment/networking/edge/UI owners: pinned builds, slug ownership, route/TLS verification, DNS guidance/optional Cloudflare, safe cutover, logs/resources/updates/recovery; Tailscale/tunnel/custom paths verified individually. |
| 8. Release/source closure | Clean install, upgrade, restore and support work; fully accounted source folders become archive candidates. | Install/release/source-ledger owners: clean-folder build, current CI, real provider journeys, isolation, backup/restore, public/private boundaries and preserved dirty source work. |

Each stage closes a user journey. Absorb source capabilities within their owning
stage and update the existing ledger. Incus does not close Proxmox; PostgreSQL does
not close another engine. Platform dependency packaging does not close customer
service provisioning. Automatic and advanced UI paths must share one executor.

## Immediate packet

Finish publication/CI evidence for the installation and hierarchy packet.
Next reconcile one scoped staff-role authority, including primary/secondary roles,
effective-access provenance and current-session MFA. Then complete and verify the
first real project runtime on registered infrastructure. The database hierarchy
does not close either the authorization or runtime stage.

## AI work and evidence

Every packet records requirement IDs, inspected sources/hashes, one user journey,
migration/compatibility decisions, negative paths, commands and observed acceptance
results in the existing governance session. Use the existing roadmap and ledgers;
do not create a competing panel or plan authority. Preserve broader requirements
until their gates pass. Report implemented, tested, deployed and live-provider
verified separately. Disk capacity was restored during this packet; local evidence
records both the earlier write failures and subsequent successful checks.

Detailed records: [source ownership](source-of-truth.md),
[capability ledger](design/capability-absorption.md),
[archive readiness](design/archive-readiness.md).
