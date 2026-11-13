# MCPL in Shippin

MCPL is the framework this product shares with its sibling applications so that an AI can operate an
app as the person it works for. The person explains what they want in a natural chat. The AI then
acts through the same actions the person would use, with the same permissions and audit. Plain code
does the mechanical work so the model spends its tokens only on choices. Scripted events tell the AI
when to step in, and on screen the panel moves the way a person would.

This page is Shippin's variant of MCPL: the rules every API route and every operator input follows,
what is in place, what is measured as not yet canonical, and the order of the rest. It is contract
rule MCPL-01 in [What Shippin is](objective.md).

## What makes it work

| Piece | What it is | State |
| --- | --- | --- |
| Actions | Every product action described once: ID, route, who may call it, risk, whether it takes a reason, a one-sentence summary, and the event it emits. `internal/mcpl`. | In place. 45 of the 350 served `/v1` routes are described. |
| Conformance | `TestMCPLConformance` checks every described action against the rules and the served routes, including the permission gating it. It keeps two lists that may only shrink: routes not yet described, and staff changes that take no reason. | In place. |
| Manifest | `GET /v1/mcpl` gives anyone the contract version, the domains and the public actions. `GET /v1/operator/mcpl` gives staff every action and whether they may run it. One call replaces exploring the API. | In place. |
| MCP | One main MCP for discovery and sign-in, plus one MCP per domain listing only that domain's tools, generated from the actions. A tool runs the route's own handler; there is never MCP-only business logic. | Planned for the console. `cmd/shippin-run` is a runner-domain MCP whose tools call `internal/runfleet`, the same functions as its HTTP routes. Those routes are not console `/v1` routes, so `TestMCPLConformance` does not list them. |
| Events | One envelope for every event. App events are `shippin.<domain>.<entity>.<past verb>` and are emitted only after the server has done the action. Scripted events from outside can never pose as app events. | Planned. Audit action names exist but are not canonical (below). |
| UI actions | Named intentions `shippin.ui.<domain>.<workflow>.<action>` that move the real panel, waiting until each control is visible, enabled and stable, as Playwright does, and returning a receipt of what happened. The person sees it happen and can take over. | Planned. The panel's controls already have roles and labels; the browser tests drive it by them. |
| Components and lessons | Each screen describes itself (its agent-facing name, the actions it offers and the events it emits). Lessons teach by watching real events and never act for the person. | Planned. |

## Domains

`public`, `auth`, `access`, `identity`, `tenancy`, `infrastructure`, `services`, `runtime`,
`platform`, `integrations`. These are the contexts of the [code map](architecture.md), plus `public`
(no session) and `auth` (getting a session). The same list, in the same order, names action IDs,
events, UI actions and, later, the per-domain MCPs.

## The rules

These make every input and output the same shape everywhere, so one adapter can serve every action.

**Actions**
- One action per route, and one route per action. A route that takes the action as a path segment
  (`.../{action}`) hides several actions and is split.
- The ID is `shippin.<domain>.<entity>.<verb>`, lower case with underscores, for example
  `shippin.tenancy.plan.update`. Reads use `list` and `get`.
- Risk follows the HTTP method:
  - a GET is a read;
  - a DELETE is destructive and its verb is `delete`, `remove`, `revoke`, `reset` or `clear`;
  - everything else writes.

  An AI may run reads freely and writes on the person's stated intent. It runs a destructive action
  only after the person confirms it in the panel.
- Access is `public`, `signed-in`, `staff` (with the catalog permission that gates the route) or
  `member` (a tenant member, under the tenant's own roles). Only `public` and `auth` actions change
  anything without a session.
- The summary is one plain sentence, written for a person choosing what to do.

**Routes**
- One prefix per audience:
  - `/v1/operator/...` for staff;
  - `/v1/tenants/{tenantId}/...` for an organization's members;
  - `/v1/me/...` for one's own account;
  - top-level public paths for everyone else.
- Lower-case, kebab-case segments with no verbs in them.
- Path parameters are named for what they hold: `{tenantId}`, `{planId}`, `{guestId}`, never `{id}`
  or one letter.

**Inputs**
- JSON with snake_case fields, decoded strictly: an unknown field is refused, not ignored.
- Every staff change takes `reason` in its JSON body, for every method including DELETE, and the audit
  log keeps it. One field, so an MCP tool has one argument for it.
- Credentials never pass through an AI: no password, key, token or secret is a tool argument or
  result. The one exception is the one-time setup code. Taking over the setup account and creating the
  owner's account are a person's steps.

**Outputs**
- A list is `{"items": [...]}`. One object is the object.
- An error is `application/problem+json` with a plain `message` a person can act on. A denied
  resource and a missing one give the same answer.
- Results carry names, IDs, counts and states.

**Events**
- `shippin.<domain>.<entity>.<past verb>`, for example `shippin.tenancy.plan.updated`, emitted only on
  the success path. Declared on the action; nothing aspirational.

## Measured on 2026-11-12

| What | Today | Canonical |
| --- | --- | --- |
| Served `/v1` routes described | 45 of 350 | all |
| Staff prefixes | `/v1/operator` 106, `/v1/infra` 53, `/v1/admin` 8 (API keys), `/v1/ops` 4, `/v1/servers` 3, `/v1/platform` 1 | `/v1/operator` |
| Path parameters | `{tenantId}` 106, `{id}` 79, `{guest}` 20, `{workspaceId}` 13 and one-letter names in places | named for what they hold |
| Routes hiding several actions (`{action}`) | 5 | 0 |
| Request decoding | 52 strict, 34 ignoring unknown fields | all strict |
| Reason transport | body `reason` in 21 handlers; infrastructure uses an `X-Change-Note` header | body `reason` |
| Staff changes without a reason, among those described | 19 (the catalog) | 0 |
| Errors | problem+json in about 800 places, `http.Error` in 9 | problem+json |
| Permission models | catalog permissions, role-gated infrastructure routes, API-key scopes on `/v1/admin` | catalog permissions |
| Audit and event names | mixed tenses and shapes (`invite.create`, `invite.redeemed`, `team.role_create`, `waitlist.joined`) | `shippin.<domain>.<entity>.<past verb>` |

The two shrinking lists in `internal/api/testdata/mcpl-*.txt` show every gap by route and by action.

## Order of work

1. **Done.** The action contract, its conformance test, the manifest, and the catalog as the
   reference context.
2. **Each context as it is rewritten** ([code map](architecture.md), starting with Access). It
   describes every one of its routes and moves them onto the canonical prefix and parameter names. It
   takes `reason` in the body, decodes strictly, and splits `{action}` routes. The lists shrink in the
   same commit. Routes stay where they are only until their context is rewritten, since this is
   pre-release and nothing outside depends on them.
3. **MCP from the actions.** The main MCP and one per domain, generated from the described actions and
   running their handlers, with a person's delegated session as the way an AI signs in. The bridge's
   device sign-in is the starting point.
4. **Events.** The envelope, emitted from each action's success path, and the audit log's action names
   moved onto event names.
5. **UI actions, components and lessons,** with receipts, proved in a real browser.

## Adding or changing a route

1. Pick the domain. If no domain fits, it is probably two actions.
2. Describe the action next to the route: ID, access and permission, risk, reason, summary, and the
   event once events exist.
3. Follow the rules above for its path, input and output.
4. Run `go test ./internal/api -run TestMCPLConformance`. A new undescribed route fails the test.
