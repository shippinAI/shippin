# Proxmox capability absorption

This review compares the Proxmox panel in the local mesh source with the canonical
Shippin hypervisor boundary. It records behavior and ownership; it does not import
the source implementation. The source repository had no root license file in the
reviewed tree, so its code and dependencies remain outside this repository.

## Historical WardenClyffe backup candidate

The separate local `D:\localdev\shippin` checkout is on
`feat/clyffe-code-live-engine` at `3fd18ac0`, with 62 tracked paths modified or
deleted and 26 untracked paths. Its root README describes a Proxmox-backed
WardenClyffe product and links an MIT license. The backup source and tests below
are part of that dirty local state; this review did not modify the checkout and
does not establish the provenance or license of bundled assets or external
tools.

| Candidate behavior | Review finding | Platform disposition |
|---|---|---|
| Proxmox `vzdump` guest backup and backup-volume listing | The current platform already has provider-specific `Vzdump` and backup-content read methods in `internal/hv/proxmox/phase1.go`. The historical service starts a task on a defaultable single node and launches an in-process watcher; restart recovery and durable Shippin job leases are absent. Its target/storage/mode inputs are not an installation-neutral persisted backup policy. | Reuse only the existing provider API contract after an AgentHub-reviewed durable operation and provider read-back. Do not adopt the service wrapper or its task watcher. |
| S3-compatible workspace backup with Restic | The dirty implementation calls an external `restic` binary with environment credentials and an environment-selected workspace root; its default is `/workspace/warden-storage`, exclusions encode that installation's layout, and goroutine execution is not recoverable after process restart. Restic output is retained in action results and requires explicit secret-redaction review. | Treat the underlying encrypted backup idea as a requirement, not as an implementation. Define tenant/system scope, approved source paths, restore semantics, executable provenance/pinning, durable jobs, cancellation and output redaction before authoring. |
| S3 credential configuration and connection test | Credentials are written to plaintext `s3.env` files (including a repository-relative `secrets/s3.env` path when available) and overlaid with process environment. The custom SigV4 helper is local code; the test exercises a basic HEAD request but does not establish AWS/R2/MinIO interoperability or endpoint security. `newS3HeadBucketRequest` accepts configured HTTP endpoints, which would send signed requests without TLS. | Reject this secret storage and request implementation. Use the platform's configured secret owner, validated HTTPS endpoint policy, bounded provider client, redacted test errors and prefixed persistent configuration. Verify against supported S3-compatible providers before advertising support. |
| Backup action approval and status | `insertAction` advances a new action through `approved` and `executing` itself, without an AgentHub decision. It writes to the source product's `shippin_core.action_requests` schema and does not prove tenant ownership or idempotent provider dispatch. | Reject the local approval/status model. AgentHub owns review; Shippin must persist exact action provenance and use tenant-scoped durable jobs/events with lease and generation fencing. |
| React backup panel | The untracked `BackupPanel` was not accepted as a second panel implementation; this review did not complete a UI behavior, accessibility, dependency or route audit. | Review only as workflow evidence. Any adopted controls belong in the existing `web/` infrastructure or managed-service feature, backed by real APIs and honest unsupported states. |

The candidate therefore supplies useful backup requirements, but none of its
dirty implementation is eligible for direct absorption. The matching platform
gap remains real: Proxmox backup primitives exist but infrastructure mutations
are gated by `agenthub_review_unavailable`; tenant PostgreSQL backup endpoints
currently record intent without taking a dump. A supported backup feature still
needs a separately planned, review-gated durable executor and a verified restore
path. Do not claim backup capability based on the historical panel or driver
primitive alone.

## Capability mapping

| Source behavior | Canonical owner | Disposition |
|---|---|---|
| Read API version and report availability | `internal/hv/proxmox` health probe | Already implemented with verified TLS and provider error handling. |
| List cluster resources for VMs and containers | `internal/hv/proxmox` guest inventory | Already implemented with node-aware guest identity. |
| Show CPU, memory, disk, network counters, uptime, pool, template, and tags | `hv.Guest` and the existing Infrastructure guest detail card | Newly retained as provider-reported telemetry; other drivers may omit unavailable metrics. |
| Start, stop, shutdown, and reboot | Existing hypervisor lifecycle methods and Infrastructure API | Driver operations, task completion and state read-back exist. All mutation routes currently fail closed before dispatch pending AgentHub review; they are not available in the current build. |
| Create and inspect snapshots | Existing snapshot operation | Driver support exists for QEMU VMs and LXC containers. Mutation routes are gated pending AgentHub review; do not describe snapshot creation as currently available. |
| Read node and cluster status | `internal/hv/proxmox/details.go` | Already implemented; restricted cluster access leaves optional cluster details absent. |
| Read storage and networks | Existing `Storage` and `Networks` driver methods and Infrastructure routes | Already implemented. |
| API token authentication | Existing server credential sealing and per-request secret resolver | Reused; credentials are not process-global provider settings or API responses. |
| TLS verification | Shared verified TLS configuration in `internal/hv` | Reused; the source's opt-in verification switch is not carried forward. |
| Node selection and endpoint configuration | Existing validated server attachment form | Reused; no compiled node or host defaults are carried forward. |
| Standalone HTML page, environment lookup, and direct action handlers | None | Not adopted; they duplicate the React panel and bypass the platform's provider, audit, and authorization boundaries. |

## Review boundary

Reviewed source: `shippin-mesh/warden/proxmox.go`, the source's `warden/AGENTS.md`,
and the platform driver, API, tests, and React Infrastructure view. No source files
were moved or copied. The platform-owned driver remains the only Proxmox adapter.

The source handler posts lifecycle actions without waiting for the returned task,
uses a default node, and allows TLS verification to be disabled. These shortcuts are
not adopted. Source page-specific behavior is covered only where its underlying
capability is useful to the shared platform contract. Broader Proxmox endpoint
coverage and its status remain in [the full coverage map](../reference/proxmox-full-coverage.md).
