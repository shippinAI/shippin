# warden/internal/host/

Multi-host abstraction. See
[`docs/decisions/0008-repo-foundation.md`](../../../docs/decisions/0008-repo-foundation.md)
§2-5.

A managed instance is a `Host`. Hosts are loaded from
`/etc/warden/hosts.yaml` (production) or `<repo>/registry/hosts.yaml`
(dev). Each host has a `Kind` — `proxmox`, `wardenclyffe`, `bare`,
or future — that selects which connector implementation drives it.

## Files

- [`host.go`](./host.go) — `Host`, `Kind`, `NetAddr`, `CredRefs`, `Status`
- [`connector.go`](./connector.go) — `Connector` interface; every kind implements it
- [`registry.go`](./registry.go) — load + validate `hosts.yaml`, dispatch to the right kind
- [`kinds/`](./kinds/) — one subdirectory per host kind, each implementing `Connector`

## On-disk layout per host

```
/etc/warden/hosts/<name>/
├── host.yaml                # name, kind, os, addresses, owner_client, tags
├── credentials/
│   └── *.ref                # references into the keystore (no bytes)
├── inventory/               # cached connector output (lxc.json, qemu.json, storage.json)
└── deployments/             # deployment state files for THIS host
```

## Adding a new host kind

1. Create `kinds/<newkind>/connector.go`.
2. Implement `host.Connector`.
3. Register the constructor in `registry.go`'s kind dispatch.
4. Add a smoke test under `kinds/<newkind>/connector_test.go`.

No core change required.
