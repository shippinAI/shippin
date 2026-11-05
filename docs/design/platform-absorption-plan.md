# Shippin platform absorption plan

Status: execution plan, 2026-11-04. This describes the target and acceptance
gates; it does not claim unimplemented workflows are live. The [capability
ledger](capability-absorption.md) holds source-by-source evidence and dispositions.

## Ordered delivery queue

The order below is a dependency order. Each row closes only when its observable
acceptance checks pass; a panel or catalog screen alone does not close a runtime
capability. Latest green implementation commit before this slice: `6081250`
(Checks run `41936135681`, all 13 jobs passed, 2026-11-04). Current worktree adds
AgentHub policy context fields and binds evidence to its ticket; focused Go tests
and full local `scripts/check.sh` pass. A disposable AgentHub contract run passed
ticket/evidence/submit/read-back, idempotency replay, configured human plus
reviewer quorum, no-match fail-closed behavior and stale-ticket denial. It used
seeded synthetic principals; Shippin's least-privilege principal, live service
client and provider authorization remain unverified. This does not authorize or
execute provider deployment.

| Order | Work package | Canonical owners | Exit evidence | Current state / gate |
|---|---|---|---|---|
| 0 | Source inventory, one-panel ownership, migration rules and capability ledger | `docs/design/`, `third_party/warden-ct102` reference matrix | Every candidate row has source revision, observed behavior, destination, disposition, rights/security status and acceptance evidence | In progress. Sibling trees remain read-only; Warden matrix still has open rows; archive is prohibited until these close. |
| 1 | Provider declarations, credential-safe edits/tests, Incus and Proxmox guest lifecycle, resources and operation read-back | `internal/servers`, `internal/hv/{incus,proxmox}`, `internal/api`, `web/` | Disposable Incus and Proxmox lifecycle workflows, edit/test denial, read-back, task failure, auth denial and secret redaction | Core lifecycle and shared storage inventory exist. Proxmox physical disk listing and capacity are read-only. Full provider coverage and disposable-host evidence remain open. |
| 2 | Stable Shippin system identity and AgentHub review authorization | system/identity owner, AgentHub adapter, API and audit | Exact action + system + resource + ticket hash + policy version decision; least-privilege identity; no-match, stale, denied, timeout and restart all block writes | **Hard gate, in progress.** Operators configure the HTTPS endpoint and sealed credential, read-test tenant policy, and run a recorded read-only machine/tenant mapping check. The client requires every matched policy rule to explicitly require approval plus a positive role-scoped reviewer quorum. Current worktree prepares generation-bound review context, bounded redacted ticket-bound evidence and exact ticket read-back. Disposable AgentHub test passed configured human/quorum and stale-ticket denial using seeded synthetic principals; AgentHub's seed policy matched no rule and Shippin correctly fails closed. Focused Go tests and full `scripts/check.sh` pass locally; current-slice CI pending. Production least-privilege principal, Shippin live client, timeout/restart and provider decision integration remain open; provider writes remain unavailable. |
| 3 | Deployment reconciliation and provider execution | `internal/deployments`, migrations, hostd, provider drivers, jobs, API/OpenAPI, `web/` | Transactional dispatch, fenced lease, immutable generation, provider-specific executor, restart/retry, read-back, health, idempotent removal and negative-path evidence | **Blocked on order 2.** Current API stores desired generations/events only; no reconcile/build handler exists. Implement no executor bypass or pretend-ready state. |
| 4 | Pinned source resolution and bounded builds | catalog/builder owner, isolated worker, secret resolver | Immutable input commit/digest, build provenance, bounded CPU/memory/time/storage/concurrency/egress, secret mounts, cancellation and cleanup | Not implemented. Must precede GitHub/custom Git deployment; OCI digest path still needs provider compatibility. |
| 5 | Managed PostgreSQL, Dragonfly, Redis and ClickHouse | managed-service owner, hostd/provider adapters, catalog, `web/` | Engine-specific configure/provision/upgrade/backup/restore/health/delete on supported providers; tenant isolation; restore verification | Design only; `internal/managed` is rrflow state, not an engine runtime. No turnkey engine may be advertised. |
| 6 | Cloudflare Tunnel, Tailscale and custom connectivity management | `internal/networking`, API, migration, `web/` | Per-provider add/edit/test/rotate/revoke and supported mutations, stable system attachment, permission denials, redaction and observed-vs-managed state | Configured inventory/refresh exists. Provider mutation/attachment lifecycle remains open. |
| 7 | Dynamic service health and resource views | platform-health registry, provider read APIs, `web/` | UI-managed service definitions; bounded persisted probes; truthful unknown/stale/down states; provider resource reads and independent failure display | Probe registry/configuration/history exists. Resource joins are partial; secret-backed auth and richer service metrics remain open. |
| 8 | Sole-panel workflows and clean developer build | `web/`, `cmd/console`, installer/deploy, docs | 3–4 action navigation, configuration persistence, no dead controls, clean checkout build, standalone executable and install/upgrade evidence | One React panel exists. Full end-to-end workflows and archive-ready clean build remain open. |
| 9 | Archive readiness | capability ledger and source owners | All useful source rows dispositioned, source changes/dirty user data preserved, rights and dependencies resolved, clear archive checklist | Not ready. Do not delete, reset, move or shut down source folders before user reviews the completed checklist. |

