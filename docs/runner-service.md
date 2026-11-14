# Runner service panel integration

Status: development integration, not deployed or owner-accepted.

The existing panel owns runner navigation. The runner repository owns delegation,
scoped authorization, control, and storage. No runner source dependency is added to
the platform: a private TLS service is the boundary.

Server configuration:
- SHIPPIN_RUN_GATEWAY_ORIGIN: exact private HTTPS origin, no path or credentials.
- SHIPPIN_RUN_GATEWAY_CA: absolute regular PEM CA file for that service.

Both unset keeps runner routes explicitly unavailable (503); partial or invalid
configuration fails startup. Never disable certificate validation. Gateway private
signing keys and actual subject/tenant/project grants are configured only in the
runner service. No installation account or authorization is inferred by this work.

The platform proxies /v1/runner/projects and /v1/projects/ using only the existing
shippin_session cookie and allowlisted action headers. TLS trust is explicit,
redirects are refused, requests and responses are bounded, errors sanitized, and
upstream cookies and private headers stripped. The runner gateway rechecks live
platform identity and project authority before issuing scoped control requests.

The panel uses live authorized project discovery, filters by selected tenant, and
shows permission-aware reads. Profile configuration uses scoped, versioned templates. Credential rotation remains unavailable until its provider-bound mTLS runtime is activated. There is no fixture account
picker and no fabricated healthy runner pool.

Qualification: focused Go tests cover real TLS upstream forwarding, credential
filtering, disabled configuration, duplicate sessions, and redirect rejection.
Runner-owned tests cover bridge to real control API and project revocation.
Neither is a substitute for the pending complete platform-session browser flow.
Remaining acceptance includes configured real grants, platform login through
project selection into actual control records, persisted state after recreation,
credential rotation, trace correlation and a verified public deployment.

## Reproducing isolated browser acceptance

`deploy/tests/runner.compose.yaml` is a disposable fixture, never an installation
upgrade. Build `shippin-panel:runner-qualification` from this branch; generate new
DB_PASSWORD and DEV_PASSWORD in a private ignored env file. Generate a gateway TLS
certificate for the isolated `gateway` hostname; expose only its public cert in
RUNNER_TEST_TRUST_DIR. The gateway private key stays in its own private mount,
owned by UID 65532, mode 0600. Run the fixture with its distinct Compose project.

`web/scripts/test-runner-integration.mjs` requires explicit isolated opt-in and a
loopback HTTPS URL. Use the locked Playwright dependency from the runner repository
scripts/browser directory. Its source header lists required environment variables.
Never print passwords, setup codes or cookies into test receipts.

Sequence:
1. Run bootstrap with the fixture panel's one-time setup code. Restart only fixture
   panel so its configured development staff/customer accounts can be seeded.
2. Run prepare. It logs in to the actual API and creates customer projects, saving
   only returned identity/tenant/project IDs to RUNNER_TEST_IDS_FILE.
3. Activate only the test customer's draft tenant in the disposable database. This
   is an explicit fixture prerequisite, NOT proof that host provisioning ran. Do
   not weaken the gateway's active-tenant check or provision a real host for a test.
4. Start the runner's dedicated Compose control fixture on the same private test
   network. Copy only its public CA and the panel public certificate to the trust
   directory. Mount its signer volume only into the isolated gateway.
5. Configure a narrow gateway grant for the returned customer IDs, runs:read and
   workers:manage, repository fixture/repository, profile fixture-profile. Staff
   and denied fixtures receive no runner grant. The gateway validates real platform
   sessions and projects on each request.
6. Seed a record with runner TestSeedPlatformAcceptance (explicit fixture opt-in),
   then run verify with RUNNER_TEST_EXPECT_RUN_ID. It performs actual password-form
   login and checks the same persisted record in API and rendered UI, plus denied
   cases. No route interception or injected sessions are used.
7. Recreate only the fixture control container, preserving its volumes, and rerun
   verify with the same run ID. Stop only the fixture stacks afterward; retain
   volumes until their evidence is reviewed. Never run down -v against installation
   projects.

A successful result demonstrates the integrated development path with fixtures.
Real-owner activation still needs reviewed subject/tenant/project grants, installed
TLS trust, deployment of the reviewed panel/gateway, and actual owner acceptance.
It does not demonstrate public DNS, GitHub job execution, a healthy runner pool,
provisioning, full configuration editing, or rotation UX.

### Observed fixture receipt (2026-11-14)

Built the actual platform image from the PR20 integration (5aa1e69, including
reviewed base checker fix) and the runner gateway/control from PR2 (d845791).
A real Chromium run passed password-form login, unassigned staff/customer denial,
wrong-project denial, project discovery, persisted run table rendering, Connections,
Configuration unavailable state, and URL state after reload. No API responses were
mocked and no session cookies were injected.

The same verification passed after forced replacement of only the fixture control
container (old ID f7740de35a25, new ID 475f8a7b3103), keeping dedicated PostgreSQL
volumes. The test run was prepared through the actual PostgresStore, not a GitHub
job. This proves persisted read-path continuity, not job execution.

Failures discovered during setup were retained as findings: gateway could not read
its generated private TLS key until correct nonroot ownership was assigned; draft
tenant correctly had no runner access until explicit fixture activation; repeated
logins hit the existing per-IP limiter. Only fixture state was corrected/restarted.
No authorization, TLS checks, limiter or assertion was weakened.

The final strengthened pass also rejected cross-origin runner cancellation (403)
and same-origin cancellation by the read-only fixture grant (401/403). A subsequent
real read verified unchanged run state and cancellation flag. Actual run-response
objects were recursively checked for prohibited credential fields. These checks
passed without changing the authorization policy.



## Versioned profile configuration

Configuration now renders the canonical runner-profile schema and only the profile
IDs discovered for profiles:read. A write grant additionally enables Save. The
seven allowed fields contain resource limits and pinned build identifiers, never
credentials. Unset optional values remain unset so canonical defaults are visible
without being mistaken for explicit user choices.

New templates use expected revision 0; updates use the loaded revision. A 409 means
another writer changed it: retain edits, block blind retry and offer explicit
Reload and discard edits. Uncertain failures likewise require reconciliation,
not automatic resubmission. Every successful save says worker application is
unreported. This UI does not configure pools or apply templates to running workers.

For isolated configuration qualification add the reader fixture in the Compose
template and run reader-prepare; existing fixture IDs are preserved. Activate only
its synthetic tenant and grant profiles:read for read-only-profile. Give the writer
fixture profiles:read/profiles:write for fixture-profile, keeping denied/admin
without grants. Prepare the reader template using runner
TestSeedReadOnlyPlatformProfile with the same isolated DSN opt-in and the reader's
actual fixture tenant/project IDs. Then run the browser harness configuration
phase; it stores a nonsecret exact-revision receipt. After replacing only control,
rerun with RUNNER_TEST_PROFILE_PERSISTENCE=1 before any further write.

Observed actual browser run: create revision 1; competing writer saved revision 2;
stale UI save returned 409 and kept edits 120 while stored value was 150; explicit
reload observed 150; subsequent UI save persisted 140 at revision 3. Unknown secret
fields returned 400 and out-of-grant writes were denied without changing stored
revision. A separate read-only identity rendered its real stored revision 1 and
could not write via UI or a direct API call. These are isolated fixture results.
Credential rotation remains explicitly unavailable and accepts no key input.

The post-replacement browser pass verified exact revision 3 and template values,
including timeout 140, before any further successful write. Reader revision 1 and
the earlier persisted run regression also passed. No worker-applied claim follows.

