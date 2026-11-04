# Hypervisor calls for an agent

Use the console API. Do not invent hosts, tokens, or a second login.

## Before a mutation

1. `GET /v1/infra/servers` as the infra role, or `GET /v1/tenants/{tenantId}/servers` as an org admin, and take `id` from the row the user named.
2. `GET .../guests` and match the guest by `name` or `id`. The status in the response is the hypervisor's word.
3. Provider mutations are currently disabled until AgentHub review is integrated. Requests return `503` with `code: agenthub_review_unavailable`; the provider is not called. Keep using the read routes for current inventory.

Operator base: `/v1/infra/servers/{id}`.
Tenant base: `/v1/tenants/{tenantId}/servers/{serverId}`.

Create, delete, migrate, upload, and `PUT /v1/infra/servers/{id}` are operator-only.

## When a mutation is denied or pending

Read the response code, `code` and `message`. AgentHub policy/ticket integration is not implemented
yet. Shippin-local policy and task-board settings do not authorize provider changes.

| code | What to do |
|---|---|
| `agenthub_review_unavailable` | Stop. Do not retry against the provider, create a local board card, infer a reviewer or suggest a role to add. The AgentHub decision path is pending. |

A 403 that names `op_infra` and `IAM_INFRA` means the caller needs that Nomen role. The console does not create it.

## What not to do

- Do not put a token secret, a password, or `InsecureSkipVerify` in a request or a config file.
- Do not call a guest on a server the org was not assigned. A 404 means it is not theirs.
- Do not poll faster than the task endpoint already refreshes. `GET .../tasks/{upid}` returns `done` when the task has stopped.
