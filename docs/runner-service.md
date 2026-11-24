# Runner gateway source disposition

PR #20 is reconciled against the current platform runner contract. Its original
implementation remains readable at commit
[`6ee87f6c3a81479a765109391487fb7fcdd3432d`](https://github.com/shippinAI/shippin/tree/6ee87f6c3a81479a765109391487fb7fcdd3432d).
The merge retains that ancestry. It does not activate a second gateway or replace
the current runner page. [Build runners](runners.md) owns the current design and
[the roadmap](roadmap.md) owns capability status.

This distinction matters: retaining source history is not implementing every
capability in that source. The external service's versioned profile templates,
project grants, connection view and browser acceptance are not current platform
features merely because its branch is merged.

## Source accounting

The candidate changed 22 paths. Each family has one disposition below. Original
files, tests and the historical fixture receipts are available at the pinned
revision above; no external checkout, service or database is changed.

| Candidate paths | Disposition and current owner |
| --- | --- |
| `cmd/console/main.go`; `internal/runnerbridge/proxy.go`; `internal/runnerbridge/proxy_test.go` | Superseded wiring. Keep main's startup and the canonical MCPL surface. The old proxy registered broad `/v1/projects/` routes, forwarded the platform session to a separate gateway, and obtained authorization from that service. Its TLS, header filtering and negative tests remain historical reference; no such route or credential sharing is activated by this merge. |
| `web/src/app/router.tsx`; `web/src/template/nav-items.ts`; `web/src/lib/ui-config/text-keys.ts` | Keep current main byte-for-byte. There is one Build runners destination, using current labels and tenant permissions. Do not add a second `tenant.runners` entry or overwrite the newer route. |
| `web/src/features/runtime/runners/model.ts`; `model.test.ts`; `ui/runners-page.tsx` | Keep current main byte-for-byte. Tenant jobs and repository assignments use generated MCPL calls. The older gateway project/scope model is preserved in history rather than mixed into the current model. |
| `web/src/features/runtime/runners/profiles.ts`; `profiles.test.ts`; `profiles.test-fixture.ts`; `ui/runner-configuration.tsx` | External-service-specific profile schema and optimistic revision form remain in history. Current platform settings and staff runner sizes remain authoritative. They are not claimed to implement the old service's immutable profile revisions. The useful stale-save, read-only and uncertain-outcome requirements remain open below. |
| `web/src/features/runtime/runners/ui/runner-resource.tsx`; `render.test.ts` | External run/connection rendering and its component assertions remain in history. Current job UI and provider inventory retain their owners. No fixture rendering is presented as current live capacity. |
| `web/scripts/test-runner-integration.mjs`; `test-runner-profile.mjs`; `test-runner-render.mjs`; `deploy/tests/runner.compose.yaml` | Retain with their historical gateway implementation. These fixtures target removed gateway routes and depend on a separate runner repository, grants and seeded database. They are not current platform checks. Their old receipts do not qualify current main. |
| `web/package.json` | Keep the entire current main test command and dependency metadata. No current runner, acquisition, API, permission or panel test is removed or relaxed. Tests for the retired gateway implementation remain available with its original source. |
| `.ai/plans/runner-main-reconciliation.handoff.json` | Superseded handoff pinned to an older baseline and one package conflict. Preserve in history; current work records use ignored `.ai/data/` under governance. Its old scope and approval statements are not current authorization. |
| `docs/runner-service.md` | This disposition replaces obsolete activation instructions. The original design and reported fixture acceptance remain at the pinned source revision. |

## Requirements retained for future implementation

These are gaps, not completed work. Re-author them in the canonical owners only
after the implementation packet and checks are grounded in current source.

- **RUN-01 / CFG-01:** versioned runner-image/profile configuration, including
  pinned image and kernel provenance where the provider supports them. A saved
  template must remain distinct from a worker actually applying it.
- **MCPL-01:** any future external runner integration must use declared actions,
  typed contracts, audit and the existing permission boundary. Do not restore a
  broad startup proxy to bypass those contracts.
- **IAM-02:** project access, staff permissions and external execution grants must
  remain distinct. Staff status alone never implies access to a customer's runs.
- **CFG-01:** stale revision writes must preserve edits, refuse blind retry and
  require an explicit reload. Unknown outcomes must be reconciled before retry.
  Read-only users must be denied by the server as well as by controls.
- **RUN-01:** persisted records, connection enrollment and visible capacity are
  separate observations. Verify a real job and guest lifecycle before claiming
  an operational runner service.
- **ENG-01:** carry the historical acceptance scenarios into tests of the actual
  canonical implementation when those gaps are authored: real-session tenant
  denial, cross-origin write denial, credential stripping, persistence after
  service recreation, and stale-revision conflict handling.

## Verification boundary

Reconciliation is checked against main `ab8cee65dd7fbd7cd28a4ae0b77dac2bf5da9b5a`:
`cmd/`, `internal/`, `web/` and the current test commands remain unchanged. The
original PR source commit must be an ancestor of the merged result. Documentation
links and all exact-head CI checks must pass before merge, followed by main CI.
No external gateway or real-owner browser acceptance is claimed by this review.
