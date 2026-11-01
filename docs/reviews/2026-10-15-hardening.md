# Hardening review — 2026-10-15

Scope: every file changed in `shippin-console` since `b0f463f`. That covers the provider panel, templates, preview URLs, tenant re-network, operator IdP read access, hostd and the web UI (112 files, about 4.2k lines). Fixes are in the commits from `668ebdb` onwards.

Severity:
- **High**: exploitable, or able to break other tenants or the platform.
- **Medium**: a correctness, race or data issue a normal user can hit, or a rule violation.
- **Low**: hygiene. Left open and listed at the end.

## High (all fixed)

| # | Area | Finding | Fix |
|---|------|---------|-----|
| H1 | Secrets / authz | `auth.Token`/`SessionToken` accepted **any** stored session id as a console session. That included workspace-host sessions and one-time codes. The `shippin_ws` cookie was also forwarded to the workspace upstream (IDE or **any dev server via previews**). Attack: a workspace creator lures an org admin onto their private preview, reads the admin's `shippin_ws` cookie in the dev server, and replays it as `shippin_session` to get the admin's console session. | Only plain sign-in sessions are console sessions (`Session.Console()`). Workspace cookies are bound to their exact host (`Session.Workspace(host)`) and codes are never accepted. Caddy strips `shippin_ws` from the `Cookie` header before every upstream; app cookies are kept. Live checks: the replay now returns **401**, and an echo server behind a private preview saw `appcookie=keepme` only. (668ebdb, 5ac1f54) |
| H2 | Tenant lifecycle | The customer's `POST /v1/tenants/{id}/provision` ran `ensureTenant`, which set status `active`. A suspended tenant's owner could **un-suspend themselves**, and a deleted tenant could be **re-provisioned**. | `ensureTenant` refuses suspended, deleting and deleted tenants with `errTenantLocked`, returned as 409. Live: customer and provider retries during suspension both return 409. (b561bfa) |
| H3 | Shared endpoint | Firewall rules only blocked 22 and the SSH range. A tenant admin could publish **tcp/443 or 80**, which hostd adds to the edge forward and an nft DNAT on the edge, hijacking platform HTTPS. Separately, an IPv6 source rendered `ip saddr <v6>`, which breaks the whole nft table. | Reserved ports come from config (`reserved_ports` [22,80,443] plus the SSH range) and are checked in both the console and hostd. Sources must be IPv4. (0bf2b62) |
| H4 | Edge routing | A custom domain could be any FQDN outside `*.ws`, including **`console.shippin.cloud` or `id.shippin.cloud`**. Routes are inserted first, so that would hijack the console or IdP. One domain could also be claimed by two workspaces (traffic and auth confusion). | `reserved_domains` (the platform domain and everything under it) are refused in the console and hostd. A domain already attached to another live workspace returns 409. At most 10 domains, no duplicates. (0bf2b62, 5ac1f54) |

## Medium (all fixed)

