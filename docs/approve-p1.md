# approve-p1: HTTPS click-approve (Phase 1 of MOBILE-APPROVAL-DEVICES-PLAN.md)
Builds on sign-p0. Box only; nothing deployed.

## What changed vs sign-p0
- **Signed payload has a `type`** (`approval` for approve/deny, `revoke` for cert revocation). It is part of
  the JCS canonical string and must match `decision`; Go (`goldverify.Payload`) and TS (`canonical.ts`) share
  one test vector. **Breaking:** a sign-p0 (b120867) verifier refuses approve-p1 envelopes and vice versa, so
  Gold must get verifier binaries built from this branch (ticket GC-20261020-SIGN0 pins must be rebuilt).
- **Public host from env:** `CONSOLE_PUBLIC_HOST` (bare DNS name, e.g. the decided public console host) is the
  WebAuthn RP ID and gives origin `https://<host>`. `GOLDSIGN_RP_ID` / `GOLDSIGN_ORIGINS` still override.
  Hosts with a scheme, port, path or an IP literal are refused (feature stays off).
- **Owner name from env:** `GOLDSIGN_OWNER_NAME` (must equal `GOLD_APPROVER` on Gold) replaces the hardcoded
  name in key IDs, the pinned `owner` field and the WebAuthn user name.
- **First-time path:** a ticket page with no passkey shows "Set up passkey" inline (one tap, UV required,
  fresh Nomen sign-in still enforced server-side), then Approve/Deny. The Passkeys page uses the same hook.
- **Approve** replaces "Sign" as the button label; it triggers `navigator.credentials.get` with
  `userVerification: "required"` and challenge = sha256(canonical payload).
- **Bypass badge** stays in the top bar on every page; its details popover (revoke + WebAuthn code) is now
  lazy-loaded. Initial JS: 89,980 B gz of the 90,000 B budget (sign-p0 base 89,954 B).

## Console env (all required unless noted; secrets via Vaultix, never in files)
`CONSOLE_PUBLIC_HOST`, `GOLDSIGN_DIR`, `GOLDSIGN_FACTS_DIR`, `GOLDSIGN_OWNER_SUB` (owner's Nomen `sub`),
`GOLDSIGN_OWNER_NAME`, `GOLDSIGN_SYNC_TOKEN` (Vaultix), optional `GOLDSIGN_RP_ID`, `GOLDSIGN_ORIGINS`,
`GOLDSIGN_SYNC_CIDRS`.

`GOLDSIGN_SYNC_CIDRS` is strict: one invalid entry turns the feature off (logged). Unset is fail-closed:
`/v1/gold-sign/outbox` and `/sync` refuse every request (logged at start); sync needs both the bearer and a
source inside the list. The console sees the address of the hop in front of it (behind Caddy, the edge proxy),
so the list must name that hop; it then narrows sync to traffic arriving through it, with the bearer still the
real secret. Gold's `gold-sign-export` accepts https URLs only.

## Tests
`go test ./internal/goldverify/ ./internal/goldsign/ ./internal/api/`; `npm test`; Playwright
`deploy/screenshots/shots-gold-sign.mjs` (mock API, fake authenticator) writes `docs/screens/approve-p1/`.
Not tested: a real platform authenticator, real Nomen login, real Gold verification of a browser-made envelope.
