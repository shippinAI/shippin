# Shippin platform absorption plan

Status: active execution plan, refreshed 2026-11-05 during the Incus storage provider slice.
This describes the target and acceptance gates; it does not claim unimplemented
workflows are live. The [capability ledger](capability-absorption.md) holds
source-by-source evidence and dispositions. Checks run `41965325001` passed all
13 jobs for `f2e9ff3`; earlier runtime authorization and source-review updates
also passed their respective CI runs.

## Current execution checkpoint (2026-11-05)

The last confirmed `main` is at `690366e`. GitHub Checks run `41965589095`
passed all 13 jobs. The structured Incus OCI runtime editor update preserves all one-panel
boundaries and leaves the worker gated; previous code run `41963346185` passed
all 13 jobs for `537f304`, and documentation run `41963735539` passed all 13.
No PRs are open. PR 17 is closed without merge, but its reserved
`198.51.100.1` fixture correction is already present in `main`, so no repair
branch remains to merge. Windows standalone builds of `cmd/console` and
`cmd/shippin-hostd` succeeded; the React tests, build and bundle budget passed;
`go vet` passed. The full Windows Go suite still fails in unrelated app,
enrollment and hostd tests that assume POSIX shell/path/permission behavior or
fetch a live third-party URL. `scripts/check.sh` now discovers either `go` or
`go.exe`; its complete local check passed from Git Bash after that fix. Windows
console test selection remains build-only by design, and no claim is made about
the unrelated full-suite failures from earlier direct runs. No live Incus, Proxmox, AgentHub,
Vaultix, Cloudflare or Tailscale operation was run. The deployment worker still
makes no provider call, and archive readiness remains open. Commit `537f304`
adds lease-fenced persistence for worker review-decision provenance, with
PostgreSQL coverage for expired, replaced and current lease owners. The worker
does not call that method: just-in-time AgentHub authorization remains gated on
a complete runtime executor.