| # | Area | Finding | Fix |
|---|------|---------|-----|
| M1 | hostd validation | The `domains`/`previews` actions and `SyncPreviews` didn't check `Host`, so the caller decided which hostname gets routed. | `ValidHost` runs on every publish path. (5ac1f54, 5b5aae0) |
| M2 | hostd config | Zero values failed open: an empty `image_prefix` allowed any image, an unset `preview_min` allowed port 22, and the Caddy admin URL was hardcoded. | `Config.Check()` at load fails closed; `caddy_admin` must be localhost. The console has the same check (`Workspaces.Check()`). (a9a1593, 0bf2b62) |
| M3 | Correctness | `rebuild` rebuilt template workspaces from the **default** image. | It now rebuilds from the template image, after validation. (5b5aae0) |
| M4 | Previews / templates | Duplicate preview ports gave duplicate Caddy `@id`s: the old routes were dropped first, so every preview vanished. Templates allowed duplicate and unbounded ports. | Ports are unique and capped (at most 20; templates use `max_ports`). hostd validates before dropping anything. (0bf2b62) |
| M5 | Concurrency | State checks were read-then-write, so two actions could run at once on one workspace (interleaved route, ACL and port updates). hostd had no per-workspace serialisation. | `BeginWorkspaceOp` is an atomic conditional update (409 to the loser) and hostd has a per-workspace mutex. Live: a double stop returned 202 and 409. (d630357, 5b5aae0) |
| M6 | Input validation | The repo regex allowed `…/a/..`, `…/a/.` and `-x`. The clone dir is `basename`, so `..` resolves to `/home/dev`. Repo hosts were hardcoded in Go and TS. | `ValidRepo(url, hosts)`: owner and repo must start with a letter or digit; no credentials, port, query or extra path. Hosts come from config (`repo_hosts`) and the UI reads them from `/v1/templates`. (0bf2b62, fcb6347) |
| M7 | Data / rollback | Tenant delete ran on the request context, so closing the tab killed a half-done teardown. There was no guard against two deletes at once. Leftover volumes blocked deleting the pool. **The live test also found:** instance URLs carry `?project=` (delete aborted), cached image volumes were left in the pool, and the `ws-ingress` ACL blocked deleting the project. | The delete is detached (15-minute timeout) and claims the tenant as `deleting` first. It drops custom and image volumes and the ACL, and parses `?project=` URLs. The tenant is retired in one DB transaction. (2efff64, 48aea38, 431fb6e, ddd08fd, d630357) |
| M8 | Suspend | Suspend stopped workspaces first and marked the tenant suspended **even if stops failed**. Starts were still accepted while the stops ran. | The status flips first (blocking starts), then workspaces are stopped. Failures are recorded per workspace and returned as `stop_failed`. A second suspend returns 409. (2efff64) |
| M9 | SSH port allocation | `max(port)+1` never reused freed ports and could pass `port_max`. Unique constraints counted deleted rows, so a deleted workspace's **name couldn't be reused**. A port race was reported as "name already exists". | Migration 004 makes name, host and port unique among live rows only. `NextPort` picks the lowest free port in `[port_min, port_max]` and retries on a race. Errors are typed (`ErrNameTaken`, `ErrNoPorts`). Live: `pvtest` was recreated with the same name and reused port 22001. (d630357) |
| M10 | Operator templates | `PUT /templates/{id}` with an unknown id **created** a template. A non-UUID id gave 503. Delete of an unknown id returned 204. | PUT and DELETE return 404 unless the template exists. (0bf2b62) |
| M11 | Hardcoded values | Repo hosts, template size limits and messages, the Caddy admin URL, the image-prefix text in the UI, and `PortMin+1000` were hardcoded. | All moved to config (`workspaces.repo_hosts`, `reserved_*`, `template_limits`, `port_max`; hostd `edge.*`). (a9a1593, 0bf2b62, fcb6347) |
| M12 | Seed | The seed file wasn't validated, and the DB differed from the file after the npm fix. | `parseSeed` runs `checkTemplate`, drops duplicates and keeps valid ids. A test asserts the shipped seed is valid. The DB and the file were synced: startup scripts are equal for all 4 templates. pnpm 10+ refused esbuild/swc build scripts under CI, so the node-vite startup now allows them. (0bf2b62, a59625d) |
| M13 | Edge | An unknown `*.ws` host returned an empty **200**. | A catch-all `ws-notfound` route serves a real **404** page, installed last by `deploy/edge-ws-404.sh` (idempotent). (24d9245) |
| M14 | Provider panel | There was no provider workspace delete. | `POST /v1/operator/workspaces/{id}/delete` requires `{"confirm": "<name>"}` and uses the same `runAction` path as the owner. The UI has a typed-name dialog. (b561bfa, fcb6347) |

## Checked, no change needed

