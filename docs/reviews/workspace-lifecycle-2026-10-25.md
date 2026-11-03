# Workspace lifecycle patch review â€” 2026-10-25

This is an uncommitted local patch, not a deployment or proof of live Incus health.
Console baseline: `feat/spark-endpoints-gold`, `2f5c422ec3899277346f8e7319c32c09fe43682f`.
Contract owner: `D:\localdev\shippin-cloud\api\openapi`, baseline `b70c36cdac959d42f06cd658266deb3da3c8df71`.
The original `.gitignore`, `README.md` and `deploy/bastion/` edits remain untouched; 25 recorded hashes matched after this work.

## Critical review fixes

An independent read-only reviewer examined the patch twice; subsequent targeted review checked the fixes. Source findings were reproduced with regression tests where applicable:

- P1: each runner claim now has a unique token, preventing a stale worker in the same runner from committing under a reclaimed lease.
- P1: committed lifecycle completion receipts are replayed before the retry-budget check, including a crash after the final permitted attempt.
- P2: dispatch validates the exact recorded grant, requester, tenant, workspace scope and expiry; another preferred grant cannot substitute for it.
- P2: lifecycle acceptance and its idempotency response commit in one transaction. Failed response persistence/cleanup cannot free an accepted operation's key.
- P2: forwarding cleanup propagates failures and retains ownership metadata; retry accepts already-removed ports only after a complete readable inventory proves absence. Partial cleanup followed by retry is covered.
- P2: provisioning outcomes now require the live claim and expected provisioning state, preventing stale writes over a newer lifecycle action.
- P2: authorized lifecycle receipt replay can resolve a deleted workspace tombstone. Ordinary inventory readers still exclude deleted records; removed tenant visibility cannot replay the receipt.
- P3: corrected the customer loading text encoding.

## Implemented and reviewed

- Inventory distinguishes durable recorded state from current observation. Failed, partial, missing or malformed inventory never proves running/stopped health. Unknown provider states stay unknown.
- Customer and operator pages show observation/read failures. Failed refreshes do not retain live counts or present an empty inventory as authoritative. Customer Retry was checked in Edge using synthetic local data.
- Lifecycle acceptance atomically claims the workspace and queues a durable job. Duplicate keys replay the receipt; conflicting requests are rejected. The response includes a job and polling Location.
- Lease-fenced dispatch and completion receipts prevent stale workers from committing workspace state. Recovered restart/delete intents are not automatically repeated after uncertain dispatch.
- Completion requires provider observation. The pre-existing connection refresh after start/restart was restored during review; tests use fake clients, not real credentials.
- Deletion now propagates route cleanup errors and requires successful named inventories before declaring instance/home-volume absence. Null/malformed inventory is not success.
- The existing OpenAPI lifecycle action enum, 202 envelope, Location header, failure responses, observation fields, size and job error shapes were aligned. The focused contract test checks actual Go JSON required fields/types/enums and the POST method; it is not a full OpenAPI/schema validator.

## Test evidence

- Final Linux hostd: 95 top-level tests passed in installed Ubuntu WSL, with an empty environment and isolated network namespace. The formerly failing Windows shell/Git/ordering/path cases passed here.
- Real-Postgres starter SQL: passed separately using only a new disposable Postgres 17.10 container.
- Final Linux jobs: all 13 top-level tests passed against a newly created disposable database, including claim-specific fencing, final-attempt receipt recovery, atomic acceptance receipt persistence and stale provisioning fences.
- Final Linux API: all 114 top-level tests passed with OPENAPI_SPEC set. Full path parity, method parity, local reference resolution and actual lifecycle/operator acceptance payload checks ran without skips. The lifecycle test covers operator action enum, delete confirmation and the actual 202 receipt.
- Linux hostclient: previously verified complete-envelope, partial, null, omitted-field, nameless and duplicate-entry tests passed; this follow-up changed no hostclient code.
- New API coverage includes failed/partial/malformed reads, idempotency replay/conflict, provider failure, lease loss before/after dispatch, uncertain recovery, completion replay, and revocation of an accepted grant before dispatch (no provider call).
- Frontend existing tests, TypeScript/Vite build and budgets passed using already-installed Node 24.22 directly. Initial JS: customer 89.8 KB gzip; operator 86.5 KB; each budget 90 KB. The default npm/fnm link failed before build; no software was installed.
- Edge local fixtures: customer unknown warning, zero running count and disabled Open; failed read with Retry and no false empty list; Retry recovery. Operator unknown warning and verified counts; failed refresh clears counts and shows Retry. These are UI fixtures, not live identity/provider tests or a complete accessibility audit.
- Initial follow-up Windows DB run failed with connection refused to the isolated container port; that run establishes no DB correctness. Subsequent Linux containers on the same isolated network passed the full suites. Early test-fixture failures were repaired and the final suites rerun.
- Changed source/contract whitespace checks passed. No real account changes, credentials, remote authentication or live Incus operations were used in these checks.

