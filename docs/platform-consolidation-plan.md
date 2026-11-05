# Shippin platform consolidation plan

## Objective

Deliver one deployable, operator-configurable Shippin platform from the canonical
`shippin-platform` repository. All product UI lives in `web/`. Incus and Proxmox
are explicit infrastructure providers. Users can configure provider connections,
services, deployment sources, networking, identity and policy from the panel.
Historical folders remain source material until their useful behavior has been
reviewed, re-authored into canonical ownership, and verified; this plan does not
authorize blind copying or deletion.

## Operating constraints

- Commit directly to `main`; run `bash scripts/check.sh` before changes and repair
  a red `main` before adding new work.
- Assign one canonical owner to each capability. Do not add another panel or
  import an alternate console wholesale.
- Review source provenance, license, dependencies, security posture, tests and
  runtime assumptions before adopting historical code. Public visibility is not
  a license.
- Do not encode installation addresses, credentials, tenant IDs, project names,
  provider IDs or server-specific policy in product code. Use validated,
  persisted configuration and sealed write-only credentials.
- Separate desired configuration from provider observations. Keep changes
  revisioned, auditable, idempotent, lease-fenced where asynchronous, and
  recoverable after process restart.
- AgentHub owns human approval. Mutations must bind approval to the exact target,
  action and configuration hash. An unavailable policy/reviewer fails closed.
- Archive source folders only after the evidence gate at the end of this plan;
  do not delete user work as a shortcut to consolidation.

## Canonical ownership map

| Concern | Canonical owner | Required boundary |
| --- | --- | --- |
| Operator and tenant panel | `web/` | One React application, shared navigation and design system |
| HTTP routes, auth and orchestration | `internal/api/` | Validate requests and delegate to domain packages |
| Incus lifecycle | `internal/hv/incus/` | Incus-specific client, translation and lifecycle operations |
| Proxmox lifecycle | `internal/hv/proxmox/` | Proxmox-specific client, translation and lifecycle operations |
| Provider-neutral lifecycle contract | `internal/hv/` | Shared typed capabilities and errors; no provider-specific branches in generic lifecycle code |
| Server identity and provider configuration | `internal/servers/` plus migrations | Stable system identity, aliases/tags, endpoint configuration, secret references |
| Service definitions and deployments | `internal/catalog/`, `internal/deployments/` | User-managed source, version, schema, runtime and deployment state |
| Network integrations and observations | `internal/networking/` | Cloudflare Tunnel, Tailscale, custom inventory; desired links separate from observations |
| Platform services and health | `internal/db/` plus `internal/api/` domain handlers | Revisioned editable definitions, leases, observations and operator read models |
| Human approval and provenance | `internal/agenthub/` | AgentHub-backed task, approval, exact action hash and journal |
| Packaging and developer build | `cmd/`, `install/`, `deploy/`, `scripts/` | Reproducible build, migration path, upgrades and health checks |

## Ordered work and acceptance gates

### 0. Baseline, inventory and source review — required before every adoption batch

Read the repository instructions, `docs/source-of-truth.md`, current roadmap,
complete candidate files, callers, tests, manifests and build/deploy configuration.
Record an inventory per source folder (`shippin-mesh`, `shippin-cloud`,
`shippin-console`, and historical Warden): capability, source commit and file
hash, current owner, proposed canonical destination, dependencies, license,
security concerns, tests, disposition and evidence. Keep the audit in ignored
`.ai/data/`; retain only durable, sanitized architecture decisions in this plan.

**Gate:** no candidate is copied or moved until its origin, license, secret
handling, tests and canonical owner are understood. Unknown license or provenance
means reimplement from the desired behavior or defer adoption.

### 1. Stabilize the single deployable and panel

Map existing routes, navigation, build targets, migrations and service boundaries
to the ownership table above. Resolve duplicate UI and service owners within
`shippin-platform`; do not import a second console. Define the supported developer
build and installer entrypoints, configuration schema, migration ordering,
health/readiness semantics and supported upgrade path.

**Gate:** one documented developer command builds the one panel and server;
clean-install and upgrade checks pass; no runtime dependency on sibling source
folders remains.

