# Hypervisor drivers

Status: the console is the product. The Proxmox and Incus drivers are mounted on the server registry (`internal/db/migrations/036_server_hypervisor.sql`), `/v1/infra/servers`, and `/operator/hosts`. The Incus driver has only been exercised against a simulated API, not a real server. The capture of every imported file is `docs/design/warden-capture-matrix.md`. The full API plan is `docs/design/proxmox-full-coverage.md`.

The older panel is the Go+HTML tree imported at `third_party/warden-ct102` (built 2026-08-27). Its Proxmox client is `proxmox.go`. The files `internal/host/kinds/proxmox`, `wardenclyffe`, and `bare` are registered stubs that return "not yet implemented". "Wardenclyffe" in that tree is not a second hypervisor. Shippin-mesh notes treated it as a product name. In this console it is provenance only.

## One driver

```text
Health(ctx) (Health, error)
Guests(ctx) ([]Guest, error)
Guest(ctx, id) (Guest, error)
Start, Stop, Shutdown, Reboot(ctx, id) (Guest, error)
Snapshot(ctx, id, name) (Guest, error)
Storage(ctx) ([]Volume, error)
Networks(ctx) ([]Network, error)
```

`Guest.ID` is a string. Proxmox uses the numeric VMID. Incus uses the instance name. `Guest.Kind` is `vm` or `ct`. `Guest.Status` is the hypervisor's word (`running`, `stopped`, …), copied from the last GET, never invented.

Every mutating method:

1. GETs the guest. Missing guest is an error.
2. If the guest is already in the requested state, returns that GET. It does not POST again.
3. Otherwise POSTs, then GETs again and returns that second body.
4. If the second GET does not show the requested state, returns an error that includes the status the API actually reported.

Errors are returned to the caller. Nothing logs the same failure on a timer except the reconciler rule below.

## What a driver declares

`internal/hv/catalog.go` states what each driver needs and can do: attach fields, capabilities and verbs. Those are facts about the code. What a person reads about a driver comes from configuration (`ui.providers`), so a site can rename one and a new driver needs no panel change. `GET /v1/infra/overview` joins the two. The panel draws its tabs, add form and actions from that response and names no provider itself; `web/scripts/check-hardcoding.mjs` fails the web tests if that stops being true.

The local agent's Incus project is listed as a read-only node when hostd answers. A hostd that does not answer is reported as a note, not as a host: a fresh install has none.

`Console` and `Exec` are not on the interface yet. The panel's terminal is a host PTY, and Proxmox VNC is a websocket proxy. Both need a session broker. They are left out rather than stubbed as success.

## Drivers

Guest creation is configured from the attached provider's observed resources. The Proxmox form requires a storage that supports the selected guest kind, a bridge present on the attached node, and an explicit positive disk size. The driver validates these values again at execution. Incus creation continues to use its configured project and selected Incus image; it does not inherit Proxmox placement rules.

| Kind | Code | Source of truth |
|---|---|---|
| `proxmox` | `internal/hv/proxmox` | PVE `/api2/json`. Phase 1: version, cluster resources, qemu and lxc lifecycle, create/clone/template/delete/config/resize/migrate, snapshots and rollback, storage content, upload, download-url, node services and network, firewall read, vzdump, backup list, restore, UPID status and log. |
| `incus` | `internal/hv/incus` | Incus REST: `GET /1.0`, `GET /1.0/instances`, `GET /1.0/instances/{name}`, `PUT /1.0/instances/{name}/state`, `POST /1.0/instances/{name}/snapshots`, `GET /1.0/storage-pools`, `GET /1.0/networks`. This is the same API hostd already drives with the `incus` CLI (`internal/hostd`). The driver does not shell out. |
| `bare` | `internal/hv/bare` | No hypervisor API. Health and every mutation return `ErrNoHypervisor`. Inventory is never a fake "online" with empty guests. |

`wardenclyffe` as a host kind is dropped. The stub did not speak a different API. A future fork, if it is still PVE-shaped, is a `proxmox` endpoint with its own URL and CA.

## TLS and names

The native Proxmox driver verifies TLS and has no skip flag. The Warden example leaves the host empty and keeps verification on.

The console driver always verifies TLS. The trust bundle is either the system roots or a CA file path on the server row. There is no skip flag. The token is a `secret_ref` (`sops://` or `vaultix://`) resolved at call time by an injected function. The driver struct does not hold the token bytes in config JSON.

Host, port, node, and API URL live on the server row (`031_servers.sql` plus `036_server_hypervisor.sql`). They are not package variables and not a second registry. `hv_token_env` is an environment variable name. The secret is not a column. Seeded estate rows stay null.

## Approval and the infra role

Mutations currently call `hv.Guard` before the driver and use the local `internal/approval.Policy`. That policy has compiled fallback roles and a Shippin-local board adapter; it is not AgentHub policy evaluation. This is a known ownership gap. The target is a configured AgentHub policy/ticket/review adapter with explicit system mapping and fail-closed behavior; see [configurable human review](../approval.md). Until that integration is implemented, report the current local-board behavior accurately and do not call it AgentHub.

`roles.infra` (shipped as `op_infra`) is the only role that may call `/v1/infra/servers`. Platform admin is not implied. The approval policy still decides whether that caller must wait for a board card. The library refuses a subject the policy does not allow.

## Reconciler

`hv.Step` is one pass: probe health, list guests, remember the last error string. Backoff doubles from a base to a cap when the error string is unchanged. The pass returns whether the error string changed. The caller logs only when it changed. There is no loop in the library, so a stuck host cannot spam `log.Printf` on its own.

## Warden concept → console home

| Warden | Console | Note |
|---|---|---|
| `proxmox.go` inventory and lifecycle | `internal/hv/proxmox` | Read-back added. TLS skip removed. |
| `internal/host` YAML registry | `servers` rows | Not a second registry. |
| Kind stubs (`proxmox`, `bare`) | real drivers | Stubs are not used. |
| Kind `wardenclyffe` | dropped | No distinct API. |
| Snapshot of qemu only | `Snapshot` on Proxmox qemu and lxc, and on Incus instances | Both kinds poll the task id, then read the guest back. |
| `backups.go` tar/rsync jobs | managed-services estate backups | Shell-outs not ported. |
| `backups_s3.go`, `backups_smb.go`, `backups_infisical.go` | destination records with secret refs | Transfer clients left out. |
| `backups_secrets.go` | dropped | Wrote raw keys to `backups-secrets.json`. |
| `backups_coverage.go` catalog | evaluator only | Its site paths stay in the import, not in defaults. |
| `backups_estate.go`, `backups_events.go` | event ring + preflight checks that do not dial the network | Estate SMB to `192.0.2.117` is not a default. |
| PAM login, local users | Nomen session | Second login stack dropped. |
| File manager, docker, cron, firewall, packages | dropped | Guest OS admin, not the hypervisor driver. |
| HTML templates and static panel | dropped from the product UI | Kept only in the import. |
| `install/*.sh` | provenance | Not the installer. CRLF-only differences vs mesh main. |
| Single global `PROXMOX_*` env | per-server endpoint | Example values are not copied into `config/shippin.json`. |

## Left out of phase 1

Later Proxmox groups (HA, SDN, Ceph, ACME, consoles, agent exec, access identities) are specified in `proxmox-full-coverage.md` and are not routed. S3, SMB, and Infisical transfer are specified in `managed-services.md` and are not wired. The imported `package main` tree is a nested module and is not part of `go build ./...`.
