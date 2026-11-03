# Source ownership

This repository is the canonical source for the public Shippin platform components
it contains. The previous private development repository remains a historical source;
its other components and uncommitted work are not automatically adopted here.
Historical snapshot and integration branches remain candidates for separate review;
merging current public branches does not adopt that private history.

`api/openapi/` owns the draft API contract. `cmd/console` and `internal/` own the
control plane imported from shippin-console `e35ab817bbfedc626f35aa2f42760df49d4684c2`.
`web/` owns the installed panel. `panel/` remains an API-workbench prototype and is
not the install path. `cmd/shippin-hostd` and `internal/hostd` own the host agent.
`agent/` owns the local Incus diagnostic. `third_party/warden-ct102` owns the
Proxmox panel UI. `install/panel/` owns the one-command panel installer.
`install/primary-image/` owns the development image builder. `governance/` owns the
local plan and journal tooling. Site values live in an uncommitted site file.
See [the console import](merge-from-shippin-console.md).

The draft service-definition and attachment shapes live in
`api/openapi/schemas/service-catalog.json`, registered in the OpenAPI document.
They do not implement a catalog, replace historical endpoint responses, or confer
tenant isolation. See [service contexts](service-contexts.md) for the product map
and integration boundaries.
