# Deploy readiness: Texas, then Platinum

Status: audit of `consolidate/all-20261030` at the commit that added this file, plus `origin/main` (`93dd341`). No deploy was run. No account was created.

**Go / no-go: NO-GO for a Texas install. NO-GO for Platinum joining it.** Platinum cannot join a control plane that is not installed.

`shippinAI/shippin-cloud` and `shippinAI/shippin-platform` did not resolve (`gh repo view`: repository not found for this token). This audit is this repo only.

## Checklist

| Item | Result | Evidence |
|---|---|---|
| This branch builds (`go build`, `go vet`, `go test`, web `npm run build`) | PASS | Run on this branch before this doc. `go test ./...` passed. Web `tsc -b && vite build` passed. Database tests skip unless `SHIPPIN_TEST_DATABASE_URL` is set; see the migration row for the run that did set it. |
| `origin/main` Go build | PASS | `git worktree` of `main` at `93dd341`, `go build ./...` exit 0. Tests and the web build were not re-run on `main`. |
| CI workflows present and passing | MISSING | No `.github/` directory and no workflow YAML in the tree. Nothing on GitHub Actions was observed because there is no workflow to run. |
| Release build from `main` only | FAIL | `deploy/release/build.sh` lines 10–13 build whatever checkout is current (`git rev-parse HEAD`) and still builds when the tree is dirty (`-dirty` in the release id). There is no `main` check. |
| Installer for the main (Texas) install | FAIL | `deploy/console-container.sh` line 13 defaults `CONSOLE_IP` to `203.0.113.50` and line 64 defaults Postgres to `203.0.113.20:5432`. `deploy/hostd-install.sh` exists as a hostd unit installer, not a site installer for Texas. No script is named or parameterized as a Texas bring-up. |
| Node-join installer (Platinum) | MISSING | No join script. Search of `deploy/` for enroll/join found no node-join installer. |
| Enroll | PASS as a token exchange, not as cluster join | `POST /v1/servers/enroll` redeems a single-use token (`internal/api/server_routes.go` lines 25–33, `internal/servers/enroll.go` lines 19–54). |
| Heartbeat | PASS | `POST /v1/servers/heartbeat` (`internal/api/server_routes.go` line 33). `Store.Beat` slides the credential expiry (`internal/servers/enroll.go` lines 57–70). The agent sends it on the console's interval (`internal/enroll/agent.go`). |
| mTLS for enroll or heartbeat | MISSING | The agent uses a plain `http.Client` and a bearer credential (`internal/enroll/agent.go` lines 42–59). No client certificate, no `RequireAndVerifyClientCert`. |
| Config has no hardcoded IPs, hosts, or tenant ids | FAIL | `config/shippin.json` line 16 `idp.dial` `203.0.113.10:443`; lines 29, 36, 42, 49 probe addresses `203.0.113.30`, `203.0.113.40`, `203.0.113.10`, `203.0.113.20:5432`; line 121 `platform_org_id` ``; line 131 `staff.project_id`. `deploy/hostd.json` lines 101–108 edge `203.0.113.10`, console `203.0.113.50:8080`, hub `198.51.100.10`. `config/rrflow-tenants.env.example` documents tenant `00000000-0000-4000-8000-000000000001` (example only, not loaded). |
| TLS verification on | FAIL | IdP HTTP client verifies against a configured CA (`internal/auth/client.go` sets `RootCAs` and `MinVersion`). Tenant DB `sslmode=require` sets `ServerName` and TLS 1.2 without `InsecureSkipVerify` (`internal/tenantdb/session.go` lines 175–177). Platform probes skip verify (`internal/api/services.go` lines 24–28). Operator host dial skips verify (`internal/api/operator_host.go` line 76). |
| Migrations on a fresh database | PASS | Postgres 16.15 on this VM. Database `shippin_throwaway`, role `shippin_throwaway`, URL `127.0.0.1` only. `SHIPPIN_TEST_DATABASE_URL` set, `go test ./internal/db/ -run 'TestMigrateConcurrentStarts|TestMigrateFailingFileLeavesNoTrace' -count=1` passed. After the concurrent-start test the schema had every embedded file once. Files `008` and `009` do not exist; names jump from `007_staff.sql` to `010_catalog.sql`. That is an unused number, not a failed migration. `035_hub_external_key.sql` applied. The second test then added three throwaway `x/*.sql` files in the same database; those are the test, not console migrations. |
| Secrets in the repo | PASS | No `.sops` blobs, no `BEGIN OPENSSH`/`BEGIN AGE`/`BEGIN PGP` blocks, no committed password assignments. Tokens are env names (`SHIPPIN_FGA_TOKEN` in `internal/config/authz.go` lines 21–26, `SHIPPIN_SMTP_PASSWORD` in `config/shippin.json` `password_env`). Vaultix compose reads a password file outside the repo (`deploy/vaultix/compose.yaml` lines 14 and 28). |
| Reconciler backoff (log spam) | FAIL | Job failures do back off (`internal/jobs/runner.go` lines 87–93, config `backoff_seconds` 10 and `backoff_max_seconds` 300 at `config/shippin.json` lines 240–241). The reconciler does not: on each tick it logs the same error (`internal/api/reconcile.go` lines 47, 55, 58, 70, 86, 98). It also does not start unless `jobs.enabled` is true (`internal/api/jobs_start.go` lines 11–17). Shipped config has `jobs.enabled` false (`config/shippin.json` line 234). Claim errors log every poll with no backoff (`internal/jobs/runner.go` lines 70–74). |
| Request log redaction | PASS | `RequestLog` logs status, route pattern, subject, and duration for 4xx/5xx only. Unmatched paths are redacted. No query, headers, or body (`internal/api/reqlog.go` lines 13–36). Wired in `cmd/console/main.go` line 49. |
| Signup rate limit | MISSING | `onboard` allows one in-process setup per subject (`internal/api/onboarding.go` lines 42–46). No IP limit, no token bucket, no shared counter. |
| Founder plan as the signup default | FAIL | `onboarding.plan` is `free` (`config/shippin.json` lines 111–113). `workspaces.default_plan` is `free` (line 57). `onboard` uses that plan (`internal/api/onboarding.go` lines 62–66). `founder` exists and is not public (`internal/db/migrations/011_catalog_seed.sql` lines 9 and the `public=false` row; `internal/hostd/plans_align_test.go` lines 110–113). |

