# Managed data services

Status: design and partial deployment runtime. The catalog stores validated,
pinned source metadata and configuration schemas. The deployment worker can
execute a bounded Incus OCI create/start/remove path, persists target-scoped
health probes, and requires a positive result before reporting ready. Git/custom
source resolution, Proxmox application runtime, database-specific operators,
credential/user management, backups, restore, upgrades and live provider
acceptance are not complete. No Postgres, Dragonfly, Redis, ClickHouse or
Supabase-compatible Auth service is turnkey today. A catalog entry or accepted
desired-state request does not mean a service is deployed.

## Ownership and current state

The single `web/` panel and the existing platform bounded contexts own this capability:

| Concern | Canonical owner | Current state |
|---|---|---|
| Reusable service definitions, schema, source pin and secret references | `internal/catalog` | Definitions and source metadata exist. Source fetching/building does not. |
| Tenant desired generation, target, config snapshot and event history | `internal/deployments` | Desired generations, outbox, worker, fenced step journal, Incus OCI operations, durable health command/results and event history exist. A pending health probe remains `unknown`; only a verified probe can yield ready. Other lifecycle and providers remain incomplete. |
| Host-side bounded operations | `internal/hostd` | Existing authenticated host operations are not a managed-service runtime. |
| Incus/Proxmox provider behavior | `internal/hv/incus`, `internal/hv/proxmox` | Provider drivers and provider-specific guest lifecycles exist. General provider mutation routes fail closed. The worker executes only the listed reviewed Incus deployment steps; Proxmox application deployment remains unsupported. No disposable provider acceptance has been performed. |
| Human-review policy, tickets, evidence and decisions | AgentHub | Shippin has tenant/system mapping, ticket workflow and exact action-bound provenance. The deployment worker performs lease-fenced just-in-time authorization before implemented Incus writes. Production least-privilege identity and live AgentHub policy/quorum are unverified. AgentHub remains the only human-review policy owner. |
| Secret references | Catalog validation and tenant-scoped Vaultix resolver | `vaultix://<environment>/<folder...>/<key>` derives the project from the deployment tenant and installation prefix; one-key `Reveal` is audited before reading. SOPS references remain metadata-only until a configured resolver exists. Never store secret bytes in service config or deployment events. |

Do not extend the rrflow-specific `service_instances` table into a second generic deployment registry. Do not create a parallel `/v1/infra/services` manager. Managed engines should be typed catalog definitions and deployment instances in the existing deployment owner. Engine-specific differences belong in service contracts and runtime adapters.

Historical sources reviewed include the clean `shippin-cloud` checkout at `49475d3b` and the Warden capture matrix. Those sources inform capability accounting only; their panels, endpoint defaults, local secrets files, shell executors and approval mechanisms are not product implementations. See the [capability absorption ledger](../absorption/capability-absorption.md).

## Service contracts

Each supported engine needs an explicit, versioned definition with:

- immutable image or source identity and provenance;
- engine version, compatible provider targets and installation-selected placement;
- bounded CPU, memory, storage and replica settings where the engine supports them;
- network exposure and access policy;
- secret references, health checks and resource observations;
- upgrade, backup, restore and deletion behavior, including unsupported cases.

PostgreSQL, Dragonfly, Redis and ClickHouse must not be treated as
interchangeable containers. PostgreSQL requires a consistent backup and verified
restore. Dragonfly and Redis need separate persistence, compatibility and
replication contracts; do not add both without a concrete workload and
compatibility need. ClickHouse needs its own storage, user/role, resource and
recovery contract. Catalog examples are editable definitions, never
server-specific deployments or promises that a runtime exists.

## Product model and authentication boundary

Shippin manages infrastructure and service lifecycle; it is not the tenant's
application identity provider. Nomen authenticates people to the Shippin panel.
AgentHub supplies human-review policy for Shippin operations. A tenant's
application Auth service, when selected, is a separately deployed service with
its own URL, keys, users, policies and lifecycle. It must never replace Nomen,
AgentHub, or Shippin tenant authorization. ClickHouse service users and roles
are separately managed using ClickHouse's access-control model; they are not
Supabase Auth users.

The first data product is an explicitly created PostgreSQL instance. The
operator selects a compatible provider/target, version, compute and storage
bounds, private network, backup destination/policy and supported extensions.
Shippin generates initial credentials, stores them only through the configured
secret owner, shows each generated secret once, and offers rotation. The panel
then provides safe instance status, connection settings, database and
least-privilege role management through typed operations. It must not expose an
arbitrary SQL superuser console as its management interface. Public access is
off by default; access is configured through the supported networking feature
with explicit scope and read-back.

