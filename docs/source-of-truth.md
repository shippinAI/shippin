# Source ownership

This repository is the canonical source for the public Shippin platform components
it contains. The previous private development repository remains a historical source;
its other components and uncommitted work are not automatically adopted here.
Historical snapshot and integration branches remain candidates for separate review;
merging current public branches does not adopt that private history.

`api/openapi/` owns the draft API contract. `panel/` is an API-workbench prototype.
`install/primary-image/` owns the development image builder. `governance/` owns the
local plan and journal tooling. `agent/` owns provider diagnostic code; it
currently supplies local Incus and read-only Proxmox diagnostics. A canonical deployable control
plane and enrolled remote host agent remain to be established. No candidate code
is accepted merely by being present in another repository; review source provenance,
ownership, dependencies and tests before adopting it.

The intended product is one Shippin.app panel, with role-aware customer and operator
workflows backed by server-side authorization. The current single prototype deployment
is an API workbench, not the completed experience. Shared installation settings
belong to the control plane; browser-only preferences do not configure an installation.
Incus and Proxmox are both intended infrastructure providers. Provider capabilities
and lifecycle operations must be implemented and verified independently.

Shippin is self-hosted software for operators to install on their own infrastructure;
it does not require a commercial hosted service. Server addresses, identities and
human-review policy are installation configuration. AgentHub owns its policy
documents, change tickets and reviews. Gold approvals are not a product feature,
policy, navigation destination or default. The historical console's Gold-specific
routes and signing store and AgentHub's Gold-specific seed and runner assumptions
must be retired through a reviewed migration. Preserve historical records and
verify their meaning without treating them as new AgentHub decisions. No common
review backend or integrated navigation exists here yet. See the
[system identity and human review plan](system-identity-human-review-plan.md).

The draft service-definition and attachment shapes live in
`api/openapi/schemas/service-catalog.json`, registered in the OpenAPI document.
They do not implement a catalog, replace historical endpoint responses, or confer
tenant isolation. See [service contexts](service-contexts.md) for the product map
and integration boundaries.