### Immediate execution packet

1. Keep source trees read-only and refresh `main`/CI and dirty-worktree evidence.
2. Complete the AgentHub-to-Shippin contract audit, including no-match semantics,
   typed decision intent, policy/ticket hash binding, principal permissions,
   tenant/system mapping and replay behavior. The mapping check is implemented
   as a read-only dry-run with append-only local provenance; it does not prove
   review authorization. Record exact routes, fields and source hashes.
3. Verify the action-bound ticket/evidence/submit flow on Shippin fixtures and an
   isolated PostgreSQL database, then against disposable AgentHub with an
   explicitly least-privilege principal. Confirm mapping, ticket binding,
   evidence redaction, idempotent retries, submit status, human review/quorum,
   denial, timeout and stale action/policy. Keep provider mutation disabled
   until the complete gate is independently demonstrated. A disposable
   AgentHub test has now passed ticket create/evidence/submit/read-back,
   idempotency replay, configured human plus reviewer quorum, stale-ticket
   denial and no-match fail-closed behavior. It used seeded synthetic
   principals, so it does not close the least-privilege Shippin identity gate.
   The Shippin ticket must carry reviewable `topology`, `blast_radius`,
   `snapshot`, research/evidence references and honest future verification
   criteria; the evidence must identify the same ticket and immutable
   generation. Exercise both AgentHub's seeded policy and an operator-configured
   exact-tool rule for `shippin:deployment.apply` / `.remove` that requires a
   human approval plus a role-scoped reviewer quorum. Do not make the Shippin
   client infer an allow when AgentHub returns no matched rule.
4. Only after step 3 passes, implement the durable deployment outbox/reconciler
   and provider-specific Incus/Proxmox execution. The desired generation and
   outbox event must commit atomically; workers must fence leases and re-read the
   current generation and AgentHub decision before any side effect.
5. Continue work packages 4–9 in order, updating this ledger after each accepted
   slice. Push cohesive commits directly to `main`; check all CI jobs before
   starting the next slice.

### Capability-by-capability execution register

This register is the working definition of “absorbed.” A source feature is not
complete because a similarly named screen, model, or file exists. For each row,
the owner must connect configuration, persistence, API contract, UI action,
runtime behavior, observed result, and negative-path evidence. The detailed
source inventory and per-file Warden accounting remain in
[`capability-absorption.md`](capability-absorption.md) and
[`warden-capture-matrix.md`](warden-capture-matrix.md); those records determine
what is useful and eligible for authoring. The order below is a hard dependency
order, not a claim that all stages are currently implemented.

