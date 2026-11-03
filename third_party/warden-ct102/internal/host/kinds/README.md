# warden/internal/host/kinds/

One subdirectory per host kind. Each implements
[`host.Connector`](../connector.go) and registers itself with the host
package's dispatch table at process init.

## Current kinds

| Kind | Package | Status | Maps to Phase |
|---|---|---|---|
| `proxmox` | [`proxmox/`](./proxmox/) | stub | Phase 2 — imports the existing `warden/proxmox.go` logic |
| `wardenclyffe` | [`wardenclyffe/`](./wardenclyffe/) | stub | Phase 2 — initial alias for `proxmox`, diverges as the fork accrues features |
| `bare` | [`bare/`](./bare/) | stub | Phase 3 — SSH-only non-Proxmox hosts |

## Adding a new kind

1. Add the constant to [`../host.go`](../host.go) `Kind` block + `IsKnown()`.
2. Create `kinds/<newkind>/connector.go` — implement `host.Connector`.
3. The package's `init()` calls `host.Register(KindNew, NewConnector)`.
4. Add a smoke test under `kinds/<newkind>/connector_test.go`.

Do not branch on `Kind` in feature code. Always go through the Connector interface.
