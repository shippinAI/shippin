# Host-side infrastructure code

This module is the source owner for Shippin's host-side provider code. Its first
command is a local, read-only Incus diagnostic:

```sh
go run ./cmd/shippin-agent inspect-incus -socket /absolute/path/to/incus.sock -project selected-project
```

The operator must supply the local socket and an existing Incus project. The
command reads the server version and the selected project's instance list; it
does not alter Incus. Socket access can grant powerful host privileges: run it
only on an authorized host and protect its JSON output as infrastructure data.
An Incus project is not automatically a Shippin project or tenant.

The agent also includes a read-only Proxmox inventory diagnostic. Supply an
HTTPS origin and token through your execution environment. For a private CA,
pass `-ca-file /path/to/ca.pem`; certificate and hostname verification remain
enabled. The token needs permission to read the desired inventory, and results
are limited to what that token can see.

```sh
go run ./cmd/shippin-agent inspect-proxmox -endpoint https://your-proxmox-host:8006 -ca-file /path/to/ca.pem
```

Set `SHIPPIN_PROXMOX_TOKEN_ID` and `SHIPPIN_PROXMOX_TOKEN_SECRET` securely in
the command's environment; do not put secrets in arguments, logs or repository
configuration. The command reads the provider version and cluster VM/container
inventory without changing provider state. An empty result does not prove that
the token can see every resource.

Guest lifecycle lives in the console's drivers (`internal/hv`), not here.

This is not yet an enrolled agent, control plane, authorization boundary,
scheduler, deployment artifact, or integrated lifecycle service. It does not
establish that a node is ready or schedulable. Provider management and real-host
verification remain outstanding.