| Stage | Capability slice to author in `shippin-platform` | Canonical destination | Required proof before closure |
|---|---|---|---|
| A | Source provenance, dirty-work preservation, license/dependency/security review; reconcile each Cloud, Console, Mesh and Warden feature family | `docs/design/`, source ledger and Warden matrix | Every source row has revision, source behavior, rights status, current owner, adopt/rewrite/reject disposition and linked behavior evidence. No sibling source is modified or archived. |
| B | Stable Shippin system identity, hostname observation, operator aliases/tags, explicit AgentHub mapping | `internal/servers`, `internal/db`, `internal/api`, existing infrastructure UI | Identity persists through restart; aliases never authorize; mapping is verified against AgentHub and records freshness/provenance; ambiguous or stale mapping blocks. |
| C | AgentHub-configured review preparation, exact operation snapshot, safe evidence, submit/review status and policy provenance | `internal/agenthub`, `internal/api`, `internal/db/migrations`, `web/` Platform settings and deployment details | Additive `shippin_` generation-bound correlation, idempotent draft/evidence/submit calls, bounded redacted snapshot evidence, and deployment review status/action UI are implemented. Focused Go race/vet, isolated PostgreSQL, full local repository checks, OpenAPI consistency, hardcoding checks, and all 13 CI jobs pass (`6081250`, run `41936135681`). Acceptance still requires disposable AgentHub integration, least-privilege principal evidence, human/quorum read-back, review decision polling/binding, stale ticket/policy and timeout coverage. Provider mutation stays blocked until these pass. |
| D | Multi-server Incus and Proxmox declarations, credential-safe edit/test, capability discovery, provider-local VM/container lifecycle, task polling/read-back and resource inventory | `internal/servers`, `internal/hv/incus`, `internal/hv/proxmox`, existing infra API/UI | Multiple configured endpoints work without source edits. Incus and Proxmox behavior stays provider-specific; authorization/audit precede writes; task failure, denial, retry, restart, deletion and observed read-back are covered on fakes and disposable hosts. |
| E | Review-gated durable deployment reconciliation from desired generation to provider execution | `internal/deployments`, prefixed migrations, `cmd/console`, hostd/provider drivers, API/OpenAPI and `web/src/features/deployments` | Intent and outbox commit atomically; fenced leases re-read generation and review decision; provider task and read-back persist truthful observed state; restart, duplicate request, stale generation, rollback and removal are exercised. |
| F | Pinned GitHub/custom HTTPS Git/OCI resolution and isolated builds | `internal/catalog`, bounded builder owner, secret resolver contract, deployment UI | Immutable source digest and provenance; bounded time/CPU/memory/storage/concurrency/egress; cancellation and cleanup; credentials never appear in URLs/logs; no arbitrary privileged host commands. |
| G | Turnkey PostgreSQL, Dragonfly, Redis and ClickHouse definitions and engine-specific lifecycle | existing managed-services owner, catalog/deployment contracts, provider adapters and `web/` | Explicit engine/version/image, placement, resource/storage/network/secrets; install, edit, upgrade, health, backup, restore and delete; tenant isolation and restore verification. Redis and Dragonfly compatibility is stated accurately. |
| H | Cloudflare Tunnel, Tailscale and bounded custom integrations: configure/edit/test/rotate/revoke, attach to stable systems, manage supported provider resources | `internal/networking`, its prefixed schema/API and existing infrastructure UI | Least-privilege provider scopes and exact write support proven; observed inventory remains distinct from requested changes; denial, credential rotation, redaction and stale inventory pass. |
| I | Nomen, Vaultix, Edge, Database, Operator and installation-added service health plus resources | platform health registry/observations, provider read APIs and existing overview/settings UI | UI-managed definitions; configured secret resolver only; bounded scheduling, persisted probes, freshness and provenance; resource metrics join by stable IDs. Missing or stale values stay unknown/stale and failed resource reads do not overwrite service reachability. |
| J | Re-author each remaining Warden/Mesh/Cloud/Console useful feature (backup, restore, DNS/ACME, firewall, logs, runners, templates, workspaces and diagnostics) only into its owned bounded context | Existing `internal/backupcov`, provider/networking, project/workspace, audit, jobs or catalog owners as documented per source row | Each feature receives rights/security review and actual API/schema/UI/runtime/failure evidence. Shell, root file manager, arbitrary Docker/Compose, local account management, unsafe disk controls, site IDs and parallel panel/policy are explicitly rejected. |
| K | One-panel navigation, fully configurable operator and tenant workflows, clean standalone developer build and rollout | `web/`, `cmd/console`, installers/deploy docs | Every implemented destination is reachable in at most four user actions; no dead/mock controls; config survives restart; clean checkout builds console and hostd, migrates, installs and upgrades with supported workflows proven. |
| L | Archive decision for each historical checkout | capability ledger and user-review checklist | All useful source rows closed, local dirty data retained, rights/dependencies recorded, and exact remaining gaps listed. Archiving/deleting source trees requires a separate user decision after this evidence is reviewable. |

