# Shippin platform absorption plan

Status: execution plan, 2026-11-04. This describes the target and acceptance
gates; it does not claim unimplemented workflows are live. The [capability
ledger](capability-absorption.md) holds source-by-source evidence and dispositions.

## Product boundary

`shippin-platform` is the only product repository and `web/` is its only panel.
The Go control plane owns configuration, authorization, desired state, operation
records and observed state. `cmd/shippin-hostd` owns bounded host-side execution.
`internal/hv/incus` and `internal/hv/proxmox` own provider-specific operations.
`internal/networking` owns external connectivity providers. `internal/catalog`
owns reusable deployment definitions and source provenance. Managed services
own engine-specific lifecycle and recovery behavior. AgentHub owns human-review
policy, tickets, evidence and decisions.

Historical panels, routes and Warden names remain source provenance only. Reuse
means comparing behavior, then authoring it into the destination owner with its
current contracts, tests and failure behavior. No server-specific hostname,
address, tenant ID, credential or reviewer becomes a product default.

## Target domain model

- **System**: one stable Shippin ID for a physical or virtual system. Provider
  identifiers and AgentHub machine IDs are explicit mappings. Hostname,
  nickname, alias and tags are descriptive fields with source/time provenance;
  they never become authorization keys.
- **Provider connection**: one configured Incus or Proxmox API with sealed
  credentials, TLS validation, discovered capabilities, test result and
  freshness. A connection does not imply that a guest or service is deployed.
- **Deployment definition**: a versioned, reviewable contract for a pinned
  GitHub, custom HTTPS Git or OCI source, build context, declared inputs,
  resource bounds, health checks, secret references and compatible target
  capabilities. Mutable references, credentials in URLs, undeclared privileges
  and unbounded commands are rejected.
- **Deployment instance**: a durable desired generation and separately observed
  provider state, linked to a system, definition version, operation history and
  secret references. `pending`, `degraded`, `unknown`, `failed` and `ready` have
  distinct meanings. Only provider read-back and a passing configured health
  check can produce `ready`.
- **Managed service**: a deployment instance with an engine-specific adapter.
  PostgreSQL, Dragonfly, Redis and ClickHouse have independent version,
  configuration, storage, upgrade and backup/restore capabilities. Shared
  lifecycle plumbing does not erase differences between their data formats or
  recovery semantics.
- **Network connection**: a configured Cloudflare Tunnel, Tailscale or custom
  provider integration. Its observed inventory is separate from requested
  provider changes and from service attachment. Unsupported write operations
  remain visibly read-only.
- **Platform service**: a UI-managed service definition and its time-stamped
  probes/resource observations for Nomen, Vaultix, Edge, Database, Operator and
  any installation-added service. Missing or stale measurements remain
  `unknown`/`stale`, never fabricated zeroes or operational status.
- **Change review**: an AgentHub policy decision and hash-bound ticket/review
  tied to the same stable Shippin system, exact action, provider resource,
  evidence and rollback. Shippin does not keep a competing approver list or
  authorize by hostname.

New durable Shippin-owned tables use the `shippin_` prefix. Existing legacy
tables are not renamed in place: each migration needs an additive backfill,
compatibility window, stable IDs, rollback plan and upgrade test. Secret values
are stored only by the configured secret owner; product rows contain references.

## Operator navigation

Keep workflows in the existing shell and group destinations so the usual path
fits within four actions:

| Starting point | Path | Main actions |
|---|---|---|
| Operator home | Infrastructure → Hosts & nodes → system detail | add/edit/test Incus or Proxmox; view guests, resources, operations and health |
| Operator home | Deployments → Catalog → definition/instance | add source, validate, choose target, configure, deploy, inspect events and roll back |
| Operator home | Services → Managed data → instance | provision PostgreSQL/Dragonfly/Redis/ClickHouse, configure, observe, back up/restore, upgrade |
| Operator home | Infrastructure → Connectivity → connection/resource | configure Cloudflare/Tailscale/custom, test, refresh, rotate/revoke and attach where supported |
| Operator home | Platform health → service detail | add/edit/disable probes, inspect freshness, latency, resources and events |
| System detail | Review status → AgentHub ticket | inspect policy provenance and hash-bound review, then retry the same Shippin operation |

Navigation visibility follows server-side capabilities and tenant scope. Buttons
must call persisted APIs, show provider-backed results and expose recovery/error
states. A design-only screen or saved catalog source is not a deployment.

## Ordered implementation phases

### 0. Source inventory and ownership

Record each source revision and dirty state. Compare each useful behavior with
its current platform API, schema, UI and tests. Record one canonical owner and
one disposition per row: already implemented, to author, replaced with evidence,
rejected with reason, or unresolved. Verify every matrix row and source family;
do not archive external trees while any useful behavior, rights question or
user-local change remains unresolved.

**Exit gate:** the source ledger is complete, license/provenance/dependency
questions are explicit, and no source is treated as a runtime dependency.

### 1. Stable identity and AgentHub review

Define the canonical system record and explicit mapping to provider and
AgentHub machine IDs. Add authenticated enrollment/observation with hostname
source and freshness; add editable nickname/aliases/tags with audit. Integrate
AgentHub using a least-privilege service principal and configured tenant
mapping. The action path must ask policy, create/retrieve idempotent tickets,
attach redacted evidence, submit, re-read the current ticket/hash and review,
then execute only if the policy says so. Record policy version and decision
provenance. Fail closed on missing mapping, stale hash, timeout, denial or
provider failure.

