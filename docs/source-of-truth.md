# Source ownership

This repository is the canonical source for the public Shippin platform components
it contains. The previous private development repository remains a historical source;
its other components and uncommitted work are not automatically adopted here.
Historical snapshot and integration branches remain candidates for separate review;
merging current public branches does not adopt that private history.

`api/openapi/` owns the draft API contract. `panel/` is an API-workbench prototype.
`install/primary-image/` owns the development image builder. `governance/` owns the
local plan and journal tooling. A canonical deployable control plane and remote host
agent remain to be established. No candidate code is accepted merely by being present
in another repository; review source provenance, ownership, dependencies and tests
before adopting it.

The draft service-definition and attachment shapes live in
`api/openapi/schemas/service-catalog.json`, registered in the OpenAPI document.
They do not implement a catalog, replace historical endpoint responses, or confer
tenant isolation. See [service contexts](service-contexts.md) for the product map
and integration boundaries.