- **Authz on every endpoint.**
  - All 15 `/v1/operator/*` routes come from one table behind `platformAdmin`, and a unit test asserts 403 for every non-admin role. Customer routes use `tenantCtx` plus `canManage` (or the creator for previews). `/auth/ws-check` binds the session to the exact host.
  - Live: 10 operator routes return 403 to a non-admin and 401 anonymously. Viewers can't create, act or change the firewall.
- **hostd allowlist.** `run()` executes only `incus|zfs|zpool|sops`, as argv with no shell. All names come from validated UUIDs; size, image and port arguments are validated, and the startup script goes in as stdin, not argv. A unit test covers the allowlist and the id checks.
- **Secrets.** No tokens or keys are logged or returned. The IdP read token comes from env and is never serialised (`json:"-"`). PATs used for testing were shredded on Gold and node.
- **Rules.** No barrel files, nothing named "core", and every changed source file is 150 lines or fewer (the one test file over the limit was split).

## Tests

- **Go unit tests: 31 test functions (≈150 cases), all pass** (`go test ./...`). They cover:
  - authz: every provider route gated, 401 without a token, role mapping, `canManage`
  - session kinds
  - validation: repos, rules, domains, previews, templates, hostd requests, host checks, config fail-closed
  - SSH port allocation and unique-violation mapping
  - preview host parsing
  - template seed parsing, plus the shipped seed and both shipped configs
  - cookie strip in routes
  - Incus URL parsing
  - hostd id checks and the binary allowlist
- **Integration (`deploy/tests/api-integration.sh`, non-destructive, run on Gold): 29/29 non-admin and 39/39 admin, all pass.**

## Live verification on Gold (throwaway tenant `hardening-test`, never the operator's `dev`)

1. **Tenant and workspaces.** Created tenant `hardening-test` (fake org id), then `pvtest` (node-vite plus `joaopaulomoraes/reactjs-vite-tailwindcss-boilerplate`) and `deltest` (blank). A duplicate name returned 409.
2. **Preview.** Signed out it returns 302; signed in it returns 200 with the real app rendered and no Vite error overlay (screenshot `screenshots/fix-preview-app.png`). An unknown host returns 404 (`screenshots/fix-ws-404.png`).
3. **Stop/start.** Stop and start each returned 202, and the DB and Incus agree. A double stop returned 202 and 409.
4. **Suspend with 2 running workspaces.** Both stopped. Start (owner and provider) returned 409, retry provision (customer and provider) returned 409, and a second suspend returned 409. Resume made the tenant active and start worked again.
5. **Provider delete.** A wrong name returned 400; the right name removed `deltest`'s instance, volume, routes and port.
6. **Tenant delete.** A wrong name returned 400. The first real attempts exposed 3 teardown bugs (M7), all fixed, after which it returned 200. Projects, networks and pools are identical to before the test. `ports.json` is identical. There are 0 hub peers, 0 edge routes and 0 nft rules for the tenant, and the platform forward is unchanged. The DB has the tenant `deleted` and its workspaces retired. The encrypted dataset `tank/tenants/01a14263-…` is kept unmounted, by design.

## Open (low)

- L1. `resyncWorkspaces` re-syncs every workspace on **every** retry provision, not just after a network migration. It touched the operator's `dev` route when retry provision was run earlier today. It's harmless (an idempotent republish), but noisy.
- L2. the operator's `dev` route was published before the cookie strip existed. It gets the strip on its next republish (start, sync or retry). It wasn't touched in this pass, by instruction.
- L3. Template edits apply to existing workspaces on rebuild (current startup script and image). Templates aren't versioned.
- L4. Customers see raw hostd error text in the workspace `error` field (for example "Could not stop: incus …").
- L5. Custom domains have no ownership (DNS TXT) verification. Platform names and duplicates are now refused.
- L6. **Resolved 2026-10-16** (see below). `canManage` hardcodes `op_superadmin`, while the panel gate uses `operator.role` from config.
- L7. **Resolved 2026-10-16** (see below). A platform superadmin can open any tenant's IDE and previews (`wsAllowed` via `canManage`). Is that intended for support? There's no audit trail.
- L8. A deleted tenant's org can't re-onboard (`idp_org_id` is UNIQUE across deleted rows).
- L9. The provider tenant list makes one Zitadel call per tenant for the owner (N+1).
- L10. Previews of a stopped workspace give a bare 502 rather than a "workspace stopped" page.
- L11. The `plan_id` on operator tenant create isn't validated against known plans; hostd rejects it and the tenant ends up `failed`.
- L12. `config.Load` still defaults `domain` to `shippin.cloud` (pre-existing).
- L13. Kept ZFS datasets of deleted tenants pile up (by design for recovery). There's no purge tool yet, so the test dataset above is still there.

