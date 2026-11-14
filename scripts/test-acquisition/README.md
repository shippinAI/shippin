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
- `/acquisition-fixture delete`: same opt-in; `ACQUISITION_TEST_LEAD_ID` must be a lowercase32hex ID. Uses `/secrets/export-signer` for GET (issuer `fixture`, audience `source-admin`, read scope) and the preserved `/secrets/admin-signer` for DELETE (issuer `fixture-management`, audience `source-management`, write scope). TLS uses `/secrets/admin-cert`. Reads actual target and refuses anything not active with an `@example.test` address before DELETE. Does not publish admin ports, accept Origin/cookies, or log email/keys/tokens.

Start signup/proxy and await readiness before admin: admin correctly fails closed until the existing scope is initialized. Never rerun source prepare to fix startup ordering, because it generates new source keys.

A successful source DELETE is not proof of panel deletion: follow with tombstone sync/read and repeat-sync assertions.


## Separated authority migration qualification

This is a synthetic fixture migration, not production provisioning or CI browser coverage. Source runtime must be built from reviewed PR4 commit `d47aad0b25ab6881aa5b10429cc4a833d05bc08e`; never rerun the source `containercheck prepare` against preserved volumes. The old private `fixture-record.json` remains historical, including its already deleted lead. New files are `separated-baseline.json` and `separated-record.json`, created exclusively.

1. Verify existing Compose project/service labels and named volumes. Start only the existing database containers first, preserving data. Build the updated stdlib helper image using `deploy/tests/Containerfile.acquisition-fixture`. Do not recreate the old admin process until its valid baseline is captured.
2. Set `SHIPPIN_ACQUISITION_DISPOSABLE=1`, then run `run.ps1 -Phase setup -PrivateFixturePath <private-directory> -PlaywrightRoot <installed-root>`. This accepts only the exact existing fixture database/scope, creates a new nonmember/nonowner export login with SELECT on four source tables, and rejects an already existing role instead of blindly changing it. The helper exclusively creates export files, validates complete existing sets, and renews expired certificates only after backup using the same TLS keys. Partial setup must be inspected, not retried blindly.
3. Copy the synthetic `/secrets/export-signer` into a new private `export-signer` file and renewed `/secrets/admin-cert` into new `separated-admin-cert`; never overwrite original private files or print secret material. Start preserved original signup/proxy/panel/admin containers. Wait for actual readiness. `prepare` reinstates explicit fixture read permissions and revokes reader mutation grants after dev-account reseeding.
4. Run `baseline` against the original admin-only process. It uses the actual UI for CA-only renewal if necessary, asserts no signer/binding/cursor/data change, then proves valid transport and a real zero-change pull. It records the actual current state, not the stale pre-deletion summary.
5. Build/tag PR4 source admin as `shippin-acquisition-source:admin`, then recreate ONLY the fixture admin using the tracked separated Compose command. The original management key now has `fixture-management`/`source-management`; new export key has the unchanged importer binding `fixture`/`source-admin`. No published admin port.
6. Run `migrate`. It asserts unchanged source state and a failed old-signer pull, uses the UI for a signing-seed-only update, asserts unchanged ID/binding/cursor, then submits a fresh public consent signup and imports it into the same source. Duplicate, transport/signature, origin and reader/customer/anonymous negatives are exercised. Old deleted lead stays absent.
7. Recreate only fixture application processes with retained volumes. Run `prepare`, then `persistence`; exact imported row, source summary and binding must survive. Never reset cursors.
8. Run the private `delete` helper inside the existing source namespace with only the exact new synthetic lead ID from `separated-record.json`. It verifies the active synthetic target, rejects forged exporter management writes and admin export reads, confirms unchanged target, then uses management authority for DELETE. Run `tombstone` and inspect the exact platform database row for terminal deletion, NULL sealed payload/expiry and advanced revision.

Helper commands `exporter-files` and `renew-certs` require disposable opt-in and the owned writable secrets volume. They require existing management/TLS keys and never regenerate them. Certificate backups have content-derived nonoverwriting names; exporter files use exclusive creation. Helper errors suppress sensitive detail.

Commands above are executable prerequisites and acceptance steps, not a claim they have run. Record actual run results separately from the earlier admin-only qualification.