Each implementation slice must name its stage and source rows in the commit and
journal, list canonical files it owns, state what is deliberately excluded, and
show an end-to-end acceptance result plus a relevant denial/failure result. If a
stage depends on an unavailable external contract, continue other independent
read-only or bounded work, but do not bypass the dependency or mark the stage
complete. Current active gate is Stage C. The worktree creates and submits an
action-bound AgentHub ticket with policy context fields, a bounded redacted
deployment snapshot naming the exact ticket, persists only Shippin correlation
metadata, and exposes review preparation/status in the existing deployment
panel. Focused Go tests and full local checks pass. Disposable AgentHub contract
behavior passed with seeded synthetic principals; current-slice CI remains
pending. Least-privilege principal evidence and a provider-side authorization
decision bound to the submitted review remain absent. All provider mutations
remain unavailable until the entire gate has acceptance evidence.

### Stop conditions

- AgentHub is unavailable, its policy has no matching explicit rule, its
  decision is not typed and bound to the exact action/system/resource, the
  ticket or policy hash changed, principal permissions are unverified, or the
  system mapping is ambiguous: stop before provider mutation.
- A provider response lacks successful task completion and read-back: retain a
  failed/unknown state and event; never report ready.
- A candidate source has unresolved license/provenance, dirty user work,
  unbounded privileges, installation-specific assumptions or no tests: preserve
  it and record the gap; do not copy it into the canonical tree.
- CI is red on `main`: repair the baseline before adding the next change.

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

## Platform health implementation contract

The final service registry is a normalized PostgreSQL owner, not a JSON blob or
a second panel. `shippin_platform_services` stores stable UUID, display name,
kind, optional alias, HTTP(S) or TCP target, optional stable system/guest link,
timeout, cadence, enabled state, update revision and audit timestamps. A target
must have exactly one supported protocol; credentials remain unsupported until
the configured Vaultix/SOPS resolver has a tested contract. Remove is a soft
disable so observation history and provenance remain addressable.

Migration imports the current `services.probes` platform override. A one-time,
marked startup import adds installation-file probes only when no registry row
already owns that stable ID; later startup must never resurrect a service the
operator disabled or removed from the active set. Legacy definitions without an
ID receive a deterministic UUID from their normalized name during import. After
the migration, all add/edit/disable actions use the registry API, not the legacy
settings key.

The console scheduler claims due rows with database leases so two replicas do
not scan the same service. It obeys the existing process-wide concurrency
setting, per-service cadence (30–86,400 seconds), 1–10 second probe timeout,
the 50-service batch bound and request cancellation. A successful or failed
attempt appends a `shippin_platform_service_observations` row with service ID,
config revision, observation time, configured/healthy state, nullable measured
latency, redacted detail and optional provider-resource snapshot. A read endpoint
does no network I/O. The explicit “check now” action uses the same bounded
worker and persistence path.

