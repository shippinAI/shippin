# Approval policy and the factory board

Hub decisions and infrastructure changes follow `approval` in `config/shippin.json`. The Go code does not
name an approver, a host, or a network. A deployment adds subjects with
`HUB_OWNER_SUB` and `HUB_OWNER_EMAIL`. Those values are copied onto the matching rules at
startup (`approval.Policy.WithRuntime`).

```json
"approval": {
  "enabled": true,
  "board": { "driver": "hub", "base_url": "", "org_id": "", "token_env": "SHIPPIN_HUB_TOKEN" },
  "rules": [
    {
      "id": "hv-lifecycle",
      "actions": ["hv.lifecycle"],
      "hosts": [],
      "approver_roles": ["founder"],
      "approver_subs": [],
      "approver_emails": [],
      "needs_approval": true
    }
  ]
}
```

`hosts` empty means every host. A rule with `"hosts": ["bastion.example"]` applies only there.
`approver_roles` matches the caller's Nomen roles (`identity.founder_role`, and the operator
role when they have one). `approver_subs` and `approver_emails` are optional extra allow-lists.
Verified e-mail only.

## Board flow

`board.driver`:

- `hub` — this console's agent checkout board (`internal/hub`), when a database is configured.
  Pending Gold tickets are posted as cards in `needs_approval` (`hub_tasks.external_key`,
  migration `035_hub_external_key.sql`). An approver decides with `POST /v1/hub/tasks/{id}/decision`.
  A passkey relay is accepted only after that card is `approved`.
- `http` — `POST` and `GET {base_url}/v1/hub/approvals` with `Authorization: Bearer` from
  `token_env`. This console serves those routes, so another console can be the board.
- `off` — no board. An allowed approver can still relay a passkey, which is the previous
  Gold path.

`POST /v1/hub/approvals` body: `key`, `title`, `body`, `action`, `host`. The caller's org is
the card's org unless `board.org_id` is set.

## Gaps

`shippinAI/shippin-cloud` and `shippinAI/shippin-platform` were not readable from the environment
that built this branch (GitHub returned 403). No factory-board API was found in this repo beyond
the agent checkout board (`/v1/hub`). The HTTP client therefore speaks this console's own
`/v1/hub/approvals` routes. Still open:

- A published agenthub schema, auth, and org model from those repos, if it differs.
- A push when the board decides. The console reads status when the ticket is opened.
- A board `approved` card does not write `/var/lib/shippin/changes/approvals`. Gold still verifies a
  passkey (`cmd/gold-passkey-verify`, approver user `GOLD_APPROVER`) before that file exists.
- `driver: hub` does nothing without `SHIPPIN_DATABASE_URL` and migration 035. Ticket reads
  then report `board_status: unavailable` and relay stays on the passkey path.

## Removed: change signing

Earlier versions carried a change-signing feature written for one installation: the `/gold` pages, `/v1/gold-sign/*` and the `GOLDSIGN_*` settings. It was off unless every one of those settings was present, and it is gone.

If your site used it:

- **Your records are untouched.** They were always files in `GOLDSIGN_DIR` and `GOLDSIGN_FACTS_DIR`, not panel data, and the panel no longer reads them. While those settings remain, it logs a notice at start. Remove the settings when you are ready; keep the files as long as you need them.
- **An old envelope stays what it was:** a record of what was approved then. It is not an approval here, and nothing in the panel treats it as one.
- **To verify one**, use the verifier as it last existed, in this repository's history: `git show 189697c:internal/goldverify/verify.go`.
- **Signing passkeys lived in those files too.** They are separate from sign-in passkeys, which still work.
- **A leftover `gold` rule in your configuration grants nothing,** because no route asks for those actions. It can be deleted.
