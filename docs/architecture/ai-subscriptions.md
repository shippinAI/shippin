# AI subscriptions (M2)

Each user connects their **own** Claude, ChatGPT/Codex and Grok subscriptions on an isolated
owner runner. **Devagotchi.com is the central hub**, integrated into Shippin.cloud. A devspace
is a hub client, not a replacement hub. The central tenant/workspace grant and runner adapter
are not implemented yet; this patch secures sign-in and explicitly blocks the legacy build
path that relied on shared credential mounts.

- Shippin accepts the Anthropic Commercial Terms. **Claude Code and Codex are preinstalled** in
  every workspace image; the Grok CLI is preinstalled when its npm package installs (optional).
- Each user activates their own subscription with the **vendor's own login**, inside **their own
  container**. The login is persisted per user on their **AI home volume** `ai-<account>`, mounted
  at `~/.ai`. It is never shared between users and never copied into Shippin.
- Share authorized project state and messages through the hub's scoped adapters. Never mount
  the credential volume into a workspace or Spark, including a workspace created by its owner:
  tenant managers and support grants can also enter those workspaces as the same Unix user.
- Free personal cloud, commercial-grade RBAC: only the owner can open their activation terminal
  (`wsauth` → `utilityAllowed`). Tenant owners, admins, operators and grant holders cannot.

Risk note: each vendor's own terms govern the user's use of their subscription inside Shippin.

## Pieces

| Layer | What |
|---|---|
| DB `017_subscriptions.sql` | `ai_homes` (one per user per placement), `subscription_connections` (status only). `CHECK (runner <> 'hosted_workspace' OR token_ref IS NULL)`: the database refuses credentials. |
| Image (`deploy/image/ai/`) | Pinned official CLIs; private credential directories; `shippin-ai-status` v2 reports supported status command exit codes, timestamps and `auth_known`. Grok 1.0.62 has no supported noninteractive status command here and reports unverified. No credential contents are read. |
| hostd | `ai-<account>` volume only in `u-<account>`; verifies Incus volume ownership, all attachment references, instance owner/tenant and exact expanded device before publishing. Conflicts fail closed. Legacy workspace attachment requests are rejected. |
| Console | `GET /v1/me/subscriptions`, `POST …/subscriptions/{provider}/activate` (job `ai.activate`), `POST …/check`, `DELETE …/subscriptions/{provider}`, `POST …/ai-terminal/touch`, `DELETE …/ai-terminal`; idle stop in the reconciler. |
| Web | Account → AI subscriptions, sign-in dialog, devspace chip. |

## Flow

1. User presses **Connect** → console ensures their `ai_homes` row and enqueues `ai.activate`.
2. hostd creates `ai-<account>` (first time), starts `u-<account>`, publishes
   `https://u-<32 hex>.<ws base>/` (tenant/account-bound, owner only).
3. User opens it and runs the vendor command (`claude auth login`, `codex login --device-auth`,
   `grok login --device-auth`). Keep the CLI waiting. Open the provider URL in the user's local
   Chrome/Edge/regular browser and complete its own YubiKey/passkey challenge. The key stays
   local; no USB forwarding to Gold is needed. A web page cannot force an embedded browser
   into a particular system browser, so provide a copyable sign-in address.
4. If either browser session or device code expires, cancel the waiting CLI login and run it
   again for a fresh code/session. Never reuse an expired challenge URL or disable account security.
5. **Check** requires successful refresh, version 2, fresh timestamps and unique provider rows.
   Missing/unsupported results become `limited` (UI: Sign-in unverified); they never remain connected.
   `connected` means CLI sign-in only, not available quota or verified Daybreak entitlement.
6. The central hub's runner adapter must enforce owner, tenant, workspace, capability, expiry and
   revocation before any subscription-backed job. Until implemented, the old automatic workspace
   build path returns an explicit unavailable error and preserves the pending prompt.

## Config