The overview joins current definitions to their latest observations. No prior
row means `unknown`; an observation older than twice its configured cadence is
`stale`; only a recent successful probe is `operational`. Provider resource
availability stays separate from service reachability. Retention is configurable
from 1 to 365 days (default 30) and pruning is bounded and scheduled. Tests cover
upgrade/backfill, stable identity, lease contention, timeout/cancellation,
redaction, stale/unknown states, retention and independent Incus/Proxmox resource
failure before the UI calls this a live registry.

### Implementation progress, 2026-11-04

The AgentHub setup slice adds an audited operator API and existing-panel form for
the endpoint and a write-only credential. Credentials are sealed with the
installation session key and a fixed application-purpose context; responses,
audit details and UI state expose only whether a credential exists. A new
`shippin_agenthub_connection` migration stores the singleton endpoint and sealed
bytes. HTTPS endpoint rejection, credential replacement/retention/clear,
round-trip decryption, ciphertext storage, and audit behavior have focused tests.
The same Platform page now offers a read-only policy check for an explicitly
entered AgentHub tenant and displays only policy version/hash metadata. This can
verify configured URL and credentials when an operator runs it; fixture tests
cover the request and local CI has not contacted a live AgentHub. It does not
validate Shippin system mapping, ticket lifecycle, quorum, service principal
permissions or provider authorization. Provider mutations continue to return
unavailable.

The first implementation slice is now authored in the canonical Go database/API
owners and existing React admin/overview pages: migration 046 backfills the old
database override; startup imports file-defined probes once without reactivating
disabled IDs; operators can create, revision-edit, soft-disable/re-enable and
check services; a leased 5-second scheduler writes results; overview reads use only
the latest persisted observation; operators can inspect the newest 20 history
entries; and retention defaults to 30 days with an audited 1–365 day override.
The disposable PostgreSQL 16 run has exercised the legacy JSON backfill (including
an orphaned provider link and an invalid target), registry store, leases,
observations, retention pruning, API create/edit/stale revision/disable, a real
refused local HTTP target, unknown/down/stale status, history read, and the
no-network overview read path.

This closes only the platform-health registry slice. Commit `c36b025` is on `main`
and all 13 jobs in Checks run `41898917087` passed. A follow-up review restored the
existing live hostd instance inventory alongside persisted probe observations. The overall source inventory, provider lifecycle completeness, executable
deployment reconciler/build path, managed database runtimes, external-network
mutation support, and AgentHub action-bound review remain open plan phases.

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

## Deployment runtime implementation contract

The saved catalog source is metadata only. A deploy operation must create a durable
desired generation, preserve the exact source and target snapshot, pass AgentHub's
configured review gate, then dispatch a provider-capability-specific operation. The
reconciler owns the transition from desired to observed state; the UI never infers
success from an accepted request or a completed job alone.

### Artifact and provider paths

