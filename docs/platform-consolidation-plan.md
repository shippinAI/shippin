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

## Current next actions

1. Add PostgreSQL-backed tests for the new network resource association, including
   invalid and revoked target, detach, reattachment and inventory omission.
2. Review the current lifecycle package and its Incus/Proxmox call graph; write a
   source-hashed implementation plan before changing lifecycle ownership.
3. Complete a provenance/dependency/license inventory for the first candidate
   folder and adopt one capability at a time under the ownership map.
4. Trace the AgentHub approval implementation from current client/policy storage
   through infrastructure provider calls and tests; do not enable writes until
   the phase 3 gate passes.
5. Maintain this document with commit/CI evidence and mark a phase complete only
   when every stated acceptance gate is met.