## 2026-10-16: consent-based platform access and break-glass (resolves L6, L7)

**Design**
- No implicit superadmin access. Platform admins see a tenant's workspaces, IDE, previews and workspace actions only through an active grant. Operator metadata pages stay open.
- Role names come from `roles` in config. The hardcoded `op_superadmin` checks are gone.
- **Request:** a reason (config min/max length), a duration from `access.durations` and a scope (whole tenant or one workspace). Owners/admins see a banner with Approve/Deny, and active grants are listed in Settings, where they can revoke them. The admin can cancel or end a grant. The sweeper (`sweep_seconds`) expires grants and audits it.
- **Enforcement:**
  - Go API: workspace list, actions, access, previews and operator workspace views.
  - Edge forward-auth (`wsAllowed`): every IDE/preview open is audited, deduped per `log_window`.
  - Workspace create and retry provision are members only, because the creator's SSH keys would outlive a grant.
- **Break-glass:**
  - Platform-admin role only.
  - Category from config, plus a justification (min length from config).
  - Fresh Zitadel login within `reauth_minutes`, using the id_token `auth_time`. Bearer PATs never qualify (428).
  - Grant length from config, effective immediately.
  - Distinct `breakglass.*` audit entries (activate, every access, end), red in the audit views, with a persistent admin banner.
  - The customer sees the audit entries and a banner. Only `withhold_notice` categories (legal_hold) may delay the notice, never past `notice_delay`, and the delay itself is audited (`breakglass.notice_withheld`). The customer can't revoke break-glass (409).
- **Audit log:** append-only (`audit_log` UPDATE/DELETE/TRUNCATE and `access_grants` DELETE/TRUNCATE are blocked by triggers). Each entry records actor, tenant, workspace, time and IP. Access is audited fail-closed: if the audit write fails, access is refused. The customer sees their tenant's entries; operators see all.

**Bugs found and fixed during this work**
- CreateGrant returned 503: `make_interval` parameter type. Fixed.
- NULL `ssh_keys` insert. Fixed. Both were caught by the new Postgres tests.
- The control-plane user became ORG_OWNER of an org it created, which made the admin a "member". The test identity script now removes that membership.

