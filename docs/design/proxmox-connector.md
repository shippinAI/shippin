# Attaching a hypervisor

One server row is one hypervisor API. The console does not keep a second registry, a root password, or a token secret.

## From the panel

Open **Infrastructure → Hosts & nodes** and choose **Add host**. The overview lists every provider this installation offers. A provider gets its own tab as soon as a host is attached to it. The form's fields come from the driver, and their names and defaults come from `ui.providers` in the config file or **Provider panel → Interface**.

**Add host** creates the server row and attaches its API in one step, which is the `POST /v1/infra/servers` call below. It saves what you enter and does not dial it. The overview then reports whether the host answers, and keeps listing a host that does not, with the reason. **Detach** removes the API and leaves the server row and its guests alone.

Put a token secret in `/etc/shippin/console.env` as `SHIPPIN_HV_<NAME>=secret` and restart the console. An installer re-run keeps `SHIPPIN_HV_*` values.

An Incus remote usually trusts a client certificate: give the certificate and key paths on the console host, and run `incus config trust add-certificate` on the server. **Project** selects the Incus project; the driver's reads and changes stay inside it.

Only start, stop, shutdown, reboot and snapshot are shared by every driver. Calls only Proxmox has answer `409` on an Incus host, and the panel does not offer them.

The local installation owner has no IdP to grant `IAM_INFRA`, so the infrastructure routes and approval rules accept them as the owner. This does not make them a founder: tenant access grants still apply.

## What you set through the API

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

Adding or detaching an API is the action `hv.attach`, shipped without a board card because it changes what the console can reach, not any guest. It is still role-gated and audited. A policy with no `hv.attach` rule uses `hv.lifecycle` for it.

**Provider panel → Platform settings → Board approval for hypervisor changes** turns the card off for an installation. The panel can only relax the rule: a config file that says `needs_approval: false` stays off, and clearing the override returns to the file.

The shipped rule `hv-lifecycle` allows `founder`, `op_infra`, `org_owner`, and `org_admin` to request the change. The board still has to approve while `needs_approval` is true. Change the rule in `config/shippin.json` to narrow that.

Set `approval.board.org_id` so the card has an org. If it is empty, a tenant card uses the tenant id and an operator card uses the caller's org, then `operator.platform_org_id`.

## Who sees which guests

| Caller | Sees | Can change |
|---|---|---|
| Infra role | Every server with a hypervisor kind | Power, snapshots, create, delete, migrate, storage, attach |
| Org owner or admin | Servers that org owns or is assigned | Power and snapshots, after the approval card |
| Org member | Those same servers | Read guests and tasks |

Another org's server returns 404. An estate seed row with no assignment is operator-only.
