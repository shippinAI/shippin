# hub-p2: agent hub, production-complete (on hub-p1)
Box only; nothing deployed.

## What changed vs hub-p1
- **Owner by verified e-mail:** `HUB_OWNER_EMAIL` (case-insensitive, only when Nomen says
  `email_verified`) or `HUB_OWNER_SUB` (exact). Either one mounts `/v1/hub`; neither = not mounted.
  `auth.Identity` now carries `EmailVerified` from userinfo.
- **Live updates:** migration 029 adds a trigger that `pg_notify`s `hub_events` on every event;
  `hub.Live` LISTENs (reconnects on error) and fans out per org; `GET /v1/hub/stream` is SSE
  (25 s heartbeat, `X-Accel-Buffering: no`). The board shows Live / Connecting / Offline and
  polls every 60 s while live, 5 s otherwise. Works across replicas (the DB is the bus).
- **Event history:** `GET /v1/hub/tasks/{id}/events` (org-scoped); every transition records its
  note (progress text, approval request, decision note). Cards have a React Aria History disclosure.
- **Decision note:** `POST .../decision {approve, note}`.
- **hub-demo** now runs the real `auth.Flow` (PKCE code flow, encrypted session cookie, bearer
  tokens validated at the IdP) against a loopback Zitadel-shaped stand-in (`cmd/hub-demo/idp.go`).
  The `X-Demo-Sub` header is gone. TLS is an in-memory self-signed cert (cookies are Secure).

## Console env
`HUB_OWNER_EMAIL=jessay@gmail.com` (or `HUB_OWNER_SUB`), optional `HUB_LEASE` (Go duration, default 10m).

## Tests
- `go test -p 1 ./...` with `SHIPPIN_TEST_DATABASE_URL`: 25 packages ok (new: history, live fan-out
  per org, verified-e-mail ownership).
- `deploy/screenshots/hub-e2e.mjs` against hub-demo: 18/18 (see `docs/screens/hub-p2/e2e.txt`).
Not tested: real Nomen (Zitadel) login, SSE through the Gold edge Caddy.
