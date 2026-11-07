# Source ownership

This repository is the canonical source for the public Shippin platform components
it contains. The previous private development repository remains a historical source;
its other components and uncommitted work are not automatically adopted here.
Historical snapshot and integration branches remain candidates for separate review;
merging current public branches does not adopt that private history.

`api/openapi/` owns the draft API contract. `cmd/console` and `internal/` own the
control plane imported from shippin-console `e35ab817bbfedc626f35aa2f42760df49d4684c2`.
`web/` owns the panel, and there is only one. See [panel variants](panel-variants.md). `cmd/shippin-hostd` and `internal/hostd` own the host agent.
`internal/hv/` owns the hypervisor drivers. `agent/` owns a host-side diagnostic. `third_party/warden-ct102` is a
reference import that is not served, kept until its remaining features are ported. `install/panel/`, `install/docker/`,
`Dockerfile` and `compose.yaml` own deployment.
`install/primary-image/` owns the development image builder. `governance/` owns the
local plan and journal tooling. Site values live in an uncommitted site file.
See [the console import](merge-from-shippin-console.md). No candidate code is
accepted merely by being present in another repository; review source provenance,
ownership, dependencies and tests before adopting it.

`internal/networking/` owns installation-scoped Cloudflare Tunnel, Tailscale, and
custom inventory connections. It persists configuration and observed resource rows
under `shippin_` table names, seals credentials through the configured installation
key, and treats remote inventory as observation rather than a deployed change.

`internal/catalog/` owns reusable service definitions, customer plan/configuration
and pinned source provenance. `internal/deployments/` owns tenant-scoped deployment
generations, the transactional `shippin_` outbox, provider targets, desired/observed
state, reconciliation and deployment events. The transactional outbox producer,
leader relay and generation-keyed job worker are implemented. For an approved
Incus desired-present generation, the worker separately reviews, authorizes,
claims and reads back each supported operation: deployment-owned persistent
volume ensure (when declared), digest-pinned OCI workload creation while stopped,
power-on of that exact workload, and guarded stop/removal with snapshot preflight
while retaining persistent data. Each write reloads the immutable reviewed
generation under its live lease, revalidates target inventory, obtains current
AgentHub authorization and consumes that step's one-time dispatch claim. Declared
environment inputs are resolved through the lease-, audit- and provider-trust-
gated Vaultix path immediately before workload dispatch. Ambiguous writes are
observed read-only and never retried blindly. A queued target-health command
leaves observed state `unknown` and returns `health_pending`; an authenticated
result is reconciled separately. Only a verified pass can produce `ready`.
Upgrades, backup/restore, complete service lifecycle and Proxmox application
execution are not implemented, so this partial runtime is not yet a turnkey
deployable service.
`internal/jobs/` remains the durable worker lease/retry mechanism, not deployment
truth. `internal/hv/incus/` and `internal/hv/proxmox/` retain provider-specific
guest behavior; `internal/hostd/` owns only the bounded in-guest bootstrap and
workload operations exposed through its authenticated contract. No provider or
builder implementation may be hidden in the
catalog handlers or the React panel.

The [capability absorption ledger](design/capability-absorption.md) records which
behaviors from historical console, cloud, mesh and Warden sources are already owned
here, which need implementation, and the evidence required before those sources are
archive-ready. The [archive-readiness ledger](design/archive-readiness.md) records
the observed state and preservation gates for each checkout; no source tree is
deleted or cleaned as part of capability review.
The [platform absorption plan](design/platform-absorption-plan.md) defines the target
domain model, UI destinations, phase order and acceptance gates.
The Proxmox operator capability is authored as Shippin Prox; `proxmox` remains the
provider kind and API terminology. Historical Warden names are source provenance only.

The product is one panel, with role-aware customer and operator workflows backed by
server-side authorization. Shared installation settings
belong to the control plane; browser-only preferences do not configure an installation.
Incus and Proxmox are both intended infrastructure providers. Provider capabilities
and lifecycle operations must be implemented and verified independently.

Shippin is self-hosted software for operators to install on their own infrastructure;
it does not require a commercial hosted service. Server addresses, identities and
human-review policy are installation configuration. AgentHub owns its policy
documents, change tickets and reviews. The Gold signing routes and store imported from
the console have been removed; see [the approval policy](approval.md) for what became
of a site's records. AgentHub's Gold-specific seed and runner assumptions live in
AgentHub and must be retired there through a reviewed migration. Preserve historical
records and verify their meaning without treating them as new AgentHub decisions.
Shippin now has AgentHub-backed deployment review preparation and exact-action
operator authorization in the existing panel. The deployment worker obtains
lease-fenced just-in-time AgentHub authorization immediately before each
implemented Incus volume, workload creation, power and removal write. Separate
single-guest Incus and Proxmox power actions also persist exact-action review and
one-shot execution provenance. Production policy quorum and live provider
execution remain unverified; other provider mutations remain fail-closed. See
the [system identity and human review plan](system-identity-human-review-plan.md).
Deployment review reservations persist the version and digest of a deterministic
provider execution plan derived from the immutable generation. The AgentHub action
and review evidence bind that plan; stale plan digests are rejected before an
authorization request. A pending application probe stays `unknown`; generations
without a supported executor remain `runtime_unsupported`. These provider steps
do not prove a complete service deployment.
Incus provider records can explicitly acknowledge that provider administrators
may read tenant deployment secrets projected into instance configuration. The
setting is operator-managed, defaults to deny, and gates the lease-fenced
Vaultix reveal helper; it does not provide isolation from provider administrators
or make the partial deployment executor a complete service runtime.

The draft service-definition and attachment shapes live in
`api/openapi/schemas/service-catalog.json`, registered in the OpenAPI document.
They do not implement a catalog, replace historical endpoint responses, or confer
tenant isolation. See [service contexts](service-contexts.md) for the product map
and integration boundaries.