The preserved 2026-11-14 fixture secrets volume was created before Compose and has no labels. Only that known fixture can use `-AllowLegacyUnlabeledFixtureSecrets` with setup and migrate: both original admin and signup containers must still carry exact project/service labels and mount this exact named volume read-only at `/secrets`. Nonempty mismatched labels always reject. This opt-in does not relabel, recreate or replace the volume. Fresh fixtures should use normal Compose-owned labels.

After separated admin startup, the launcher automatically runs `/acquisition-fixture probe` inside the guarded private fixture namespace immediately before `migrate`. It requires CA-verified exporter GET200, old management-key/old-binding GET401 and management GET401; an unreachable or TLS-broken endpoint cannot count as signer-denial evidence.


If a qualification fails after a successful signer PUT, do not reset a cursor or overwrite the prior baseline. The explicitly controlled `restore` phase first proves the current exporter works via actual platform pull with unchanged data, then restores only the original synthetic signer through the UI. It is for supervised fixture recovery, not production rollback automation. Restore the preserved original admin process and run `baseline -BaselineFileName separated-baseline2.json`; then switch separated again and use `migrate -BaselineFileName separated-baseline2.json`. New baseline files are exclusive-create. Preserve the failed-attempt evidence. Login throttling is8 attempts/10 minutes: wait the normal window rather than restarting to clear it.


`resume -BaselineFileName separated-baseline2.json` is an explicit continuation after a separately recorded successful cutover. It runs the mandatory TLS/key probe, asserts the same source/binding/cursor/lead baseline and a successful zero-change exporter pull, then reruns lifecycle and continues signup/import. It does not rerun or pretend to prove old-signer rejection. Preserve the original cutover-stage evidence and all extra paused test sources. Wait for the normal login budget before resuming.


### Observed separated-fixture evidence, 2026-11-14

- Source PR4 `d47aad0` and platform working baseline `f84972f`; preserved fixture databases/volumes and historical record were reused. Old TLS certificate was backed up and renewed with the same key. Original admin image remains tagged `admin-pre-separated-20261114`.
- Actual old-endpoint baseline succeeded with UI CA-only update and preserved source binding/cursor/historical tombstone. Initial signer PUT succeeded, but strict DOM assertion exposed retained seed markup. This was not reported as a full pass. Controlled restoration proved the saved exporter could pull and restored only original signer; the same DOM assertion reproduced.
- The reviewed native uncontrolled password-field fix was built into the synthetic panel (`5c4c07e7758e…` image config). Baseline2 passed, then separated probe and real UI key-only cutover passed. Strict lifecycle passed through source switching but the first navigation assertion ran at URL change before lazy destination render. Test now waits for the actual Customers heading before requiring the retained node detached and scrubbed; no secret assertion was relaxed.
- Explicit `resume` after the normal login window independently verified the same baseline2 source/binding/cursor/lead data and a zero-change exporter pull. It passed update success, actual API400, custom invalid-seed validation, create cancellation/success, tab/source/sidebar navigation with retained-node value/defaultValue/value-attribute/markup scrubbing, and response-value/DOM/storage nonreflection. Native browser required-field validation and opaque React Query memory were not claimed tested.
- Fresh public consent signup imported into the same source; repeat import, wrong trust/signature, cross-origin and real reader/customer/anonymous denials passed. Extra paused lifecycle sources remain; they were not deleted or substituted for migration proof.
- Planned recreation changed actual panel/admin container IDs with unchanged volumes. `prepare` and `persistence` passed exact migrated source/lead/binding assertions. The private split-authority helper proved exporter/admin denials and unchanged target before management DELETE. Browser `tombstone` plus repeated pull passed. Exact-row SQL checks proved platform deleted=true, NULL sealed payload/expiry, advanced revision; source deleted=true, NULL ciphertext/email hash, advanced revision.
- Historical `fixture-record.json`, `separated-baseline.json`, `separated-baseline2.json` and new `separated-record.json` are retained privately. Successful continuation is distinct from prior failed attempts and separately observed cutover stages. No live deployment, production readiness or browser-CI claim.

Observed commands used the launcher phases `setup`, `prepare`, `baseline`, `migrate`, `restore`, `baseline -BaselineFileName separated-baseline2.json`, `resume -BaselineFileName separated-baseline2.json`, `persistence`, and `tombstone`; successful helper phases were `exporter-files`, `renew-certs`, `probe`, `delete`. `migrate` and `restore` failed as documented above. Normal10-minute password windows were respected after diagnosis; no rate-limit configuration was disabled.
