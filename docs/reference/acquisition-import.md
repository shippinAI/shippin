# External early-access acquisition

This work implements a separate early-access source integration within the existing
Customers > Sign-ups panel. It supports LAUNCH-01, CFG-01 and ENG-01; it does not
complete those requirements or the friends-and-family launch by itself.

## Ownership and consent

The public service owns consented source records. The platform owns its encrypted
import destination, reviewed source connection, cursor and administrative view.
Imported addresses are explicitly **unverified**. Importing creates no identity,
customer, account-waitlist entry, invitation or mail. The existing verified-account
waitlist remains unchanged. Visitor analytics is not part of this integration.

The source and platform use separate databases. The platform reads only the private
versioned source HTTP export, never the source database. Nothing exposes the source
admin, database, SSH or telemetry through a public signup route.

## Authority and secret boundaries

Generated MCPL staff actions use the canonical operator permission/session/origin
gate. Acquisition read and sync permissions are distinct. Configuration and signer
replacement require platform settings authority. New acquisition grants are not
silently added to ordinary staff or development accounts; the installation owner
preset retains its explicit full-authority role. Tests grant fixture access deliberately.

The configured Ed25519 seed is submitted write-only, sealed using the installation
key and never returned to the panel. Short-lived tokens are minted only in the
backend. Lead payloads are sealed and bound to their source, lead and revision.
The current installation sealer derives from the session key: replacing that key
without a reviewed migration makes stored credentials and personal data unreadable.
Do not call this rotation-safe until that migration/recovery path is qualified.

The source verifier's generic signing-key trust can authorize more than read-only
claims. An importer choosing marketing:read is NOT a source-enforced read-only
credential boundary. Before any real signer is provisioned, review a restricted
issuer or an independently keyed export-only service. Synthetic acceptance keys
are not production identity or authorization evidence.

## Network and import constraints

A reviewed HTTPS origin and explicit numeric destination are configured together
with certificate trust. No proxy environment, redirects or DNS rebinding may change
the destination. Private and explicitly selected loopback destinations are needed
for this service; metadata, unspecified and multicast destinations are refused.
Network and trust changes are security-sensitive configuration, not an automatic
network discovery mechanism.

Each manual sync fetches one bounded page. Validated records, cursor and audit
commit together, conditional on the unchanged configuration generation, cursor and
unpaused state. Updating configuration fences stale in-flight pulls. A successful
pull is distinct from merely having saved a connection.

Deletion and expiry remove sealed personal payloads. Minimal terminal suppression
records remain, preventing even a higher-revision replay from resurrecting the
same deleted identity. A legitimate new source signup has a new source ID. Reads
exclude expired payloads independently of source reachability; context-bound
startup/periodic cleanup removes them while the platform is running. No process
can promise physical purge while it is stopped.

Source v1 provides no epoch, source identity assertion or high-water mark. Empty
pages do not prove a source was restored. A known source restore must be paused
and reconciled deliberately; blind cursor reset and automatic restore detection
are not implemented or claimed. A reviewed TLS endpoint binding remains essential.

The initial lead view is a bounded latest-100 preview with total/truncation shown,
not full pagination or a completed CRM. Source-side status/deletion orchestration,
export, invitations and automatic sync are separate capabilities, not implied by
the import button.

## Qualification boundary

Use only disposable databases and synthetic identities for acceptance. The intended
journey is real browser signup, real authenticated panel configuration, signed
private export, durable encrypted import, repeated pull, restart and source deletion
followed by tombstone application. Include wrong trust, revoked/absent permission,
customer, cross-origin, stale generation and failed-commit controls.

`mcpltest.Serve` exercises action transactions but does not enforce staff permissions.
It cannot substitute for canonical API-gate or actual browser acceptance. Report
completed, skipped and blocked checks separately. No public hostname, host, actual
owner identity, provider deployment or runner execution is established by this work.

### Repeatable local checks

The dedicated acquisition CI workflow runs the real PostgreSQL transaction tests,
including audit-failure rollback, concurrent configuration fencing, terminal
suppression and expiry. These tests refuse an unnamed/non-disposable database.
The normal panel test command includes acquisition model regressions.

The separate browser fixture is `deploy/tests/acquisition.compose.yaml` and
`scripts/test-acquisition/`. It requires built source-service images from the
reviewed Shippin Run repository and a locally available Playwright installation.
It is not an unattended production installer or a cross-private-repository CI
workflow. Never point it at a real installation or use real lead records.

Initialize a new source secret volume once with the source qualification helper.
Start source signup first and wait for its scope initialization before starting
its private admin; admin intentionally refuses to create a missing scope. Keep
source database and secret volumes paired. The platform has its own database and
state volume. Bootstrap only the fixture, restart its panel to seed the explicit
development accounts, then run `prepare` to grant the acquisition permissions.
A panel restart reseeds development permissions, so repeat `prepare` before
permission assertions. `verify`, `persistence`, and `tombstone` exercise different
phases; none may be replaced by a successful health check. Preserve databases and
keys across the persistence phase. Delete only the recorded synthetic source lead
before the tombstone phase, using the bounded private helper.

Use private temporary files for generated passwords, signer material, setup codes
and test records. Do not save them in Git or raw test output. Never rerun source
key initialization to fix an expired fixture certificate: renew only that
certificate or start an entirely new disposable fixture.

### Verified isolated acceptance, 2026-11-14

The real Docker/Chromium journey passed with synthetic identities and records:
public consent signup, authenticated UI source creation in paused state, explicit
resume, signed private pull and unverified lead display. Repeated imports did not
duplicate records. Reader/customer/anonymous and cross-origin denial checks passed.
Wrong TLS hostname and wrong signer failed without advancing the cursor or changing
lead data. A real concurrent configuration update left the local draft intact and
blocked stale Save. Pause/resume used persisted configuration and its own reason,
not unsaved endpoint or signer edits.

After recreating panel, signup, private admin and proxy processes with unchanged
database/key volumes, the exact source summary and consent record survived. Deleting
the recorded synthetic source lead through its private API produced an imported
tombstone. Repeated pull did not resurrect it. The exact platform row had
`deleted=true`, a higher revision, and NULL encrypted payload/expiry.

The initial browser fixture exposed an Ed25519 certificate incompatibility. Only
the signup proxy received a separate generated P-256 test certificate; private
source TLS verification, source signing keys and encryption keys were unchanged.
No public site, real owner access, production signer provisioning, automatic source
restore recovery or runner capacity is established by these tests.