### 2. Finish provider-neutral lifecycle and explicit Incus/Proxmox drivers

Refactor shared lifecycle orchestration into the provider-neutral `internal/hv/`
contract and provider behavior into `internal/hv/incus/` and
`internal/hv/proxmox/`. Keep discovery, validation, capabilities, errors,
operation persistence, polling and recovery explicit. Confirm the UI can add,
edit, test and remove connections using user-supplied endpoints, certificates,
credential references and provider options. No environment-specific address or
default machine identity may be embedded in product behavior.

**Gate:** contract tests cover each supported operation and unsupported path;
provider tests exercise positive and negative TLS/auth/API cases; database tests
cover operation restart, duplicate delivery and stale lease rejection.

### 3. Make AgentHub the sole human-review authority for provider mutations

Replace blanket-denied infrastructure mutations only after an end-to-end
AgentHub task and approval path exists. Bind approval to actor, system/server ID,
provider, exact action, target, configuration revision/hash and expiry. Persist
provenance and audit outcome; re-check the fence immediately before executing the
provider call. Keep hostname as discovered system identity, with user-configurable
label, alias and tags. AgentHub tenant/machine mapping remains distinct from
invented product-specific approval registries.

**Gate:** integration tests prove approved actions execute once, altered targets
or hashes are rejected, expired/replayed approvals fail, AgentHub outage fails
closed, and audit failure prevents mutation. Read-only probes remain available
without manufacturing approvals.

### 4. Complete configurable service catalog and turnkey deployments

Use a versioned, validated catalog for built-in and user-added Git/custom
deployment sources. Store source URL/ref/digest and deployment parameters as
configuration, with secret values in the platform secret boundary. Add editable
service definitions and deployment forms in `web/`. Implement lifecycle/status
and recovery as persisted operations, not mock buttons. Treat PostgreSQL,
Dragonfly/Redis and ClickHouse as independently configurable catalog offerings;
document resource requirements, ports, storage, backups, upgrades, health checks
and supported HA modes. Do not imply a service is deployable until its runtime,
provisioning path and failure recovery are implemented and verified.

**Gate:** each shipped catalog item has source/license review, schema validation,
version pinning, secret redaction, install/upgrade/rollback behavior, smoke test,
backup/restore evidence and a documented support boundary. Users can add a new
repository or custom source without code edits.

### 5. Finish network integrations as desired and observed state

Manage Cloudflare Tunnels, Tailscale and custom integrations through validated
UI configuration, scoped credentials and safe connection tests. Represent
provider observations separately from Shippin desired links and system identity.
Support explicit association, status, stale inventory, removal and audit. Any
provider-side change uses the AgentHub approval path from phase 3. Define
canonical `shippin_` prefixes for new platform-owned tables and migrations.

**Current slice:** commit `8d3f81c` adds provider-resource-to-server association
in the existing React panel, audited routes, a prefixed desired-state table and
retention across temporary observation gaps. Commit `96c4443` updates the pinned
unauthenticated route inventory. CI run `41982881794` passed all 13 jobs. These
changes associate local Shippin metadata only; they do not configure tunnels,
routes, ACLs or providers. Add database-backed regression tests for attach,
detach, target rejection and inventory omission before treating this capability
as fully covered.

**Progress:** the association workflow has PostgreSQL-backed coverage for an
unknown external resource, revoked target, attach, detach, reattachment and an
empty inventory snapshot. The network package is included in the existing
disposable-Postgres CI suite. Local Postgres 16 execution passed.

**Gate:** integrations can be added, edited, tested, synchronized and removed;
resource associations survive transient inventory gaps; forbidden targets and
provider writes fail safely; credentials never appear in responses or logs.

### 6. Make Nomen, Vaultix, Operator and health fully configurable

Inventory each platform service's actual runtime and API, then provide canonical
create/edit/disable/test forms for settings that are safe to expose. Health must
show configuration revision, check time, freshness, latency, resource usage,
address/provenance when available, and `unknown` when evidence is missing or
stale. Join Incus state only by stable system ID and retain per-service errors.
Avoid displaying placeholder zero values as live measurements.