**Tests**
- 53 Go tests pass, including Postgres-backed grant lifecycle and append-only tests (`SHIPPIN_TEST_DATABASE_URL`).
- `deploy/tests/access-live.sh` on Gold: 42/43 on the first run. The one failure was the self-approve expectation (404, not 403), now fixed in the script; not re-run.
- Live, on the throwaway tenant "Access Test Co" (the operator's dev untouched):
  - request → deny → 403
  - request → approve → IDE 200, audited with IP
  - revoke → 403
  - expiry with a 1m test duration → refused, sweeper audit entry, status expired (config restored to 1h/4h/24h)
- Playwright walkthrough (break-glass) all OK:
  - activation and IDE audited, customer notice shown, customer revoke → 409
  - non-legal-hold withhold → 400
  - legal-hold delay hidden from the customer, with notice_withheld visible to operators
  - cleanup → 403
- Screenshots are in `screenshots/access-*.png`.

**Open**
- A1. Email notification (options listed below; the operator to choose) for requests and break-glass (in-app banner and audit only for now).
- A2. **Resolved 2026-10-16** (see below). The console DB role owned the tables, so it could drop the triggers.
- A3. Admin data views (lists, settings) aren't audited. Opens, actions and changes are.
- A4. There's no admin SSH path under a grant (SSH keys come only from members).
- A5. Tenant suspend/delete stay platform-level operations without a grant.
- A6. Machine users show their numeric ID as their name.
- A7. Customers can't acknowledge the break-glass banner.
- A8. The access-log dedupe is in memory and resets on restart.
- A9. The client IP trusts XFF from the edge.

## 2026-10-16: audit table ownership (resolves A2) and a clean live re-run

**Ownership**
- `audit_log`, `access_grants` (with the `audit_log` sequence) and `shippin_append_only()` are now owned by `shippin_audit_owner`. It's NOLOGIN with no superuser, createdb, createrole, replication or bypassrls rights. It has no password, so nothing goes in sops.
- The console role keeps only SELECT and INSERT on both tables, plus UPDATE on the grant status columns (`status`, `decided_*`, `expires_at`, `ended_at`) so that approve, deny, revoke, end and expiry work. It isn't a member of the owner role.
- `internal/db/audit_owner.sql` is idempotent. `deploy/audit-owner.sh` runs it as the postgres superuser in the `pg` instance. It's deliberately not an app migration, because the app role can't and shouldn't do this. A future migration that changes these tables has to go through that superuser path too.
- Applied on Gold twice with the same result. A pg_dump backup was taken first: `/var/lib/shippin/site/backups/shippin-pre-audit-owner-*.dump` (mode 600).
- **Proof on Gold**, run as `SET SESSION AUTHORIZATION console`, each statement in a rolled-back transaction:
  - "permission denied": UPDATE, DELETE and TRUNCATE on both tables; UPDATE of a non-status grant column; SET session_replication_role; SET ROLE shippin_audit_owner.
  - "must be owner": ALTER TABLE … DISABLE TRIGGER (one trigger or ALL), DROP TRIGGER, DROP TABLE, ADD COLUMN, CREATE OR REPLACE of the trigger function.
  - INSERT and SELECT work.
- **Test:** `TestAppRoleCanOnlyAppendAudit` logs in as a real non-superuser app role. It checks that the role can run the grant lifecycle, audit writes, expiry and reads, and that the statements above (plus GRANT of the owner role and ALTER … OWNER) are refused. It also applies the SQL twice to check it's idempotent. All 54 Go tests pass.

**Live re-run**
- Recreated the test identities with `access-test-identity.sh up`. Its `up` path now reactivates a deactivated owner.
- `deploy/tests/access-live.sh`, with a temporary 1m duration for the expiry check, ran against the console under the new permissions: **43/43 passed**.
- Afterwards: config restored and console active, test users deactivated, ops-test-admin's IAM role removed, credential files shredded. No permission errors in the console log.

**Email options for A1 (research only; nothing signed up or bought)**
1. Amazon SES (SaaS, cheapest): $0.10 per 1,000 emails à la carte, about $1 a month at our volume. Needs an AWS account, DNS records (SPF, DKIM, DMARC) and a request to leave the sandbox. Bounce handling is basic, via SNS.
2. Postmark (SaaS, best deliverability and simplest): free for 100 a month, then $15 a month for 10k. Separate transactional streams, good bounce and suppression handling, SMTP or API.
3. Self-hosted on Gold (Postal or Stalwart, or Postfix) relaying out through a small VPS with clean IP/PTR (about $5 a month). No per-email cost and full control, but we'd own IP reputation, warm-up, blocklists and bounces. Gold sits behind a NAT/residential uplink, so direct sending from it would land in spam. More ops work than it's worth at our volume.
- Also considered: Resend (free for 3k a month at 100 a day, then $20 a month for 50k; nice API, younger company).
- Suggestion: Postmark or SES, behind a small `notify` interface with SMTP settings from config and credentials in sops, so switching providers stays cheap.

## 2026-10-16: E-mail notices (Shippin Mail)

**Mail server.** Stalwart v0.16.26 (tag commit 18f4e95) is built from source on Gold with the 18
branding edits and the `enterprise` feature left on. It runs as Shippin Mail in container `mail`
(platform project, 203.0.113.70; .60 belongs to pdns-primary). The repo is `/srv/projects/shippin-mail`.
- Secrets are in sops (`mail_admin_password`, `mail_notices_password`, `mail_test_password`,
  `mail_tls_key`). The bootstrap admin secret was written to a 0600 file and replaced. Nothing
  was printed.
- DKIM keys: ed25519 and rsa, selectors `v1-*-20261016`. The DNS records are prepared in
  shippin-mail `docs/dns-records.md`: SPF, DKIM, DMARC `p=quarantine`, with MX and A later. They
  are not applied.
- Nothing leaves Gold, enforced two ways:
  - Remote mail goes to a "relay" route parked on 127.0.0.1:1, so it stays queued.
  - The OVN ACL `mail-egress` rejects outbound TCP 25/465/587/2525. Tested from the container:
    smtp.gmail.com on 25/465/587/2525 got connection refused, while 443 connected.

**Console mail module** (`internal/mail`, `internal/db/mail.go`, migration 006):
- Config-driven SMTP: implicit TLS to 203.0.113.70:465 with the internal cert pinned via `ca_file`, PLAIN auth.
- Templates, and a Postgres outbox with dedupe key, lease (SKIP LOCKED), exponential backoff and
  permanent failure on 5xx.
- Notices:
  - access request goes to the tenant managers
  - approve, deny, revoke and expire go to the requesting admin; expire also goes to the managers
  - break-glass goes to the managers; a legal-hold notice is queued with `send_after = notice_at`
  - workspace ready goes to the creator, or to the managers when the creator is a machine user
- Recipients come from Zitadel.
- Tests:
  - Go: fake SMTP covering TLS, STARTTLS, untrusted CA, 4xx/5xx, dedupe and escaping.
  - DB-backed notifier test, including the legal-hold delay.
  - All pass, including with -race.

**Mail admin.** https://mail-admin.shippin.cloud uses a Let's Encrypt DNS-01 cert. It sits behind the
console's forward_auth through `operator.admin_hosts`, platform admin only, with access logged per window.
- A tenant owner's token gets 403 at ws-login. Anonymous requests are redirected to sign-in.
- The Stalwart login still applies after that.

**Live run** (throwaway tenant Access Test Co; `deploy/tests/mail-live.mjs` + shippin-mail `deploy/mail-verify.py`):
- All 15 trigger steps passed.
- All 7 notice types arrived in mailtest@ (sub-addressed +admin/+owner). Both DKIM signatures
  (ed25519 and rsa) verify against the server's own zone. Auto-Submitted is set and the links are present.
- The legal-hold notice is pending in `mail_outbox` until 2026-11-15 11:22 PT.
- An outside probe stayed queued with a connection error to the parked relay. The probes were removed afterwards.
- Observed: Stalwart's default inbound throttle (5 connections/s per IP) refused 3 of the first
  burst. The outbox retried them 60 s later and all were sent. The retry is the intended behaviour;
  the upstream default was left as is.
- Cleanup: the temporary `1m` duration was removed, test identities deactivated (with the new
  human owner), credential files shredded. the operator's dev untouched.

**Open / minor.**
- The break-glass notice shows the category id (`security_incident`), not its label.
- The Stalwart dashboard page is enterprise-gated (lock icon). The admin screenshot shows Accounts.
- No Received header is added for authenticated local delivery. The rebranded greeting is
  verified on the SMTP banner instead: `220 mail.shippin.cloud Shippin Mail ESMTP`.
