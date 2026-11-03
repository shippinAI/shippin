# sign-p0: passkey signing for Gold approvals (box only; nothing deployed or installed)

Written 2026-10-20 (PDT). Builds on guardrails `docs/passkey-approvals-design.md` and the `cert:` source in
`research/agent-tickets/AGENT-TICKETS-PLAN.md` §C3/C5/C6, with one deliberate change: **no relying-party
service key**. Gold verifies the operator's raw WebAuthn assertion offline against pinned public keys (plan §C3), so
neither the console nor any service holds a key that can mint an approval.

## Flow
1. gold-policy on Gold exports review facts per ticket (`{id, sha256, tier, tier_reasons, classes, host,
   oob_path, commands, rollback, verification, requested_at, agent_summary}`) into `GOLDSIGN_FACTS_DIR`
   (read-only sync). The console shows these facts and the full sha256; agent text is shown last and labelled.
2. the operator taps **Sign** or **Deny** (equal size). The browser calls `navigator.credentials.get` with
   `challenge = sha256(JCS{decision, ticket_id, ticket_sha256, ts})`, `userVerification: "required"`,
   `allowCredentials` = his registered credentials.
3. The console relays the envelope (payload, kid, credential id, authenticatorData, clientDataJSON,
   signature) to its outbox. It checks shape and the current sha256 only to fail fast; that check has no authority.
4. On Gold, `gold-passkey-verify.timer` (root) pulls the outbox (see Sync) and runs `gold-passkey-verify drain`, which for
   each envelope checks: key pinned **and** `owner == operator`; credential id; `type == webauthn.get`; challenge
   equals sha256 of the canonical payload; origin allowed, not crossOrigin; rpIdHash; UP+UV flags;
   ES256/EdDSA signature; `ts` no older than 5 min and not more than 1 min in the future; sha256 of
   `/var/lib/shippin/changes/tickets/<id>.md` equals the signed hash; challenge not in the root used-set (replay).
5. Only then it writes `/var/lib/shippin/changes/approvals/<id>.approval` in gold-approve's exact field order, chowned
   to operator, 0644, via tmp + rename, and appends a `decisions.jsonl` entry (`source: gold-passkey`). Every
   refusal is logged as `result: unverified-passkey` and writes nothing. `gc_check_approval` is unchanged.

Approval file (identity shows the passkey; everything else is identical to `gold-approve`):
```
decision: approved
approved_by: operator
identity: operator@passkey:example
ticket_id: GC-20261020-01
ticket_sha256: <64 hex>
approved_at: <unix now>
reason: passkey approval (example)
```

## Why no agent can mint or forge an approval
- The only private keys are inside the operator's authenticators (phone/desktop passkeys, UV required).
- Gold trusts only keys in root-owned `/etc/shippin/signer/owner-keys.json`, added by a reviewed T2 ticket.
  `gold-passkey-verify` refuses to start if that file is not root-owned or is group/world writable.
- Registration in the console is limited to the exact Nomen subject `GOLDSIGN_OWNER_SUB` plus a Nomen sign-in
  in the last 10 minutes, but it is **not** trust: a key registered by a compromised console or Nomen is
  ignored by Gold until the operator pins it after comparing the fingerprint shown on the Passkeys page.
- Envelopes carry no secret. An agent that copies or injects one gains nothing: a replay hits the used-set,
  an edit breaks the challenge, a stale ticket fails the hash, an old one is expired.
- The console never writes approval files and has no Gold write path; Gold pulls.

## Bypass certificate badge
The app-shell header always shows a React Aria badge (every page, phone width included): `No bypass active`,
or `Bypass active · <agent> · <scope> · <time left> left` (red). A popover lists every active certificate
(agent, full scope, time left and expiry, valid-from, uses, cert id, reason) and a **Revoke** button that signs
`{cid, decision: "revoke", ts}` with the passkey; Gold's verifier records it in `revoked/<cid>`. The badge stays
red ("Revoke sent; still active until Gold confirms") until the revocation syncs back. If status cannot be
loaded the badge says `Bypass status unavailable` (amber), never "none". Certificates are read from
`GOLDSIGN_DIR/certs/` (cert service export, plan §C); the console cannot issue one.

## Sync (Gold <-> console, no signing keys)
`gold-passkey-verify.service` (root, every 15 s via the timer) runs three steps:
1. `gold-sign-export pull`: GET `/v1/gold-sign/outbox` into the root-only inbox.
2. `gold-passkey-verify drain`: verify, write approvals (skips envelopes already in the used-set).
3. `gold-sign-export push`: PUT `/v1/gold-sign/sync` with a full snapshot: facts parsed from
   `gold-policy review <id>` for every ticket with no approval for its current sha256 (line counts checked,
   sha re-hashed), the accepted bypass certs from `GOLD_CERT_DIR` (badge fields only), `revoked/` and the
   last 48 h of the used-set. The console swaps facts/, certs/, revoked/ atomically and drops consumed envelopes.
The only credential is the sync bearer (`/etc/shippin/signer/gold-sign.token` on Gold, `GOLDSIGN_SYNC_TOKEN` on
the console). It reads envelopes and replaces synced copies; it cannot sign, approve or issue a cert. The worst
case if it leaks is misleading facts on screen, and Gold still re-hashes the ticket before writing anything.
Sync is https only. `GOLDSIGN_SYNC_CIDRS` is required: without it the sync routes refuse every request
(fail-closed, see approve-p1.md). If no sync arrives for 2 min, or Gold cannot read its cert store, the
badge reads "Bypass status unknown (Gold not synced)" and the pending list warns that it may be out of date.

## Limits
- `GOLD_CERT_DIR` is where the future gold-policy `cert:` source stores accepted certs. It is absent today,
  so "No bypass active" is true until that source exists.
- Approve and Deny for the same hash are both valid; the later verified one replaces the file.
- Synced passkeys usually report sign count 0, so replay protection is the used-set, not the counter.
- Pinned keys are removed only by a T2 ticket; a lost phone stays trusted until then (Passkeys page note).
- Sign stays disabled until the end of the commands list has been on screen above the decision bar. Deny is
  always enabled.
- `gold-policy review` runs once per pending ticket per 15 s poll (no cache).

## Console config (when deployed; not done)
`GOLDSIGN_DIR`, `GOLDSIGN_FACTS_DIR`, `GOLDSIGN_OWNER_SUB` (the operator's Nomen `sub`), `GOLDSIGN_RP_ID`
(console host), `GOLDSIGN_ORIGINS` (space-separated, e.g. `https://console.<domain>`),
`GOLDSIGN_SYNC_TOKEN` (sync bearer, from Vaultix; never logged), `GOLDSIGN_SYNC_CIDRS` (required for sync). All unset = feature off.

## Install on Gold
See the gold-change ticket draft `GC-20261020-SIGN0` (guardrails `drafts/install-sign0/`). the operator runs its
pinned `install.sh` as root in gold-rec: snapshot, 600 s revert plus a boot revert, verification within 60 s,
and owner-key pinning checked against the fingerprints written in the approved ticket.

## Tests (box)
`go test ./internal/goldverify ./internal/goldsign`; `cd web && npm test`; `deploy/screenshots/shots-gold-sign.mjs`
(38 browser checks + 16 screenshots in `docs/screens/sign-p0/`).
