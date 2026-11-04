# Proxmox capability absorption

This review compares the Proxmox panel in the local mesh source with the canonical
Shippin hypervisor boundary. It records behavior and ownership; it does not import
the source implementation. The source repository had no root license file in the
reviewed tree, so its code and dependencies remain outside this repository.

## Capability mapping

| Source behavior | Canonical owner | Disposition |
|---|---|---|
| Read API version and report availability | `internal/hv/proxmox` health probe | Already implemented with verified TLS and provider error handling. |
| List cluster resources for VMs and containers | `internal/hv/proxmox` guest inventory | Already implemented with node-aware guest identity. |
| Show CPU, memory, disk, network counters, uptime, pool, template, and tags | `hv.Guest` and the existing Infrastructure guest detail card | Newly retained as provider-reported telemetry; other drivers may omit unavailable metrics. |
| Start, stop, shutdown, and reboot | Existing hypervisor lifecycle methods and audited Infrastructure API | Already implemented with task completion and state read-back. |
| Create and inspect snapshots | Existing snapshot operation | Already implemented for both QEMU VMs and LXC containers. |
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
coverage and its status remain in [the full coverage map](proxmox-full-coverage.md).
