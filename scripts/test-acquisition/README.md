# Disposable acquisition acceptance

Root orchestration owns `deploy/tests/acquisition.compose.yaml`. These test artifacts do not deploy or configure any real installation.

## Browser environment

- `ACQUISITION_TEST_ISOLATED=1`
- `ACQUISITION_TEST_ORIGIN=https://127.0.0.1:25444`
- `ACQUISITION_TEST_SIGNUP_ORIGIN=https://127.0.0.1:25443`
- `ACQUISITION_TEST_SELF_SIGNED=1` permits only the guarded loopback fixture browser origins. Panel-to-source TLS remains verified.
- `ACQUISITION_TEST_PASSWORD_FILE`: private file containing the generated fixture password, not an env-file with other settings.
- `ACQUISITION_TEST_SETUP_CODE_FILE`: private current fixture setup-code file, bootstrap only.
- `ACQUISITION_TEST_PLAYWRIGHT_ROOT`: directory containing installed locked Playwright package.json (existing runner scripts/browser may be used).
- `ACQUISITION_TEST_IDS_FILE`: private JSON output containing only canonical fixture subject IDs.
- `ACQUISITION_TEST_RECORD_FILE`: private nonsecret fixture-record output for subsequent persistence/tombstone phases.
- `ACQUISITION_TEST_SIGNER_FILE`: private synthetic Ed25519 PKCS8 signer file; never print.
- `ACQUISITION_TEST_SOURCE_CA_FILE`: fixture source admin certificate PEM.

PowerShell launcher requires explicit `-PrivateFixturePath` and `-PlaywrightRoot` (or ACQUISITION_FIXTURE_PATH / ACQUISITION_TEST_PLAYWRIGHT_ROOT). It never defaults to another checkout. Bootstrap captures only the fixture panel setup code privately.

Commands from platform repository root:

```
node web/scripts/test-acquisition-integration.mjs bootstrap
# Root restarts ONLY the new fixture panel to seed dev accounts.
node web/scripts/test-acquisition-integration.mjs prepare
node web/scripts/test-acquisition-integration.mjs verify
# Root recreates fixture containers with retained volumes.
node web/scripts/test-acquisition-integration.mjs persistence
# Root runs the private helper delete inside the fixture network namespace.
node web/scripts/test-acquisition-integration.mjs tombstone
```

The verify, persistence and tombstone phases passed against the isolated fixture on 2026-11-14. The recorded platform row was additionally checked for terminal deletion, NULL payload/expiry and an advanced revision. These are separate observed phases, not evidence of live deployment.

Dev account reseeding restores direct configuration grants. Re-run prepare after a panel restart before permission qualification. The password limiter is real and must not be disabled: prepare uses four logins; verify uses three. Repeated test batches may need the limiter window to expire. Never present a throttled login as a credential failure.

## Private fixture helper

Build stdlib-only `scripts/test-acquisition/helper` with CGO disabled. Root owns Containerfile and Compose.

- `/acquisition-fixture proxy`: requires `SHIPPIN_ACQUISITION_DISPOSABLE=1`; TLS on namespace18443 to HTTP12718080; preserves Host; `/secrets/proxy-cert` and `/secrets/proxy-key`. Generate them once using `/acquisition-fixture proxy-cert` with disposable opt-in and the writable owned secrets volume. This creates separate P-256 browser-compatible TLS material and refuses to overwrite; Ed25519 source-admin and signing keys remain untouched.
- `/acquisition-fixture delete`: same opt-in; `ACQUISITION_TEST_LEAD_ID` must be a lowercase32hex ID. Uses `/secrets/admin-signer` and `/secrets/admin-cert`; verified TLS12718082, issuerfixture/audiencesource-admin/tenantcontainer/projectsignup, read+write delegated scope. Reads actual target and refuses anything not active with an `@example.test` address before DELETE. Does not publish admin ports, accept Origin/cookies, or log email/keys/tokens.

Start signup/proxy and await readiness before admin: admin correctly fails closed until the existing scope is initialized. Never rerun source prepare to fix startup ordering, because it generates new source keys.

A successful source DELETE is not proof of panel deletion: follow with tombstone sync/read and repeat-sync assertions.
