# Shippin.app integration plan

This plan tracks the work needed to turn this checkout into the deployable
Shippin.app platform and retire the historical checkouts. It is not a claim that
the platform, node enrollment or managed services are implemented. Preserve the
source folders, local modifications, branches and relevant ignored data until
their contents have been accounted for and the replacement has been verified.

> **Status:** written before the shippin-console import landed in PRs #9 to #14.
> `cmd/console`, `internal/` and `web/` are now in this repository, `web/` is the
> installed panel and `panel/` remains the API workbench. Where this plan
> differs, [source ownership](source-of-truth.md) is current.

## What has been established

This checkout owns the draft API in `api/openapi/`, panel in `panel/`, provider
diagnostics in `agent/`, and installation work in `install/`. The panel currently
serves an API workbench; its connection settings are browser-scoped. The agent
can read local Incus state and read Proxmox inventory using a configured HTTPS
origin and token. These diagnostics are not enrollment or lifecycle management.

The owner confirmed that the historical projects are their own code. Candidate
revisions observed during initial review were `shippin-mesh` at `26cce35`,
`shippin-cloud` at `49475d3`, and `shippin-console` at `2f5c422`. The console
checkout also has significant tracked and untracked lifecycle changes; the
revision alone does not identify that work. Re-inventory and hash the actual
working trees before each adoption. Older cloud recovery notes point to another
platform checkout and console backup branches: inspect those alternatives rather
than assuming any one copy contains everything. Do not publish private history or
installation-specific data as a side effect of local consolidation.

Shippin is a self-hosted product for other operators, not an assumption of a
commercial hosted service. Each installation configures its own infrastructure,
identities, service endpoints and policies. Gold is one operator's environment;
it is not an approval feature, a policy preset, a product-wide default or a
navigation destination. Human review is configurable in AgentHub and refers to
the same system identity used by the Shippin control plane.

| Concern | Existing evidence and candidates | Intended owner and integration |
| --- | --- | --- |
| Product API | `api/openapi/` draft; console `internal/api/` routes and cloud's historical API | One versioned contract in `api/openapi/`, updated with the implementation and compatibility tests. |
| Control plane | Console `cmd/console/`, `internal/api/`, `internal/db/`, `internal/jobs/`, auth and configuration | One durable control-plane implementation in this repository. Review complete dependency and caller graphs before selecting its package layout; adapt the console implementation where it fits. |
| Panel | Current `panel/` workbench; console `web/` customer and operator bundles | One Shippin.app panel under `panel/`; reconcile features and design, API clients, authorization and build scripts instead of maintaining two panel authorities. |
| Infrastructure | `agent/` diagnostics, console `internal/hostd/` and `internal/hostclient/`, mesh `warden/proxmox.go` | One provider-neutral infrastructure boundary with separate Incus and Proxmox implementations and explicit observed capabilities. Preserve provider-specific behavior behind it. |
| Services and deployments | Console `internal/catalog/`, `internal/hostd/service.go`; mesh `warden/catalog.go`, `warden/deployments*.go` and `templates/compose/` | One service catalog and deployment lifecycle in the control plane, with provider execution behind its infrastructure boundary. Definitions, instances, desired state and observed health have distinct responsibilities. |
| Identity and secrets | Console `internal/auth/`, `internal/authz/`, integrations and configuration | One server-side authorization boundary; identity and secret systems retain ownership of their records. Browser configuration is not a credential store. |
| Install and operations | `install/primary-image/`, historical console `deploy/` and mesh scripts | One documented developer build and supported installation path; inspect every deployment artifact for environment-specific paths, credentials and destructive assumptions. |

The platform's own data services and customer-managed offerings are separate
instances and scopes. Shippin's transactional control-plane records belong in
its own PostgreSQL instance. A self-hosted cache (evaluate Dragonfly against the
actual Redis commands and durability requirements) and a self-hosted ClickHouse
analytics store are part of the intended platform stack. ClickHouse serves
analytical workloads, not the transactional source of truth. Do not place the
only copy of job, authorization or audit state in a cache. The operator can
configure placement, credentials, networking, storage and resource limits; the
deployment must not assume shared customer infrastructure. Product choices and
compatibility must be tested against actual usage, not inferred from names.