## Ordered work before Texas

1. Add CI that runs `go build ./...`, `go vet ./...`, `go test ./...`, and `web` `npm run build` on this branch and, after merge, on `main`. Do not treat a laptop build as a passing workflow.
2. Make `deploy/release/build.sh` refuse to build unless `HEAD` is `main` (or an explicit signed tag of `main`) and the tree is clean.
3. Replace Gold IPs in `config/shippin.json`, `deploy/hostd.json`, and `deploy/console-container.sh` with names or env overrides. A Texas file must not contain `203.0.113.20` or `203.0.113.50`. Move `platform_org_id` and `staff.project_id` to env.
4. Turn probe and operator-host TLS verification on, with the site CA. Leave `InsecureSkipVerify` out of the install path.
5. Turn `jobs.enabled` on only after reconciler errors back off and do not log the same failure every tick.
6. Add a signup rate limit that survives more than one process. Decide in config whether Texas signups get `free` or `founder`; today they get `free`.
7. Write a Texas installer that takes a config file, runs migrations (they already apply cleanly on empty Postgres 16), and does not embed a host address.
8. Fresh-database proof stays: re-run `TestMigrateConcurrentStarts` against a throwaway URL in CI.

## Ordered work before Platinum joins Texas

Do these after the Texas list, not before.

1. A join installer: one command on Platinum that enrolls to the Texas console URL and writes the credential file. That command does not exist.
2. Decide enroll transport. Today it is a single-use token plus a bearer heartbeat, not mTLS. If Platinum must use mTLS, that is new work on `internal/enroll` and `internal/api/server_routes.go`.
3. Platinum's host record is a row in `servers` (`internal/db/migrations/031_servers.sql`), not a second control plane. Do not point Platinum at its own copy of `203.0.113.*`.
4. Re-run the Texas checklist against the join config. Platinum is NO-GO while Texas is NO-GO.

## What this audit did not do

No deploy, no DNS change, no image push, no Nomen user, no write to `main`.
