# Managed data services

Status: design only. The catalog stores validated, pinned source metadata and configuration schemas. The deployments context stores desired generations and events. There is no provider executor, reconciler, managed-service runtime, backup executor, or live Postgres, Dragonfly, Redis, or ClickHouse provisioning flow. A catalog entry or accepted desired-state request does not mean a service is deployed.

## Ownership and current state

The single `web/` panel and the existing platform bounded contexts own this capability:

| Concern | Canonical owner | Current state |
|---|---|---|
| Reusable service definitions, schema, source pin and secret references | `internal/catalog` | Definitions and source metadata exist. Source fetching/building does not. |
| Tenant desired generation, target, config snapshot and event history | `internal/deployments` | Desired generations, outbox, worker and durable failure events exist. Unsupported runtimes are reported as failed; no application executor or provider reconciliation is connected. |
| Host-side bounded operations | `internal/hostd` | Existing authenticated host operations are not a managed-service runtime. |
| Incus/Proxmox provider behavior | `internal/hv/incus`, `internal/hv/proxmox` | Provider drivers exist. All provider mutations currently fail closed with `503 agenthub_review_unavailable`. |
| Human-review policy, tickets, evidence and decisions | AgentHub | Shippin has a configured adapter, tenant/system mapping, ticket workflow and exact action-bound authorization provenance. The worker does not yet reauthorize immediately before provider operations; production least-privilege identity has not been verified. |
| Secret references | Catalog validation and tenant-scoped Vaultix resolver | `vaultix://<environment>/<folder...>/<key>` derives the project from the deployment tenant and installation prefix; one-key `Reveal` is audited before reading. SOPS references remain metadata-only until a configured resolver exists. Never store secret bytes in service config or deployment events. |

Do not extend the rrflow-specific `service_instances` table into a second generic deployment registry. Do not create a parallel `/v1/infra/services` manager. Managed engines should be typed catalog definitions and deployment instances in the existing deployment owner. Engine-specific differences belong in service contracts and runtime adapters.

Historical sources reviewed include the clean `shippin-cloud` checkout at `49475d3b` and the Warden capture matrix. Those sources inform capability accounting only; their panels, endpoint defaults, local secrets files, shell executors and approval mechanisms are not product implementations. See the [capability absorption ledger](capability-absorption.md).

## Service contracts

Each supported engine needs an explicit, versioned definition with:

- immutable image or source identity and provenance;
- engine version, compatible provider targets and installation-selected placement;
- bounded CPU, memory, storage and replica settings where the engine supports them;
- network exposure and access policy;
- secret references, health checks and resource observations;
- upgrade, backup, restore and deletion behavior, including unsupported cases.

PostgreSQL, Dragonfly, Redis and ClickHouse must not be treated as interchangeable containers. PostgreSQL requires a consistent backup and verified restore. Dragonfly and Redis must expose their own persistence and replication semantics. ClickHouse needs its own storage, user and recovery contract. Catalog examples are editable definitions, never server-specific deployments or promises that a runtime exists.

## Persistence and migrations

Use the existing `shippin_deployment_instances`, generation and event owners for desired and observed deployment state. Add only engine-specific tables when a distinct durable concern has no existing owner. New tables use the `shippin_` prefix; backup records, if introduced, must reference a deployment and exact generation and contain storage references rather than credentials or payloads.

Do not widen or repurpose rrflow tables to represent managed engines. Before adding schema, document additive migration behavior, tenant isolation, idempotency, audit, retention, rollback and restore. Do not infer deployed instances from historical hostnames, addresses, tenant IDs, compose files or source-control state; an operator must explicitly enroll or create each target.

## Review and provider execution

AgentHub owns configurable review policy. Shippin must submit the exact action, stable Shippin system ID, provider resource, desired-generation hash, policy and ticket provenance, evidence and rollback data. Only a typed, current AgentHub decision bound to that request can authorize execution. A local Shippin task-board card, boolean from an unverified decision response, ticket status, hostname, role name or UI confirmation is not provider authorization.

Shippin's AgentHub decision adapter validates the exact action, current ticket and policy and records safe provenance, but production least-privilege identity and worker-side just-in-time authorization are not verified. Therefore provider mutations remain disabled. Do not advertise an accepted intent as running until the worker gate and a provider-specific executor are proven. Each action must be idempotent, generation- and lease-fenced, audited before the side effect and confirmed by provider read-back and configured health observations. Denial, timeout, stale review, changed policy, provider failure and restart must retain an honest pending, failed or unknown state.

## Panel workflow

The deployment page in `web/` is the only service configuration surface. It discovers services and tenant-authorized targets, renders validated configuration forms, shows pinned source and desired generation, and distinguishes provider-observed state with event history. Editing a definition or requesting a deployment does not imply that a provider operation ran. Review status comes from AgentHub; Shippin does not offer a local-board approve or retry-and-apply control. Vaultix references name an environment and key path only; the tenant project is resolved from the authenticated deployment tenant. The SOPS shape is accepted as metadata but has no deployment resolver yet.

Engine-specific operations such as backup, restore, scale and upgrade become available only when their API, authorization, executor, persistence, read-back, failure recovery and panel workflow are implemented for that engine.

## Backup source review

The Warden reference contains estate-backup concepts for S3, SMB, Infisical, preflight, coverage and event history. The old shell panel is not served. Its secret file and shell-out transfer executors are excluded. The 2026-11-05 source review found the captured backup subsystem has no Git history, uses a local JSON store and estate-specific paths, executes configurable tar/rsync commands (including rsync --delete), and does not prove the platform tenant, AgentHub, audit, generation or secret-resolver boundaries. Its S3/SMB/Infisical code and tests are rejected as implementation sources; this does not close the product backup requirement. Re-author bounded destination metadata, secret references, coverage facts and redacted events under the appropriate deployment or backup owner after separate provenance, dependency, license and threat review. Hypervisor snapshots remain distinct from engine-consistent database backups.

## Acceptance gate

For each engine, a self-hosted operator must be able to configure a supported version and target, obtain AgentHub authorization for the exact generation, deploy it through a bounded provider adapter, observe provider read-back and health, rotate credentials, perform and verify backup/restore, upgrade and remove it. Tenant isolation and negative paths must be tested against disposable providers and PostgreSQL. Until that evidence exists, the capability remains design-only and is not turnkey.