Application Auth is a separately selectable service after PostgreSQL lifecycle
and restore are proven. Evaluate the supported upstream Auth service and its
license, version, image provenance, upgrade path and maintenance burden before
adding it as a versioned catalog bundle. Bind its database to the selected
tenant instance and show its endpoint, callback allowlist, provider settings,
user/role administration and key rotation in the same Shippin panel. Do not
claim the full Supabase product surface (REST, Realtime, Storage, Functions,
Studio, pooling and management API) from Auth alone. Any optional component
must have its own explicit service contract and failure state.

ClickHouse is a separate analytical data product and not an extension of the
PostgreSQL/Auth instance. Provision it with engine-specific storage and resource
limits, private network, TLS and native users/roles/quotas. The Shippin panel
manages instance lifecycle and typed access entities without treating the
control-plane login as a database credential. Backups and restores need an
operator-configured compatible object-store destination, provider verification
and a tested restore target before they are offered.

## What to learn from Neon and Supabase

Use Neon as a product-workflow reference: clear projects and environments,
branch-aware development, isolated preview databases, connection details,
resource visibility and safe recovery. Do not represent a VM-backed PostgreSQL
clone as Neon-style instant copy-on-write branching. That property depends on a
storage architecture built for versioned pages/WAL and shared data; Shippin
should begin with explicit backup/restore or supported storage snapshots and
publish measured operation time and cost. Scale-to-zero and compute/storage
separation remain future architecture work, not UI labels for a normal VM.

Use Supabase as a reference for a cohesive developer workflow where Auth and
other services integrate with one Postgres instance and are managed from one
console. Its official self-hosting guide documents that the self-hosted stack is
one project, mostly environment-configured, and lacks several managed-platform
features, including its Management API, managed backups/PITR and branching. That
is a warning against shipping an upstream Compose file and calling it a
multi-tenant managed platform. Shippin must own project/instance records,
configuration persistence, health/resource history, upgrades, backup/restore,
and the single management UI itself.

