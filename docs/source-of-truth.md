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

The [capability absorption ledger](design/capability-absorption.md) records which
behaviors from historical console and mesh sources are already owned here, which
need implementation, and the evidence required before those sources are archive-ready.
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
records and verify their meaning without treating them as new AgentHub decisions. No common
review backend or integrated navigation exists here yet. See the
[system identity and human review plan](system-identity-human-review-plan.md).

The draft service-definition and attachment shapes live in
`api/openapi/schemas/service-catalog.json`, registered in the OpenAPI document.
They do not implement a catalog, replace historical endpoint responses, or confer
tenant isolation. See [service contexts](service-contexts.md) for the product map
and integration boundaries.