**Gate:** UI mutations persist, survive restart and are audited; invalid config
and unreachable services produce useful failure states; stale probe results
cannot overwrite a newer revision; secrets are sealed and write-only.

### 7. Packaging, independent evaluation and archive readiness

Run clean Linux developer builds, fresh installs, upgrades, restore/recovery,
secret scans, database migrations, provider simulators and UI workflows. Capture
actual deployment targets and logs with secrets redacted. Evaluate on independent
projects/systems not used to author the integration. Produce an archive ledger
mapping every candidate capability/file to its canonical implementation or
explicit rejected/deferred disposition, with tests and source evidence.

**Gate:** all shipped work builds from `shippin-platform` alone, CI and
install/upgrade tests are green, supported operations are documented, no
candidate source contains unique unreviewed capability, and the user can inspect
the archive ledger. Only then is archival of the external folders safe to
consider; deletion is a separate explicit action.

## Execution register

This register is the active work order. It deliberately separates implemented
metadata and design from behavior that has actually been exercised. Complete
each packet in order, update its status with commit and CI evidence, and do not
start a dependent packet while its gate is open. Source-by-source provenance and
dispositions remain in [`design/capability-absorption.md`](design/capability-absorption.md);
the architecture and UI path remain in [`design/platform-absorption-plan.md`](design/platform-absorption-plan.md).

| Order | Packet and canonical owner | Current status | Implementation order and observable exit gate |
| --- | --- | --- | --- |
| 0 | Baseline and one deployable: `web/`, `cmd/console`, `cmd/shippin-hostd`, `install/`, CI | **Passed locally** on `main` `d01f51c`; `bash scripts/check.sh` passed on 2026-11-05. Full Linux CI is tracked separately. | Keep all panel work inside `web/`; verify clean standalone control-plane and host-agent artifacts, install/upgrade/migration flow, and no sibling checkout runtime dependency. Record Linux CI and installation evidence. |
| 1 | Source inventory and rights: historical `shippin-mesh`, `shippin-cloud`, `shippin-console`, and Warden-named source; ledger in `.ai/data/` | **Partially audited; not archive-ready.** Existing ledger records revisions and broad feature families; candidate-level rights/dependency/test review is incomplete. | Inventory every candidate path by source revision/hash, behavior, dependency/license, security boundary, current canonical owner, implementation evidence and disposition. Read-only review only; no wholesale import, reset, merge, or deletion. Exit when no useful capability or user-local change is unresolved. |
| 2 | Provider mutation approval: `internal/agenthub/`, `internal/api/infra*`, `internal/servers/`, one `web/` guest workflow | **Single-guest power review and configurable local expiry are implemented and PostgreSQL-tested; live AgentHub/provider behavior remains unverified.** Start, stop, shutdown and reboot require exact-action AgentHub review, a configured 1–1440 minute Shippin execution window (default 30), one-shot consume, and durable operation/review provenance. Expiration is persisted per review and atomically rejected/journaled before provider execution. This does not close or expire the AgentHub ticket. Batch and all other provider writes remain fail-closed. | Confirm live AgentHub rule/quorum semantics; inspect one-shot recovery and operator/tenant authorization against independent negative cases. Then exercise against disposable Incus and Proxmox systems before enabling/claiming live behavior. Extend operation-by-operation only. |
| 3 | Incus and Proxmox lifecycle: `internal/hv/{incus,proxmox}/`, `internal/hv/`, infra API and UI | **Drivers and lifecycle files exist; all current writes remain disabled.** | Finish guest power first; then create/configure/clone/delete, snapshots/restore, storage and networking as distinct typed provider capabilities with provider-specific contracts. Test unsupported paths, auth/TLS failures, idempotency, operation recovery, read-back and disposable Incus and Proxmox behavior. UI add/edit/test/remove uses configured endpoints, certs and credentials only. |
| 4 | Deployment runtime and source builder: `internal/deployments/`, `internal/catalog/`, `internal/jobs/`, `internal/hostd/`, deployment UI | **Desired generations, source metadata, outbox, jobs, fences and review helpers exist; executor is deliberately `runtime_unsupported`.** | Specify and implement a bounded isolated Git/custom builder with immutable source/artifact provenance and configured limits; then tenant-safe typed Incus OCI executor and separate supported Proxmox guest runtime. Add JIT AgentHub immediately before provider writes, secret delivery policy, reconciliation/read-back, health, cancellation, update, rollback and deletion. Keep unsupported targets visibly unsupported. |
| 5 | Turnkey data services: catalog/runtime adapters and deployment UI | **Design/catalog groundwork only; no verified turnkey service deployment.** | On the verified runtime, implement PostgreSQL, Dragonfly, Redis and ClickHouse as explicit engine-specific versioned offerings. Configure compute, storage, placement, network, users, secret rotation, health, upgrades, backups and actual restore tests. Never call a catalog card deployed until provider read-back and health prove it. |
| 6 | Networking: `internal/networking/`, `shippin_network_*` schema, infrastructure UI | **Configuration, inventory sync and local resource association exist; remote tunnel/device/routes writes do not.** | Finish add/edit/test/rotate/revoke and permission-scoped writes for Cloudflare Tunnel and Tailscale; define a minimal validated custom integration contract. Persist desired links apart from observations; verify stale inventory, removal, tenant/target denial, audit and secret redaction. |
| 7 | Platform services and health: platform service domain, API and existing settings/overview UI | **Editable registry, bounded probes, observations and stale/unknown state exist; some probes/resource integrations remain incomplete.** | Complete Nomen, Vaultix, Operator, service catalog and platform deployment configuration through persisted create/edit/disable/test APIs. Replace missing measurements with `unknown`, expose freshness/revision/provenance and errors, enforce revision fences, and test persistence across restart. No installation-specific address or credentials in defaults. |
| 8 | Dashboards and navigation: the existing `web/` feature routes | **Separate infrastructure, health, services and deployment views exist; unified live deployment/resource history remains incomplete.** | Join only by stable system/service/deployment IDs. Build operator views for provider resources, service instances, operation events and health; preserve role/tenant access and keep each destination within four actions from its parent workflow. Validate API-backed behavior and failure/empty/stale states. |
| 9 | Developer build, rollout, evaluation and source retirement: `scripts/`, CI, installer, docs and absorption ledger | **Local repo checks pass; standalone rollout and live provider acceptance are not yet evidenced.** | Verify Linux release binaries, clean install, upgrade, backup/restore, secret/public-tree scan, migration replay and disposable-provider workflows. Use independent evaluation targets. Finish every source ledger row and rights review, then produce archive readiness evidence. Source deletion is separate and is not authorized by this plan. |