Reference documentation: [Neon branching](https://neon.com/docs/introduction/branching),
[Neon storage architecture](https://neon.com/storage), [Supabase self-hosting](https://supabase.com/docs/guides/self-hosting),
[Supabase Auth architecture](https://supabase.com/docs/guides/auth/architecture),
[ClickHouse access control](https://clickhouse.com/docs/concepts/features/security/access-rights),
and [ClickHouse backup/restore](https://clickhouse.com/docs/concepts/features/backup-restore/overview).

## Public core and private extensions

`shippin-platform` remains the sole product panel and canonical owner of its
public API contracts, shared domain models and deployment/configuration flow.
Private modules must not create another panel, fork the control-plane database,
or drift by copying public code. Where a feature must remain private, put it in
an independently versioned service or build artifact behind a documented,
authenticated, versioned contract owned by the public platform. The panel learns
available capabilities from that installation's authorized API response; absent
private services disable their routes/actions with a truthful unavailable state.
The public build and self-hosted installation must work without credentials or
access to private repositories.

Before any private module is split out, record which source paths and data it
owns, the contract and compatibility window, installation/configuration method,
secret and review boundaries, SBOM/provenance, release artifact verification,
upgrade/rollback behavior and the license/reuse rights for both repositories.
Do not make a Go `plugin` loader or download executable code into a running
control plane as an extension mechanism. A separate authenticated service is
the default boundary; a build-time private Go module is acceptable only if
release reproducibility and customer distribution/licensing are explicitly
resolved without making the public developer build depend on it.

## Persistence and migrations

Use the existing `shippin_deployment_instances`, generation and event owners for desired and observed deployment state. Add only engine-specific tables when a distinct durable concern has no existing owner. New tables use the `shippin_` prefix; backup records, if introduced, must reference a deployment and exact generation and contain storage references rather than credentials or payloads.

Do not widen or repurpose rrflow tables to represent managed engines. Before adding schema, document additive migration behavior, tenant isolation, idempotency, audit, retention, rollback and restore. Do not infer deployed instances from historical hostnames, addresses, tenant IDs, compose files or source-control state; an operator must explicitly enroll or create each target.

## Review and provider execution

AgentHub owns configurable review policy. Shippin must submit the exact action, stable Shippin system ID, provider resource, desired-generation hash, policy and ticket provenance, evidence and rollback data. Only a typed, current AgentHub decision bound to that request can authorize execution. A local Shippin task-board card, boolean from an unverified decision response, ticket status, hostname, role name or UI confirmation is not provider authorization.

The deployment worker validates the exact action, current ticket and policy,
records safe provenance, and performs just-in-time AgentHub authorization before
the implemented Incus volume, OCI create/start and removal writes. Those Incus
steps have durable intents, one-time dispatch claims, read-back and hosted
PostgreSQL recovery tests. Target health is separately queued to the enrolled
host and only a verified application result can produce ready. This does not
establish live AgentHub quorum, disposable Incus behavior, Proxmox application
runtime, or a managed engine. Do not advertise an accepted intent as running.
Each future action must be idempotent, generation- and lease-fenced, reviewed
immediately before mutation and confirmed by provider read-back and configured
health observations. Denial, timeout, stale review, changed policy, provider
failure and restart must retain an honest pending, failed or unknown state.

## Panel workflow

The deployment page in `web/` is the only service configuration surface. It discovers services and tenant-authorized targets, renders validated configuration forms, shows pinned source and desired generation, and distinguishes provider-observed state with event history. Editing a definition or requesting a deployment does not imply that a provider operation ran. Review status comes from AgentHub; Shippin does not offer a local-board approve or retry-and-apply control. Vaultix references name an environment and key path only; the tenant project is resolved from the authenticated deployment tenant. The SOPS shape is accepted as metadata but has no deployment resolver yet.

Engine-specific operations such as backup, restore, scale and upgrade become available only when their API, authorization, executor, persistence, read-back, failure recovery and panel workflow are implemented for that engine.

## Delivery sequence

1. Complete disposable Incus health/restart acceptance and repeat the provider
   lifecycle matrix on disposable Incus and Proxmox. Keep the executor boundary
   provider-specific; Proxmox VM/container guest management does not imply an
   OCI application runtime.
2. Add PostgreSQL as the first managed engine: typed catalog recipe, target
   capability checks, isolated storage/network, generated and rotated secrets,
   create/read-back/health, safe connection details, user/role operations,
   upgrade and exact-instance removal. Add no default external endpoint.
3. Add PostgreSQL backup and restore to operator-configured destinations,
   restore rehearsal, freshness/retention display and recovery UI before calling
   the engine production-ready.
4. Add the optional Auth service and its instance-level user/provider/settings
   UI only after its provenance/license and compatibility are reviewed. Keep
   Nomen and AgentHub as separate control-plane concerns.
5. Add ClickHouse with separate resources, role/quotas, connectivity, backups
   and restore proof. Add Dragonfly or Redis only when a supported app workload
   demonstrates the need and compatibility choice; test each as its own engine.
6. Evaluate preview/branch workflows only with storage technology that provides
   the required isolation, retention and recovery semantics. For initial
   serverful PostgreSQL, offer measured clone/restore operations rather than
   claiming instant branching.
7. Integrate optional private features through the documented API/service
   contract and verify clean public checkout build/install without access to
   private source. Release versioned artifacts with provenance and SBOM.

## Backup source review

The Warden reference contains estate-backup concepts for S3, SMB, Infisical, preflight, coverage and event history. The old shell panel is not served. Its secret file and shell-out transfer executors are excluded. The 2026-11-05 source review found the captured backup subsystem has no Git history, uses a local JSON store and estate-specific paths, executes configurable tar/rsync commands (including rsync --delete), and does not prove the platform tenant, AgentHub, audit, generation or secret-resolver boundaries. Its S3/SMB/Infisical code and tests are rejected as implementation sources; this does not close the product backup requirement. Re-author bounded destination metadata, secret references, coverage facts and redacted events under the appropriate deployment or backup owner after separate provenance, dependency, license and threat review. Hypervisor snapshots remain distinct from engine-consistent database backups.

## Acceptance gate

For each engine, a self-hosted operator must be able to configure a supported
version and target, obtain AgentHub authorization for the exact generation,
deploy it through a bounded provider adapter, observe provider read-back and
health, rotate credentials, perform and verify backup/restore, upgrade and
remove it. Tenant isolation and negative paths must be tested against
disposable providers and PostgreSQL. The shared deployment substrate is partly
implemented; every managed engine and its user-facing lifecycle remain
unavailable until their own evidence passes. Do not call a catalog row,
container start or SQL connection turnkey.
