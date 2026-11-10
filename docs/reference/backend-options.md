# Hypervisor backends

A server has one kind. The kind is a column, not a process and not a second product.

| Kind | When | How the console talks |
|---|---|---|
| `proxmox` | Proxmox VE 8 or 9 | `/api2/json` with an API token. TLS verified. Phase 1 routes are in `docs/design/proxmox-full-coverage.md`. |
| `incus` | The hostd machine | The Incus REST driver in `internal/hv/incus`. The Instances page still reads hostd. Guest methods beyond that driver are not routed yet. |
| `bare` | No hypervisor API | Every call returns `ErrNoHypervisor`. Inventory is never a fake online. |

Pick `proxmox` when the machine is a VE node or cluster. Pick `incus` when hostd is already the agent. Pick `bare` when the server is only an enrolled machine.

There is no fourth kind. A PVE-shaped API is `proxmox` with its own URL and CA file.