### Current execution slice

Packet 2 now has a locally implemented slice for single-guest start, stop,
shutdown and reboot. It persists exact action and endpoint hashes, creates and
submits a mapped AgentHub ticket with evidence, rechecks the submitted ticket
hash and current decision before execution, consumes the review once, and joins
the existing operation journal. Operator and tenant routes use the same React
dialog in `web/`. Disposable PostgreSQL tests cover replay/conflict, changed
ticket binding, pending review, approval, outage, one-shot execution and
provenance. Local Go tests, disposable PostgreSQL, panel build, full
`bash scripts/check.sh`, and GitHub Checks run `41987565565` for commit
`223add2` pass. A follow-up adds an audited operator setting for the local review
window, a fixed database-clock expiry on each request, an append-only expired
event, and atomic expiry rejection at claim. PostgreSQL-backed API tests confirm
an AgentHub-passed but locally expired ticket never calls the provider. The local
window does not close the AgentHub ticket. Live AgentHub policy/quorum and
Incus/Proxmox calls remain unverified. Batch and all other provider writes remain
fail-closed. The
source-hashed handoff is stored in the Git-ignored session
`.ai/data/sessions/2026/11/9db05ac5-2958-4ec6-95c2-f5d9e9692ece/`.

### Evidence update rules

Each packet update names the owning files and migration, tests actually run,
local environment limits, resulting main commit, and GitHub workflow conclusion.
Do not mark a packet complete from schema presence, mocked UI, accepted HTTP
requests, or an agent's summary. If the provider or AgentHub environment is
unavailable, keep that behavior explicitly unverified and continue only with
independent work that does not depend on it.