PR 17 remains closed unmerged, but its CGNAT fixture correction is present on
`main` as an independently authored equivalent patch (`b3992ab`); no open PR
was found. The Incus driver now has a tested OCI application-container primitive with
deployment-generation identity checks and retry reconciliation: it
requires an immutable `sha256` image reference, validates resource inputs,
preflights Incus 7.3.0 or newer, waits for the create operation, and confirms
the OCI marker and requested run state by reading the instance back. It is not
wired to the catalog worker. The Incus provider primitive now optionally
creates and attaches a deployment-named custom data volume in an explicitly
selected storage pool, then verifies volume ownership/size and device read-back.
It rejects invalid configuration before provider access and refuses to recreate
a missing volume for an existing instance. This remains a provider primitive:
there is no target-specific pool selector, worker call, network exposure,
health check, secret delivery, update, backup, or removal ownership; it is not
a deployable catalog runtime yet. Proxmox health now
rejects malformed version responses and versions outside the documented 8/9
major support window. The implementation is
limited to the Incus provider and follows Incus' [OCI remote and application
container workflow](https://linuxcontainers.org/incus/docs/main/howto/images_remote/)
and [application-container launch path](https://linuxcontainers.org/incus/docs/main/howto/instances_create/).

## Ordered delivery queue

The order below is a dependency order. Each row closes only when its observable
acceptance checks pass; a panel or catalog screen alone does not close a runtime
capability. Commits through `3efd1a7` are on `main`; CI runs `41963346185` and
`41963735539` passed all 13 jobs for `537f304` and `3efd1a7` respectively.
AgentHub ticket preparation, exact action authorization and safe
decision provenance are implemented, but production least-privilege identity
verification and worker-side authorization immediately before a provider write
remain gates. The deployment outbox and generation-keyed worker are implemented;
without a typed executor, the worker records `runtime_unsupported` and makes no
provider call. Commit `f076cbd` adds a lease-fenced durable unsupported
outcome. Commit `d9f1c24` adds tenant-scoped Vaultix reference parsing and a
one-key audited resolver boundary. Typed runtime execution, worker-side
just-in-time authorization, provider I/O, read-back and deployment health are
still absent. Commit `668aa56` adds a validated version 2 Incus OCI recipe
across catalog persistence, API/OpenAPI, immutable generation types and the
existing catalog editor; it remains declarative and cannot enable provider
writes by itself. Commit `537f304` adds lease-fenced persistence for future
worker authorization decisions, but is not wired into the unsupported worker.

| Order | Work package | Canonical owners | Exit evidence | Current state / gate |
|---|---|---|---|---|
| 0 | Source inventory, one-panel ownership, migration rules and capability ledger | `docs/design/`, `third_party/warden-ct102` reference matrix | Every candidate row has source revision, observed behavior, destination, disposition, rights/security status and acceptance evidence | In progress. Sibling trees remain read-only; Warden matrix still has open rows; archive is prohibited until these close. |
| 1 | Provider declarations, credential-safe edits/tests, Incus and Proxmox guest lifecycle, resources and operation read-back | `internal/servers`, `internal/hv/{incus,proxmox}`, `internal/api`, `web/` | Disposable Incus and Proxmox lifecycle workflows, edit/test denial, read-back, task failure, auth denial and secret redaction | Core lifecycle and shared storage inventory exist. Proxmox physical disk listing and capacity are read-only. Full provider coverage and disposable-host evidence remain open. |
| 2 | Stable Shippin system identity and AgentHub review authorization | system/identity owner, AgentHub adapter, API and audit | Exact action + system + resource + ticket hash + policy version decision; least-privilege identity; no-match, stale, denied, timeout and restart all block writes | Shippin identity mapping, generation-bound ticket preparation, exact action authorization and append-only decision provenance are implemented and covered by API/store tests. Disposable AgentHub coverage used synthetic principals. Production least-privilege principal permissions, target-installation timeout/restart behavior, and worker-side just-in-time reauthorization before a future provider write remain open; provider mutation remains unavailable. |
| 3 | Deployment reconciliation and provider execution | `internal/deployments`, migrations, hostd, provider drivers, jobs, API/OpenAPI, `web/` | Transactional dispatch, fenced lease, immutable generation, provider-specific executor, restart/retry, read-back, health, idempotent removal and negative-path evidence | Outbox relay, generation-keyed jobs, stale-generation handling and lease-fenced unsupported outcomes are implemented. Incus OCI v2 recipe is committed and declarative. Commit `537f304` adds a lease-fenced review-decision persistence method, but the worker does not call it. Execution, JIT authorization, provider I/O, read-back and deployment health remain absent. |
| 4 | Pinned source resolution and bounded builds | catalog/builder owner, isolated worker, secret resolver | Immutable input commit/digest, build provenance, bounded CPU/memory/time/storage/concurrency/egress, secret mounts, cancellation and cleanup | Not implemented. Must precede GitHub/custom Git deployment; OCI digest path still needs provider compatibility. |
| 5 | Managed PostgreSQL, Dragonfly, Redis and ClickHouse | managed-service owner, hostd/provider adapters, catalog, `web/` | Engine-specific configure/provision/upgrade/backup/restore/health/delete on supported providers; tenant isolation; restore verification | Design only; `internal/managed` is rrflow state, not an engine runtime. No turnkey engine may be advertised. |
| 6 | Cloudflare Tunnel, Tailscale and custom connectivity management | `internal/networking`, API, migration, `web/` | Per-provider add/edit/test/rotate/revoke and supported mutations, stable system attachment, permission denials, redaction and observed-vs-managed state | Configured inventory/refresh exists. Provider mutation/attachment lifecycle remains open. |
| 7 | Dynamic service health and resource views | platform-health registry, provider read APIs, `web/` | UI-managed service definitions; bounded persisted probes; truthful unknown/stale/down states; provider resource reads and independent failure display | Probe registry/configuration/history exists. Resource joins are partial; secret-backed auth and richer service metrics remain open. |
| 8 | Sole-panel workflows and clean developer build | `web/`, `cmd/console`, installer/deploy, docs | 3–4 action navigation, configuration persistence, no dead controls, clean checkout build, standalone executable and install/upgrade evidence | One React panel exists. Full end-to-end workflows and archive-ready clean build remain open. |
| 9 | Archive readiness | capability ledger and source owners | All useful source rows dispositioned, source changes/dirty user data preserved, rights and dependencies resolved, clear archive checklist | Not ready. Do not delete, reset, move or shut down source folders before user reviews the completed checklist. |

### Immediate execution packet (current)

1. Keep historical source trees read-only and update provenance/disposition rows
   before adopting any candidate capability. Do not archive any source.
2. **Completed safety boundary:** every worker-written unsupported deployment
   outcome is fenced by its current unexpired job lease and desired generation in the
   same database transaction. API/store tests prove an expired or replaced
   worker cannot append an event or change observed state and a current lease
   can persist `runtime_unsupported`. This slice performs no provider I/O.
   Commit `537f304` also adds `RecordReviewDecisionWithLease`; PostgreSQL
   integration coverage proves an expired or replaced lease cannot append
   AgentHub decision provenance. The reconciler does not call it yet.
3. Define a complete versioned runtime recipe before writing an executor: source
   digest and provider compatibility; process/config contract; resource bounds;
   persistent volume layout; exposure; health/read-back; secret references and
   rotation; upgrade, backup, restore, removal and retry behavior. Reject fields
   whose semantics cannot be enforced by the selected provider.
4. Select one first provider/runtime pair only after the recipe and external
   authorization principal are verifiable. Keep Incus OCI application containers
   and Proxmox guest bootstrap as separate adapters. Add a provider simulator and
   disposable-host acceptance before exposing any apply action.
5. **Completed secret-reference boundary, not runtime delivery:** a bounded
   Vaultix resolver accepts only tenant-scoped
   `vaultix://<environment>/<folder...>/<key>` references, derives the project
   from the tenant ID and installation prefix, allowlists configured
   environments, audits the deployment/generation before one-key `Reveal`, and
   never calls broad `Pull`. The resolver is not wired to a worker yet; SOPS
   remains unresolved. Before wiring it, finish the runtime secret-delivery
   contract, rotation behavior and least-privilege Vaultix identity evidence.
6. Continue the managed database, networking, dynamic health/resource, one-panel
   and archive work packages in order. Each commit updates this plan, the source
   ledger and the ignored evidence journal; run the repository checks before
   pushing directly to `main`, then confirm every CI job before starting another
   execution slice.

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
| C | AgentHub-configured review preparation, exact operation snapshot, safe evidence, submit/review status and policy provenance | `internal/agenthub`, `internal/api`, `internal/db/migrations`, `web/` Platform settings and deployment details | Commits `6081250` and `c196b35` provide generation-bound ticket/evidence preparation, required review context, bounded redaction and status UI; local checks and all 13 CI jobs pass (`41938627494`). Commit `39b9d80` adds owner/admin authorization recheck for the exact action/ticket/policy/system, normalized denial events, append-only `shippin_deployment_events` provenance, OpenAPI and panel feedback. Focused Go tests/vet, panel/governance checks and all 13 CI jobs pass for this slice (`41942328098`); Windows-only full-suite limitations are recorded in the ledger. Synthetic fixture identities do not prove the least-privilege production AgentHub credential. Provider mutation stays blocked until deployment credential permissions and future worker-side reauthorization are verified. |
| D | Multi-server Incus and Proxmox declarations, credential-safe edit/test, capability discovery, provider-local VM/container lifecycle, task polling/read-back and resource inventory | `internal/servers`, `internal/hv/incus`, `internal/hv/proxmox`, existing infra API/UI | Multiple configured endpoints work without source edits. Incus and Proxmox behavior stays provider-specific; authorization/audit precede writes; task failure, denial, retry, restart, deletion and observed read-back are covered on fakes and disposable hosts. |
| E | Review-gated durable deployment reconciliation from desired generation to provider execution | `internal/deployments`, prefixed migrations, `cmd/console`, hostd/provider drivers, API/OpenAPI and `web/src/features/deployments` | The design and dependency contract is specified in [`deployment-reconciliation.md`](deployment-reconciliation.md). The transactional outbox now relays identifier-only, generation-keyed jobs through the existing leader and job runner; PostgreSQL tests cover enqueue/receipt crash replay, generation separation, stale-worker fencing and an explicit `runtime_unsupported` event with no provider mutation. **Not implemented:** worker-side just-in-time AgentHub authorization, lease-loss recovery proof, job/event UI linkage, typed runtime execution or provider application lifecycle. |
| F | Pinned GitHub/custom HTTPS Git/OCI resolution and isolated builds | `internal/catalog`, bounded builder owner, secret resolver contract, deployment UI | Immutable source digest and provenance; bounded time/CPU/memory/storage/concurrency/egress; cancellation and cleanup; credentials never appear in URLs/logs; no arbitrary privileged host commands. |
| G | Turnkey PostgreSQL, Dragonfly, Redis and ClickHouse definitions and engine-specific lifecycle | existing managed-services owner, catalog/deployment contracts, provider adapters and `web/` | Explicit engine/version/image, placement, resource/storage/network/secrets; install, edit, upgrade, health, backup, restore and delete; tenant isolation and restore verification. Redis and Dragonfly compatibility is stated accurately. |
| H | Cloudflare Tunnel, Tailscale and bounded custom integrations: configure/edit/test/rotate/revoke, attach to stable systems, manage supported provider resources | `internal/networking`, its prefixed schema/API and existing infrastructure UI | Least-privilege provider scopes and exact write support proven; observed inventory remains distinct from requested changes; denial, credential rotation, redaction and stale inventory pass. |
| I | Nomen, Vaultix, Edge, Database, Operator and installation-added service health plus resources | platform health registry/observations, provider read APIs and existing overview/settings UI | UI-managed definitions; configured secret resolver only; bounded scheduling, persisted probes, freshness and provenance; resource metrics join by stable IDs. Missing or stale values stay unknown/stale and failed resource reads do not overwrite service reachability. |
| J | Re-author each remaining Warden/Mesh/Cloud/Console useful feature (backup, restore, DNS/ACME, firewall, logs, runners, templates, workspaces and diagnostics) only into its owned bounded context | Existing `internal/backupcov`, provider/networking, project/workspace, audit, jobs or catalog owners as documented per source row | Each feature receives rights/security review and actual API/schema/UI/runtime/failure evidence. Shell, root file manager, arbitrary Docker/Compose, local account management, unsafe disk controls, site IDs and parallel panel/policy are explicitly rejected. |
| K | One-panel navigation, fully configurable operator and tenant workflows, clean standalone developer build and rollout | `web/`, `cmd/console`, installers/deploy docs | Every implemented destination is reachable in at most four user actions; no dead/mock controls; config survives restart; clean checkout builds console and hostd, migrates, installs and upgrades with supported workflows proven. |
| L | Archive decision for each historical checkout | capability ledger and user-review checklist | All useful source rows closed, local dirty data retained, rights/dependencies recorded, and exact remaining gaps listed. Archiving/deleting source trees requires a separate user decision after this evidence is reviewable. |

#### Current Stage E slices: fenced outcomes and typed recipe contract

The job runner cancels a handler when its renewable lease is lost, while the
deployment store already fences writes by desired generation. A cancellation
alone is not an atomic persistence fence: a worker can lose its lease between
checking context and committing a deployment event. `RecordRuntimeUnavailable`
now has a worker-only variant that locks and validates the live
`deployment.reconcile` job row in the same transaction as the generation check,
state update and failure event. An expired or replaced worker receives a
normalized lease-loss error and writes nothing. The ordinary store helper
remains only for isolated store setup/tests; the production reconciler uses the
lease-fenced path. This proves durable outcome fencing only; it does not prove
external provider-call fencing or enable execution.

Commit `537f304` adds the matching `RecordReviewDecisionWithLease` store
boundary. It locks and validates the live deployment reconcile job before the
generation, review, ticket and append-only decision checks in the same
transaction. PostgreSQL integration coverage exercises expired, replaced and
current lease owners. It is not wired into `authorizeDeploymentAction` or the
worker; AgentHub calls still happen only in the operator review endpoint while
the executor is unavailable.

The catalog runtime declaration now has two versions. Version 1 preserves the
existing compatibility-only Incus/OCI marker. Version 2 requires CPU/memory,
persistent storage size and normalized mount path, private/Edge exposure with
declared ports, and a bounded TCP/HTTP health check. The catalog UI edits the
recipe through structured controls in the existing service form; new recipes
start incomplete instead of assuming service-specific values, and API
validation remains authoritative. Version 1 edits remain version 1. PostgreSQL
rejects incomplete version 2 shapes. Tenant discovery and immutable deployment
generations carry the validated version. This UI and contract work does not
implement provider storage/network attachment, secret delivery, an executor,
health polling, upgrade, backup/restore or removal semantics; the worker must
continue to fail closed with `runtime_unsupported` until the corresponding
executor is proven.

Each implementation slice names its stage and source rows in the commit and
journal, lists canonical files it owns, states what it excludes, and shows an
acceptance result plus a denial/failure result. If an external contract is
unavailable, keep its gate open and continue only independent work. Current
active stage is E. AgentHub ticket preparation, operator-side exact-action
authorization, lease-fenced decision persistence, desired-generation outbox and
current unsupported-runtime reporting are on `main`; local Windows and Linux CI
limitations, production AgentHub credential status and lack of a live provider
are recorded in the evidence journal. The first provider deployment remains
unimplemented and unavailable.

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
The current execution boundary, same-database outbox, worker fencing,
just-in-time AgentHub authorization, provider/runtime contracts and acceptance
tests are specified in [`deployment-reconciliation.md`](deployment-reconciliation.md).

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
