# Attaching a Proxmox cluster

One server row is one hypervisor API. The console does not keep a second registry, a root password, or a token secret.

## What you set

1. Create the server in the org (or use one that already enrolled). Turn on `servers.enabled` so the org can open it.
2. On **Hosts**, pick the server and fill the form. The same call is `PUT /v1/infra/servers/{id}`. `GET /v1/infra/servers/registry` lists servers that are not revoked, including ones with no hypervisor yet.

```json
{
  "kind": "proxmox",
  "base_url": "https://pve.example:8006",
  "node": "node1",
  "token_id": "agent@pve!console",
  "token_env": "SHIPPIN_HV_TOKEN",
  "ca_file": ""
}
```

`base_url` is `https` with a host and no user, query, or path. `token_id` is the Proxmox token id, not the secret. `token_env` is the name of an environment variable on the console process. `ca_file` is optional; empty uses the system trust store plus nothing else. TLS verification stays on.

3. Put the token secret in that environment variable. It is not a column and not a response field.
4. Grant `IAM_INFRA` in Nomen. `roles.iam` maps it to `op_infra`. The console does not create the Nomen role.
5. The org opens **Servers**, then the row. Guests, start, stop, snapshot, and task lookup are on that page. The operator page **Hosts** is the same model for every attached server, plus create.

## Approval

A mutation that the policy marks `needs_approval` files one factory-board card and returns `409` with `code` `waiting_on_board` and `card_key`. Approve that card. Retry the same request. There is no second confirm in the console.

The shipped rule `hv-lifecycle` allows `founder`, `op_infra`, `org_owner`, and `org_admin` to request the change. The board still has to approve while `needs_approval` is true. Change the rule in `config/shippin.json` to narrow that.

Set `approval.board.org_id` so the card has an org. If it is empty, a tenant card uses the tenant id and an operator card uses the caller's org, then `operator.platform_org_id`.

## Who sees which guests

| Caller | Sees | Can change |
|---|---|---|
| Infra role | Every server with a hypervisor kind | Power, snapshots, create, delete, migrate, storage, attach |
| Org owner or admin | Servers that org owns or is assigned | Power and snapshots, after the approval card |
| Org member | Those same servers | Read guests and tasks |

Another org's server returns 404. An estate seed row with no assignment is operator-only.