- Console `ai`: `enabled`, `providers[] {id,label,vendor,login_command,device_url?}`, `idle_minutes`,
  `touch_seconds`, `tenants` (rollout allow-list; empty = all), `egress_hosts`, `terms_note`.
- hostd `ai`: `home_path`, `home_gib`, `volume_shifted`, `user`, `uid`, `status_path`,
  `status_unit`, `status_max_bytes`, `utility {image,cpu,memory,ttyd_port,ttyd_iface,ttyd_unit,
  ttyd_env_file}`, `logout {provider: argv}`. Unset `home_path` disables every AI route.
- The idle stop runs in the reconciler, so it needs `jobs.enabled=true`.

## Egress

The CLIs need the hosts in `ai.egress_hosts` (Anthropic, OpenAI, xAI, npm). Gold's legacy tenant
bridges have no egress allow-list today, so nothing is enforced yet; when allow-lists land, they
read this list.

## Known limits

- **Not deployed.** Coordinate hostd, image status v2, console and UI. Old status v1 is rejected.
- Before enabling, stop existing activation runners, snapshot their private volumes and remove
  every legacy `ai-home` workspace device without deleting the underlying volume. Inventory
  parent and nested Incus projects. Keep activation disabled until the attachment audit is clean.
- Move each home to its new tenant/account-bound terminal hostname and route ID. Old account-only
  terminal hosts are denied until this migration. Preserve the unique `utility_host` constraint.
- Before publishing any terminal, authenticate the edge-to-ttyd connection (for example mTLS)
  and deny direct sibling/other-tenant traffic. The current image runs plain ttyd; forward-auth
  at the edge is insufficient. Verify real HTTP/WebSocket denial, failure to publish, reboot
  and session revocation. These are still open launch blockers.
- The status report is advisory CLI state from a user-controlled runner, not an entitlement,
  resource grant, tenant authorization or proof of Daybreak access. A separate authenticated
  resource/runner grant and actual provider response must gate hosted execution.
- Existing template images remain unsafe if they carry credentials; retain the publish guard.
- The live console founder-access fix at `f64b96c` must be preserved in a reproducible release.
- the operator corrected the central domain to `devagotchi.com` on 2026-10-22. Its DNS resolves and
  HTTPS currently redirects to `https://devagotchi-com.l.ink/`; it does not yet serve the hub.
  The Cloudflare zone is accessible in the operator's account; the registrar is Porkbun. Replace the
  landing-page route only after the authenticated hub is ready. Nomen integration, tenant
  isolation and scoped runners remain deployment prerequisites. The earlier `.net` DNS
  failure concerns the wrong domain and is not a blocker for this deployment.