## Failing or missing release gates

1. **Source provenance unresolved.** The documented founder release/fix source is still unavailable locally. Earlier local-only checks found the two PR2 commit objects but missing trees; neither is an ancestor of this baseline. PR4/founder objects were unavailable. Git lazy fetching remains disabled. Do not deploy over the founder behavior without recovering and reviewing its source.
2. **OpenAPI regression gate closed for this packet.** Documented the 53 newly undocumented paths plus two previously excepted operator workspace paths (55 paths / 61 methods added). Removed seven stale exceptions plus those two operator exceptions; no exceptions added and the original full path check is unchanged. Owner now has 96 paths / 127 methods: 81 registered, 2 registered but execution-blocked agent methods, 44 explicitly planned. Historical 53 code-side and 23 spec-side path exceptions remain; this is not a claim that every operator API is now fully specified. See the owner coverage report.
3. **Queued member revocation rule is not yet implemented.** Current code captures member authority at acceptance; grants are rechecked before dispatch. Proposed rule below requires review and implementation before release.
4. **Live acceptance remains unrun.** An explicitly disposable Incus workspace must demonstrate start/stop/restart, persisted home data, operation/job recovery, and complete route/port/firewall/instance/volume deletion. Approved target/access and source provenance are prerequisites. No Proxmox token or substrate change belongs in this packet.
5. **Integration gaps remain.** Test actual Nomen authorization changes, Vaultix/GitHub refresh failures, shared-file concurrency and provider timeouts against approved staging. Do not claim production readiness or Windows guest support from these unit/local-browser results.

## Proposed queued authorization rule

Accept only after the existing tenant/workspace permission and audit gates. Store actor, tenant, workspace, action and the exact grant reference if applicable; never persist a bearer token.
Immediately before the first dispatch, re-resolve the actor's current tenant-management authority through the existing Nomen owner and the configured manager roles, or validate the exact original approved/unexpired grant and workspace scope. Removed/disabled actors, revoked memberships, downgraded roles and revoked/expired grants must fail without a provider call. An unavailable authority source must fail closed and remain safely retryable before dispatch, within the job's attempt budget.
After dispatch, recovery may observe and finalize that accepted operation even if authority is later revoked; it must not start another lifecycle action or repeat an uncertain restart/delete. State this limitation explicitly: revocation cannot undo an already dispatched action.
Do not use the account mirror or an old session-membership snapshot as an authoritative member lookup. The current worker has no such fresh lookup wired. Implementation is pending the requested policy confirmation: this changes queued authorization semantics, and the delegated scope explicitly requires a proposal while security changes remain gated. Acceptance-time identity and the account mirror are insufficient authority evidence. No new account or ownership transfer is requested. Implement that through the existing identity boundary, with tests for removal, downgrade, disabled user, authority outage, grant expiry and revocation after dispatch. This proposal changes no deployed permissions or configuration.

## Next implementation packet

1. Recover and compare the founder fix from an authorized source; reconcile this baseline before any release commit.
2. Implement the reviewed dispatch-time authorization rule using the existing identity client boundary, with fake authority tests first.
3. Review the remaining historical operator/design contract exceptions as a separate bounded packet if full operator coverage is required. This packet resolves every new path/method failure without extending the exception list.
4. Run the existing packages and focused contract below, then the approved disposable Incus acceptance flow. Only then produce a reviewable deployment plan.

## Verification entry points

Set `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`. A Go 1.26 toolchain and cached modules were used. Run DB tests only with `SHIPPIN_TEST_DATABASE_URL` explicitly set to a newly created disposable database: some existing tests drop its public schema. Never inherit an ambient DSN.

```text
go test -mod=readonly -p 1 ./internal/jobs ./internal/api ./internal/hostclient -count=1
go test -mod=readonly ./internal/hostd -count=1
OPENAPI_SPEC=<shippin-cloud>/api/openapi/openapi.yaml go test -mod=readonly ./internal/api -run '^TestWorkspaceLifecycleContract$' -count=1
OPENAPI_SPEC=<shippin-cloud>/api/openapi/openapi.yaml go test -mod=readonly ./internal/api -run '^TestOpenAPIContract$' -count=1
npm test
npm run build
npm run budget
```

The first line skips the contract tests without OPENAPI_SPEC; it was explicitly set for the final Linux API run. The hostd suite skips starter SQL without its explicit disposable-superuser command; that test passed separately in the preceding verification. Both all-route and method contract checks now pass. Linux execution used Windows-cross-compiled test binaries in Ubuntu and existing Docker images; no toolchain/image downloads were needed. Logs are retained in the task workspace as review-final-linux-{jobs,api,hostd}.log and review-v2-web-{test,build}.log. The newly created disposable database/container/network were removed after verification.