- **OCI image on Incus:** use Incus's native OCI application-container path when the
  attached server advertises it. Require an explicit registry and sha256 digest,
  record the resolved digest, and use provider read-back plus the configured health
  check. Incus distinguishes OCI application containers from full-system containers
  and VMs; keep those instance types separate in Shippin. See the
  [Incus instance model](https://linuxcontainers.org/incus/docs/main/explanation/instances/),
  [image references](https://linuxcontainers.org/incus/docs/main/howto/images_remote/),
  and [OCI application launch workflow](https://linuxcontainers.org/incus/docs/main/howto/instances_create/).
- **OCI image on Proxmox:** use an explicitly supported guest runtime and a
  provider-specific bootstrap path. The current Proxmox guest API creates VMs or
  system containers; it does not yet provide a Shippin application-container
  executor. Do not pass arbitrary OCI references to its template field. Require a
  supported guest template, one-time enrollment, and a constrained Shippin hostd
  workload contract before advertising this path.
- **GitHub/custom Git build:** fetch the pinned commit in a short-lived isolated
  worker, produce an OCI artifact, and record its digest and build provenance before
  target mutation. The builder must be a separately configurable service with
  resource, concurrency, storage, and egress bounds. A rootless builder reduces
  daemon privilege but is not itself an egress policy. Build credentials use secret
  mounts; never pass them as build arguments or ordinary environment variables.
  See Docker's [BuildKit configuration](https://docs.docker.com/build/buildkit/toml-configuration/),
  [rootless mode](https://docs.docker.com/engine/security/rootless/), and
  [build-secret handling](https://docs.docker.com/build/building/secrets/).

### Required durable operation contract

Each `shippin_` deployment instance stores tenant, catalog service, stable target
system, source snapshot, desired generation, typed resource/config values, secret
references, desired state and current observed state. An append-only event row stores
actor, generation, operation, provider result, health result, redacted failure and
time. Job rows remain the worker lease/retry mechanism; they are not deployment
truth. A repeated request with the same idempotency key returns the same generation;
an update with a stale base generation conflicts. A retry reuses the generation and
provider operation key. Deletion records desired absence, then confirms provider
absence before reporting deleted. Restart recovery resumes by reading current
provider state and reconciling the durable generation.

No generation may execute until the exact target system/provider, catalog service
revision, artifact digest, resource bounds, secret references, health contract,
requesting actor and rollback strategy are bound to the AgentHub ticket/review. A
policy denial, missing mapping, unavailable AgentHub, stale ticket hash, missing
provider capability, ambiguous provider outcome, failed health check or failed
read-back leaves the deployment pending/degraded/failed as appropriate and blocks
`ready`. Never store secret bytes in the definition, generation, job, result, event,
logs or API response.

### Runtime delivery packets

1. Finalize the typed deployment-instance/config contract, target capability matrix,
   idempotency semantics, event/redaction contract, and additive migration tests.
2. Implement tenant-scoped desired generations, optimistic update, request replay,
   append-only events, and read APIs. Confirm authorization and cross-tenant denial.
3. Implement AgentHub service identity and the review adapter against a verified
   typed decision/ticket contract. Exercise allow, require-review, deny, stale,
   timeout and unavailable paths before any provider write.
 4. Add a bounded, configurable builder and immutable Git-to-OCI resolution. Prove
    egress denial, CPU/memory/disk/time limits, cleanup, no credential values in
    build state or logs, artifact digest/provenance capture, cancellation and restart.
5. Add the Incus OCI adapter and verify create/configure/start/read-back/health,
   update, rollback and deletion on a disposable Incus host.
6. Add a separate Proxmox guest bootstrap/hostd adapter. Verify template capability,
   enrollment, resource bounds, create/update/restart/health/delete on a disposable
   Proxmox host. If the environment cannot provide the guest prerequisites, keep
   this provider capability visibly unsupported rather than simulating it.
7. Wire user/operator forms, dashboards, operation events and retry/rollback actions
   to those persisted APIs. Verify tenant scoping and four-action navigation.
8. Build managed PostgreSQL, Dragonfly, Redis and ClickHouse as versioned catalog
   definitions and engine-specific contracts on the verified runtime, including
   storage, credential rotation, backups, restore proof and upgrade behavior.

Packets 1–3 gate every provider write. Packets 4–6 establish a real runtime. Package
templates and product cards do not count as deployed services until provider
read-back and configured health observations prove the requested generation.

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
human-readable failure text would be brittle. Source inspection further confirms
the engine defaults to `allow=true` if no rule matches, and its response does not
bind the exact Shippin action payload or report matched rule effects. Thus the
boolean is not sufficient authorization. Define or verify a typed decision
contract bound to the exact action, system, ticket and policy hashes, and establish
service-principal permissions before connecting writes. Local Shippin approver
defaults and the `HUB_OWNER_*` bypass are removed; the example installation denies
gated actions.

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
kind. Lifecycle behavior is already separated into
`internal/hv/incus/lifecycle.go` and `internal/hv/proxmox/lifecycle.go`; preserve
that ownership and close remaining gaps with provider-specific tests and
disposable-host read-back.

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