References: [Codex device authentication](https://learn.chatgpt.com/docs/auth),
[Daybreak hardware-key requirements](https://help.openai.com/en/articles/20001259-openai-daybreak-common-issues-and-troubleshooting).

- AI homes are keyed by tenant (boundary empty) because hostd places every workspace of a VM tenant
  in its tenant VM. When hostd places by boundary VM, key homes by boundary.
- `volume_shifted` stays false. The gold check, and why a failed `security.shifted` create would
  break every AI home, is in `docs/architecture/org-storage.md`. Without the shift, containers
  with isolated idmaps see the shared volume under different uids. `ownAIHome` chowns the mount
  point only.
- `check` runs inline (one hostd status read, 45 s cap) rather than as an `ai.check` job.

## Exact Gold handoff (2026-10-22)

This is a blocked handoff, not an approved execution ticket. Do not restart public sign-in to
test the UI until the following items have evidence:

1. Recover or independently reconstruct and review the live founder-access fix from
   `/opt/console/releases/20261021185722-f64b96c-founder`. Its recorded commit is
   `f64b96ced775325298acafde0d0ca0fac1f9d779`; it is absent from the inspected GitHub/devspace
   and `/root/src-sync-20261021/bundles/shippin-console.bundle` heads. Keep the live binary.
2. Replace plain ttyd backend access with authenticated, encrypted transport and network
   isolation, then test direct-IP HTTP/WebSocket denial from sibling workspaces and other
   tenants, plus reverse access from the runner, before and after reboot. This needs code and
   network verification, not an approval checkbox.
3. Keep activation disabled during coordinated hostd, image status-v2, console and UI rollout.
   Snapshot AI volumes and DB metadata privately. Preserve credentials without reading them.
   Inventory both parent and nested Incus projects; remove legacy shared-workspace devices.
   Verified the operator legacy attachment: project `t-00000000-0000-4000-8000-000000000001`, instance
   `w-01a1414a-4346-7d6c-ab90-87332303e90d`, device `ai-home`, pool
   `p-00000000-0000-4000-8000-000000000001`, source `ai-01a15525-132b-71cd-bc53-f25bc2c4940f`.
   This parent volume is distinct from the identically named nested volume. Inside
   `tvm-00000000-0000-4000-8000-000000000001`, project
   `pj-01a15525-132b-71cd-bc53-f25bc2c4940f`, the nested `default` pool volume is attached only
   to `u-01a15525-132b-71cd-bc53-f25bc2c4940f`, verified **Stopped**. Do not conflate them.
4. Migrate existing `ai_homes.utility_host` values using `subs.TerminalLabel(tenant, account)`
   and remove old account-only Caddy routes. The new code deliberately refuses old hosts.
   Preserve unique host-to-home mapping and verify denied access after membership revocation.
5. Deploy the central hub at `devagotchi.com`, replacing its current landing-page redirect in
   the operator's Cloudflare account after staging verification. DNS authority is already working.
   The hub needs Nomen identity, tenant-scoped data, workspace/resource grants,
   owner runner adapters and revocation. No hub container/route was found in Gold's platform
   inventory. Current agent-hub code does not implement these hosted guarantees.
   Verified Cloudflare zone `7b8a176774301f13d5773f00d223c057`, account
   `ada34de5c97b755fd886372fc7b339a7`: apex A records are `207.207.210.36` and
   `207.207.210.50`; `www` and wildcard CNAME point to `uixie.porkbun.com`, all proxied.
   Preserve email MX/SPF and unrelated records. No DNS changes have been made.
6. Pin the completed release hashes and Daybreak-reviewed checkset in a Gold ticket. Current
   ruleset SHA is `ebe610e267ed08da2e7237cbcba2b57270b3ff4d8938beb14674b4e3b64da010`.
   `/usr/local/sbin/gold-policy` requires exact-ticket the operator approval for system/network changes,
   a fresh snapshot (maximum age 3600 seconds), topology, blast radius and documentation.
   Network changes also need verified independent recovery access and automatic rollback
   (10–300 seconds); health verification must finish within 60 seconds. Do not self-approve
   or change policy to make the release pass. The earlier bastion review also remains blocked.
7. After approved deployment: fresh provider device code → local browser YubiKey/passkey →
   official CLI success → fresh status → actual authorized provider call. Verify Daybreak
   access separately with the intended account/workspace, then reconnect after restart and
   prove revocation denies new calls and closes active sessions. Rollback must keep activation
   disabled, not restore shared credential mounts or unauthenticated terminals.

## Verification and review checkpoint

Targeted hostd tests passed on Windows and Linux ARM64 (Spark 01), including rejected workspace
mounts, wrong owner/device/pool/path/project, shared volumes, attach conflicts, status freshness,
and logout refusing an unverified volume. The actual image status script passed Linux syntax
and positive/signed-out/error/unsupported/missing-CLI tests with disposable fake probes.
PostgreSQL-backed API regressions passed for activation, canceled checks, valid negative reports
after cancellation, atomic rollback, sticky revocation, tenant-specific terminal hosts and
membership denial, and preservation of prompts when the runner adapter is absent. Frontend
tests, production build/budget and typecheck passed; the dialog was inspected in a local preview.

Actual `gpt-daybreak-blue-latest` reviews are saved locally in
`.run/daybreak-subscriptions-r{1,2,3}-review.md`; pinned source manifests accompany revisions 2/3.
R3 permits retention for further development only. It explicitly blocks exposed deployment.
The final fresh-context write for a valid report and its database regression were added after
R3; this small follow-up and documentation updates are not represented as independently approved.

R3's unique-host concern is already covered by `017_subscriptions.sql` (`utility_host text UNIQUE`).
Route IDs are tenant/account scoped, not hashed; hashing them was not a security requirement.
Remaining real work includes authenticated backend transport, durable cleanup and cross-process
lifecycle fencing, current membership checks for queued jobs, closing established sockets on
revocation, activation/check generations, and status request throttling. Status remains advisory.
These code requirements precede a deployable Gold ticket; the operator approval alone cannot fix them.

## Hardware-key sign-in and lifecycle follow-up (2026-10-22)

The earlier browser/device attempt expired without logging the remote CLI in. A fresh official
device flow completed after the operator's browser approval. Spark 01's official Codex 0.163.0 then
reported **Logged in using ChatGPT**. A separate invocation of `gpt-daybreak-blue-latest` with
`--cyber-access-program daybreak_blue` returned the expected `DAYBREAK_ACCESS_OK`, exit 0.
Both API-key environment variables were unset; this exercised the operator's subscription identity.
The same Spark identity performed the R4/R5/R6 source reviews. Provider credential contents were
not read or copied to Gold, Shippin, a shared devspace, or the hub.

R4 identified an early-start cleanup gap. The follow-up withdraws and confirms route absence
before placement/instance discovery, requires a successful typed Incus inventory, verifies
owner/tenant before mounting or stopping, and compensates failures even after cancellation.
Service restart errors propagate. Route absence and stopped state must be confirmed separately;
failed starts use a fresh DB context and error-state homes are retried without the idle delay.
An independent timeout is reserved for stopped-state verification. These changes do not add
backend authentication, durable cross-process lifecycle fencing, or a cleanup outbox.

Windows and Linux ARM64 failure-injection tests and actual PostgreSQL API regressions passed.
R5's failed-stop recording and malformed inventory findings were repaired; R6 confirmed those
fixes. Follow-ups after R6 atomically clear the cleanup error on confirmed stop (volume state
returns to `creating` until verified by activation), record successful cleanup with a fresh DB
context, and reject Unicode whitespace/control characters in inventory names. These final
follow-ups have regression coverage. The 2026-10-22 actual Daybreak rollout review accepted all
three and identified a low-severity gap in the remaining inventory-name grammar. The follow-up
now requires Incus' documented ASCII instance names, including start/end and length constraints;
Windows and Linux ARM64 regression tests passed. No production deployment is implied.
An additional actual Daybreak source pass confirmed that the inventory-grammar finding is closed.
Source snapshots, hashes and actual Daybreak outputs are in
`.run/daybreak-subscriptions-r{4,5,6}-*`; these are review evidence, not Gold approval or a claim
that the hosted hub integration is operational.

Spark 01 now has a persistent owner launcher at `/var/lib/shippin/operator/.local/bin/devagotchi-codex`.
It uses the pinned official CLI and `/var/lib/shippin/operator/.local/share/devagotchi/providers/codex` (0700;
provider credential file metadata verified 0600), with API-key variables unset. The temporary
login home was renamed in place on the same Spark; no provider credentials were transferred.
`devagotchi-codex login status` succeeded after this change. Provider refresh/revocation still
applies; this is not a permanent or shared service identity.

The separate Shippin bridge returned HTTP 401, then successfully renewed through Nomen as the
same `shippin-bridge on dabo` account. A real bridge exec into the operator's existing devspace succeeded
and updated `/home/dev/.local/share/shippin/guides/devagotchi-bridge.md` to the `.com` guide
(SHA256 `7447235dd22aed1b2b34274bdb8058323c44a0171fc2dcc13259ed36c4b6000f`). This verifies
devspace access separately from provider access; the central hub adapter remains unimplemented.
