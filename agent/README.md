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

This is not yet an enrolled agent, control plane, authorization boundary,
scheduler, deployment artifact, or lifecycle implementation. It does not
establish that a node is ready or schedulable. Proxmox support and real-host
verification remain outstanding.
