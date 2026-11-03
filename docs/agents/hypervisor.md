# Hypervisor calls for an agent

Use the console API. Do not invent hosts, tokens, or a second login.

## Before a mutation

1. `GET /v1/infra/servers` as the infra role, or `GET /v1/tenants/{tenantId}/servers` as an org admin, and take `id` from the row the user named.
2. `GET .../guests` and match the guest by `name` or `id`. The status in the response is the hypervisor's word.
3. Send the mutation. Power is `POST .../guests/{guest}/status/{start|stop|shutdown|reboot}`. Snapshot is `POST .../guests/{guest}/snapshots` with `{"snapname":"before"}`.

Operator base: `/v1/infra/servers/{id}`.
Tenant base: `/v1/tenants/{tenantId}/servers/{serverId}`.

Create, delete, migrate, upload, and `PUT /v1/infra/servers/{id}` are operator-only.

## When the response is 409

Read `code`, `message`, and `card_key`.

| code | What to do |
|---|---|
| `waiting_on_board` | The console already filed the card. Tell the user the `card_key` and that they approve it on the factory board (`/hub`). Then send the same request again. Do not ask them to confirm a second time in the console. Do not invent a new card key. |
| `board_org_missing` | `approval.board.org_id` is empty and no org could be chosen. Say that. Do not call the hypervisor. |
| `approval_denied` | The caller's roles are not on the `hv.lifecycle` rule. Say which role the policy expects. |

A 403 that names `op_infra` and `IAM_INFRA` means the caller needs that Nomen role. The console does not create it.

## What not to do

- Do not put a token secret, a password, or `InsecureSkipVerify` in a request or a config file.
- Do not call a guest on a server the org was not assigned. A 404 means it is not theirs.
- Do not poll faster than the task endpoint already refreshes. `GET .../tasks/{upid}` returns `done` when the task has stopped.