**Current gap:** Shippin now persists an explicit server UUID to AgentHub
tenant/machine mapping with operator provenance; the mapping is not validated
against AgentHub and there is no adapter. AgentHub exposes authenticated
tenant-scoped policy and ticket routes. The reviewed decision
response does not expose a structured `review_required` outcome; parsing its
human-readable failure text would be brittle. Define or verify a typed decision
contract and service-principal permissions before connecting writes. Local
Shippin approver defaults and the `HUB_OWNER_*` bypass are removed; the example
installation denies gated actions.

**Exit gate:** identity collision/change tests pass; AgentHub decision, review,
replay, denial, unavailable service and restart cases pass; no local reviewer
rules authorize provider actions.

### 2. Incus and Proxmox lifecycle

Keep one server/system registry and distinct provider implementations. Add/edit/
test must validate endpoint, credentials, TLS, selected node/project and
capabilities without saving a failed probe. Lifecycle requests use typed
provider operations, durable operation IDs, idempotency, read-back and honest
unknown/degraded states. Add Proxmox placement only from live storage/network
capabilities; preserve provider-specific VM/container semantics. Present the
operator capability as **Shippin Prox**, while `proxmox` remains the provider
kind. Split shared `lifecycle.go` behavior into clear Incus and Proxmox modules
where operation semantics differ.

**Exit gate:** multiple configured systems can be connected and edited; create,
start, stop, snapshot, delete and failure/retry are tested against disposable
providers, with authorization, AgentHub review, audit and provider read-back.

### 3. Catalog contracts and deployment runtime

Version source and deployment contracts. Validate immutable Git commit or OCI
digest, trusted source origin, subdirectory containment, declared parameters,
secret references, resource/capability limits and health checks. Fetch and build
in an isolated bounded builder with configured egress, storage and concurrency
limits. The control plane schedules a desired generation; provider-specific
drivers apply it; a reconciler observes actual state, retries safe operations,
records events and handles cancellation/restart/deletion. Never accept
unbounded shell, privileged Compose or arbitrary host execution as a shortcut.

**Exit gate:** a pinned source can be configured, reviewed, built, deployed to
an explicitly selected compatible target, observed healthy, updated and removed
with failure/recovery coverage. A metadata pin alone remains labeled metadata.

### 4. Managed data services

Build service adapters on the deployment runtime, keeping engine differences
explicit. Configuration covers engine/version, placement, CPU/memory/storage,
replicas where supported, network access, secret refs, health, upgrades and
backup/restore. PostgreSQL backups include consistency and restore verification;
Dragonfly and Redis document persistence/replication differences; ClickHouse
documents storage layout, users and restore semantics. Examples are editable
catalog definitions, not hardcoded server deployments.

**Exit gate:** an operator can configure and provision each supported engine,
inspect real resources/health, rotate credentials, back up, restore, upgrade and
delete; tenant scope and recovery are tested. Unsupported operations are not
advertised.

### 5. Connectivity providers

Complete Cloudflare Tunnel, Tailscale and custom connection workflows in
`internal/networking`: configure, validate, test, refresh, rotate/revoke and
perform provider writes only when supported. Associate observed resources with
stable systems and service instances. Keep provider API scopes minimal and
credentials sealed; do not represent imported inventory as a configured
connection or a live route.

**Exit gate:** each supported provider has add/edit/test/read-back and
permission-denial coverage; custom integrations declare the minimum protocol
and resource fields and use prefixed tables.

### 6. Platform service registry and dashboards

Replace static probe-only facts with a persistent UI-managed registry. Configure
display name, protocol, endpoint, probe bounds/cadence, optional secret
reference, resource sources and enabled state. Collect live probes and provider
resource observations with time, provenance and retention bounds. Build linked
operator dashboards for platform services, provider systems, deployment/service
instances, operations and events.

**Exit gate:** add/edit/disable/test works; stale and unknown are distinct;
latency, memory and address are sourced rather than mocked; all destinations are
reachable in four actions or fewer from the relevant operator view.

### 7. Release and source retirement readiness

Run the documented developer checks, full Linux Go and panel checks, fresh and
upgrade migrations, secret/public-tree scans, install/compose paths and
disposable Incus/Proxmox scenarios. Build standalone control-plane and host-agent
executables from clean `main`. Reconcile every source row against implementation
evidence and document excluded features, external user changes, unresolved
rights and failure paths.

**Exit gate:** clean developer build and CI are green; every claimed deployment
is provider-backed and observed; each remaining source row has an evidence-backed
disposition. Produce an archive checklist for user review. This plan does not
delete, reset or archive the sibling repositories.

## Required evidence for each phase

Every implementation slice links its owning Go/API/schema/UI files, tests,
configuration shape, security boundary, failure behavior and deployment evidence
in the absorption ledger and ignored session journal. New tables must have
explicit `shippin_` names. Every write needs authorization, tenant scope,
idempotency, audit, secret redaction and read-back. CI is checked after each
direct-to-main commit; a red main is fixed before the next slice.
