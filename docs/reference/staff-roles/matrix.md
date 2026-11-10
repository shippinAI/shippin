# Permission matrix

Legend: **-** none · **R** read · **A** actions (non-destructive) · **W** write/config ·
**D** destructive (delete) · **G** request consent grant · **2P** needs second approver.
Deny by default: anything not listed is denied, including new routes until mapped.

| Section | Owner | Platform Admin | Ops | Support | Billing | Security | Auditor |
|---|---|---|---|---|---|---|---|
| Tenants | R W D(2P) | R W D(2P) | R A | R G | R (name/status/plan) | R | R |
| Workspaces | R A G | R A G | R A G | R G | - | R | R |
| Users & sign-in | R W | R W | R | R | - | R | - |
| Host | R W | R | R A | - | - | R | R |
| Infrastructure ([permissions](../control-plane.md#cp-3-each-duty-has-its-own-permission)) | R W, own guest | - | R A | - | - | R | R |
| Edge | R W | R W | R A | - | - | R | R |
| Access & audit | R | R (own + tenant) | R (own) | R (own) | - | R W (legal hold, approve BG) | R |
| Templates | R W D | R W D | R | R | - | R | R |
| Mail | R W | R W | R | - | - | R | R |
| DNS | R W | R W | R | - | - | R | R |
| Billing | R W | - | - | - | R W | R | R |
| Team | R W (2P for Owner/Security) | R | - | - | - | R | R |

"Workspaces R" = metadata only. IDE, previews and workspace data always need a consent grant or
break-glass, for every role including Owner (unchanged from the 50bcc84 design).

## Permission strings (server-side)
`<section>.<verb>`; the UI uses only `/me` capabilities, the API re-checks every call.

| Permission | Current route(s) |
|---|---|
| `tenants.read` | GET /v1/operator/tenants, GET .../vm, GET .../apps (status only; no install command) |
| `tenants.lifecycle` | POST .../provision, suspend, resume, POST .../vm, .../vm/{action}, .../vm/harden, POST .../desktop/prepare |
| `tenants.delete` | DELETE /v1/operator/tenants/{id} |
| `workspaces.read` | GET /v1/operator/workspaces |
| `workspaces.act` | POST /v1/operator/workspaces/{id}/{action} |
| `projects.create` | POST /v1/operator/tenants/{id}/projects, GET .../tenants/{id}/workspaces/{id}/discover (owner only; Risk) |
| `identity.read` | GET /v1/operator/identity |
| `templates.read` / `templates.write` | GET / POST, PUT, DELETE /v1/operator/templates |
| `host.read` | GET /v1/operator/host |
| `edge.read` | GET /v1/operator/edge |
| `grants.request` | GET/POST /v1/operator/access-requests, POST .../end (own) |
| `breakglass.request` | POST /v1/operator/break-glass |
| `breakglass.approve` | new: POST /v1/operator/break-glass/{id}/approve |
| `audit.read` | GET /v1/operator/audit (scope filtered: own vs all) |
| `audit.legal_hold` | new: withhold notice decision |
| `mail.*`, `dns.*`, `billing.*`, `edge.write`, `host.write` | **[guess]** sections not yet routed in `operator.go`; reserve names now |
| `team.read` / `team.write` | new `/v1/operator/team/*` |
| `panel.view` | `/me` `provider.panel` = holds any staff role |

`workspaces.act` scope: Ops actions are restart/start/stop/retry only; destroy-type actions map to
`tenants.delete`-level **[guess: depends on the action list in `workspace_action.go`]**.
