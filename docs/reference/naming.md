# Naming

The console is the product. Packages, routes, UI labels, and config keys use the names already in this repo.

| Say this | Not this |
|---|---|
| server | a second host registry |
| guest | a panel-specific VM object |
| `roles.infra` / `op_infra` | a role baked into Go |
| `IAM_INFRA` | an account this repo creates |
| `hv.lifecycle` | a private approval action |
| `/v1/infra/servers` and `/v1/tenants/{tenantId}/servers/{serverId}` | a parallel API tree |
| Hosts, Guests, Servers | a separate panel brand in the UI |

`proxmox`, `incus`, and `bare` are driver kinds on the server row. They are the hypervisor, not a product name.

Provenance for the imported panel stays in `docs/design/warden-capture-matrix.md` and the import commit. It does not appear in package names, routes, UI strings, or config keys.