The [ClickHouse workload overview](https://clickhouse.com/resources/engineering/oltp-vs-olap)
describes the transactional and analytical distinction. [Dragonfly's documentation](https://www.dragonflydb.io/docs)
describes its API compatibility; Shippin must test the commands and failure
semantics it actually uses. PostgreSQL backup design must include a tested restore;
see [PostgreSQL recovery documentation](https://www.postgresql.org/docs/current/continuous-archiving.html).

## Findings that change the implementation

- Console `internal/catalog/store_intents.go` records pending desired state; it
  does not provision a service. Its service implementation currently provisions
  a particular RRFlow workload inside an Incus boundary VM, not PostgreSQL or Redis.
- Warden's rollout pipeline calls an external hook and marks the deployment healthy
  after a successful hook exit without an independent service health observation.
  Its default domain and Proxmox node are installation-specific, and its older
  Proxmox client disables TLS verification by default. Do not carry those behaviors
  into the platform. A provider response or successful hook is not proof of health.
- Console catalog configuration merging needs security review before adoption:
  the bounded-number rule returns a nonnumeric child value unchanged. Validate
  types and bounds at the server boundary and test rejected and conflicting values.
- Console lifecycle work has local changes for job fencing, provider observation,
  cleanup and recovery. Preserve and review the complete patch and its tests;
  do not substitute only the committed version. Its review notes also identify
  dispatch-time authorization and real-host verification as outstanding.
- A single Compose panel service now exists, but Docker has not been available to
  verify it here. Two old runtime configuration files remain for migration.
- Historical console Gold screens, signing routes and storage are separate from
  AgentHub's versioned change-policy documents and tickets. AgentHub's current
  seed includes installation-specific rules and a named principal, and its
  evaluator recognizes a Gold-specific runner. Retire those product assumptions,
  preserve historical records as evidence, and never convert an old signature into
  a new policy decision without verified semantics. The present checkout does not
  yet implement the replacement. See the
  [system identity and human review plan](system-identity-human-review-plan.md).

## Ordered implementation and acceptance

1. **Account for the source.** Enumerate tracked files, untracked and relevant
   ignored work, branches and alternative snapshots in all three checkouts and the
   other platform checkout mentioned by the recovery index. Record each candidate's
   revision, working-tree hash, owner, dependencies, disposition and replacement.
   Check third-party attribution and dependency licenses without treating the
   owner's code as unowned. Acceptance: an inventory reconciles every relevant
   file and local change; uncertain or excluded material is preserved and explained.
2. **Establish a runnable control-plane baseline.** Review the console's config,
   migrations, auth, API, jobs, frontend and host-client call chains, including
   uncommitted changes. Adapt them to canonical owners with a clean developer
   command and disposable local dependencies. Provide configuration validation,
   startup errors, migrations, health and shutdown. Acceptance: a clean checkout
   builds and starts locally; initialization, restart, invalid configuration,
   unavailable database and migration failure are observed. No fixed tenant,
   service address or credential is required.
3. **Unify the panel and shared configuration.** Reconcile the console UI and
   existing workbench under `panel/`, with customer and operator navigation in one
   application. The server owns validated shared settings, audit and secrets
   references; UI forms expose only authorized changes and clear errors. Preserve
   existing API compatibility or document and test migrations. Acceptance: one
   panel changes a setting, another session observes it after restart, unauthorized
   and invalid changes fail, and credentials do not appear in logs or browser storage.
4. **Connect existing infrastructure safely.** Complete durable host records,
   short-lived enrollment, node identity, revocation and readiness. Reconcile the
   existing hostd implementation with `agent/`; do not assume a Proxmox host must
   use the same transport or agent placement as an Incus host before validating it.
   Inspect existing provider resources without changing cluster membership, disks,
   networking or workloads. Acceptance: UI adds and reconnects an authorized
   disposable node of each provider; expired, replayed, revoked and untrusted
   credentials fail; pre-existing resources remain unchanged.
5. **Implement provider lifecycle.** Define provider-neutral placement, inventory,
   jobs and observation contracts, with explicit provider capabilities. Adapt
   Incus lifecycle and Proxmox operations from reviewed sources, including task
   completion and uncertain-result recovery. Acceptance: create, inspect, start,
   stop and remove isolated workloads through the panel on both providers; verify
   actual provider state, authorization, interrupted requests, offline nodes and
   idempotent retry. Do not schedule on unverified or revoked nodes.
6. **Deliver the service catalog.** Reconcile catalog definitions, policy and
   attachments with the existing API schema. Implement a durable, authorized
   deployment job with immutable template/version references, validated user
   inputs, secret references, placement, idempotency, observed health and cleanup.
   Review Warden templates and third-party dependencies individually before
   adapting them; catalog availability is not runtime health. Acceptance: failed,
   partial, stale and unauthorized operations remain visibly distinct from ready;
   restart and retry cannot silently duplicate a deployment.
7. **Ship PostgreSQL and Redis first.** Provide individually reviewed, versioned
   service definitions with operator-selected placement, storage, network, resource
   limits, secrets, persistent volumes and connection information. PostgreSQL
   needs backup and restore; Redis needs explicit persistence and recovery choices.
   Acceptance: deploy each through the UI on authorized disposable infrastructure,
   connect using scoped credentials, restart and recover data, perform backup/restore
   where supported, and demonstrate failed deployment and cleanup. Add other
   services only after each has equivalent configuration and lifecycle evidence.
8. **Deploy Shippin's own data stack.** Define and implement a self-hosted
   PostgreSQL installation, a validated cache implementation and ClickHouse
   analytics as separately configured platform services. Establish how events
   reach analytics without dual-write data loss; provide replay, retention and
   tenant-scoped queries. Configure private networking, credentials, persistent
   storage, resource limits, metrics, backups, restore and upgrades. Database
   startup, migrations and recovery must not depend on a functioning tenant
   service catalog. Acceptance: a clean platform installation starts its own
   stack, survives service restarts, restores representative data and reports
   outages accurately. Prove cache loss cannot lose authoritative records and
   an analytics outage cannot silently lose acknowledged events. Test tenant
   isolation and data deletion across both transactional and analytics stores.
9. **Make the developer and release paths repeatable.** Add Lefthook, Biome and
   Zod to their appropriate owners; update CI, lockfiles and documented commands
   without creating duplicate schema authorities. Verify clean installation,
   first-owner setup, upgrade, rollback or recovery, artifact provenance and backup.
   Acceptance: clean-checkout builds and checks pass and a documented isolated
   end-to-end environment exercises the real UI, API, database and both providers.
10. **Archive only after reconciliation.** Compare the inventory with adopted code,
   tests, history and durable data; resolve every open item and independently
   verify backups and restore. Document the final replacement and archive procedure.
   Do not stop live services or remove source folders as a byproduct of this plan.

Run work in small, complete vertical slices across contract, storage, backend,
provider and UI, with relevant tests and explicit failure handling. Record actual
commands, results, source hashes and remaining gaps under `.ai/data/` as described
in [governance](../governance/README.md). A mock, build or successful schema check
alone does not establish real-host deployment or isolation.
