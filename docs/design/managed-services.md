# Managed data services

Status: design only. The catalog stores validated, pinned source metadata and configuration schemas. The deployments context stores desired generations and events. There is no provider executor, reconciler, managed-service runtime, backup executor, or live Postgres, Dragonfly, Redis, or ClickHouse provisioning flow. A catalog entry or accepted desired-state request does not mean a service is deployed.

## Ownership and current state

The single `web/` panel and the existing platform bounded contexts own this capability:

| Concern | Canonical owner | Current state |
|---|---|---|
| Reusable service definitions, schema, source pin and secret references | `internal/catalog` | Definitions and source metadata exist. Source fetching/building does not. |
| Tenant desired generation, target, config snapshot and event history | `internal/deployments` | Persistence and UI intent workflow exist. Observed state remains unknown; no executor or reconciler is connected. |
| Host-side bounded operations | `internal/hostd` | Existing authenticated host operations are not a managed-service runtime. |
| Incus/Proxmox provider behavior | `internal/hv/incus`, `internal/hv/proxmox` | Provider drivers exist. All provider mutations currently fail closed with `503 agenthub_review_unavailable`. |
| Human-review policy, tickets, evidence and decisions | AgentHub | Shippin has no adapter. Local task-board policy cannot authorize provider writes. |
| Secret references | Catalog validation and configured Vaultix/SOPS reference forms | Runtime secret resolution is not implemented. Never store secret bytes in service config or deployment events. |

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

The AgentHub decision contract and least-privilege service identity are not verified. Therefore provider mutations remain disabled. Do not implement a managed-service executor or advertise an accepted intent as running until that gate and the deployment reconciler are proven. Once available, each action must be idempotent, generation-fenced, audited before the side effect and confirmed by provider read-back and configured health observations. Denial, timeout, stale review, changed policy, provider failure and restart must retain an honest pending, failed or unknown state.

## Panel workflow

The deployment page in `web/` is the only service configuration surface. It should discover services and tenant-authorized targets, render validated configuration forms, show pinned source and desired generation, and distinguish provider-observed state with timestamps and event history. Editing a definition or requesting a deployment must not imply that a provider operation ran. Review status must come from AgentHub; Shippin must not offer a local-board approve or retry-and-apply control.

Engine-specific operations such as backup, restore, scale and upgrade become available only when their API, authorization, executor, persistence, read-back, failure recovery and panel workflow are implemented for that engine.

## Backup source review

The Warden reference contains estate-backup concepts for S3, SMB, Infisical, preflight, coverage and event history. The old shell panel is not served. Its secret file and shell-out transfer executors are excluded. Re-author only bounded destination metadata, secret references, coverage facts and redacted events under the appropriate deployment or backup owner after checking provenance, dependency and license status. Hypervisor snapshots remain distinct from engine-consistent database backups.

## Acceptance gate

For each engine, a self-hosted operator must be able to configure a supported version and target, obtain AgentHub authorization for the exact generation, deploy it through a bounded provider adapter, observe provider read-back and health, rotate credentials, perform and verify backup/restore, upgrade and remove it. Tenant isolation and negative paths must be tested against disposable providers and PostgreSQL. Until that evidence exists, the capability remains design-only and is not turnkey.
