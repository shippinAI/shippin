# Historical source archive readiness

Status: **not ready to archive any source checkout**. This ledger is an
evidence-backed preservation and disposition record; it is not authorization to
delete, reset, move, or shut down a source tree. The `shippin-platform` checkout
on `main` is the only writable product source. Archive readiness is evaluated
per checkout after capability, rights, dependency, security, tests, dirty-work
and destination evidence are complete.

## Current checkout ledger

State observed 2026-11-05 from the local Git worktrees. Historical trees were
inspected read-only. A clean checkout or a root license alone does not close its
capability review.

| Checkout | Observed branch / revision / worktree | Rights and provenance | Absorption and archive gate | Decision |
| --- | --- | --- | --- | --- |
| `shippin-platform` | `main` `278d049`; clean and synchronized with `origin/main`; checks run `42025778200` passed all 13 jobs | Canonical product source. Repository rules and source ownership are in `AGENTS.md` and `docs/source-of-truth.md`. | Continue authoring in existing Go, migration, API, provider and `web/` owners. Keep the one panel. The full deployment, service, integration, rollout and source-disposition gates remain open. | **Keep active.** Not a historical archive candidate. |
| `shippin-cloud` | `main` `49475d3b0b57`; clean and synchronized with `origin/main`; 177 tracked files recorded in the capability inventory | Historical README names `shippin-platform` as the public product. Recheck the checkout's license and every vendored dependency/asset before adopting code. | Review design/API documents against current platform contracts, retire stale site-specific facts from candidate requirements, and disposition every capability. The React/Vite workbench is superseded by `web/`; do not create a second panel. No current runtime code has been accepted from this checkout. | **Preserve.** Archive only after per-capability dispositions and rights evidence are reviewed. |
| `shippin-console` | Local `main` `93dd3413024b`; `origin/main` `e35ab817bbfe`; local tree is 164 commits behind that remote tip and has one untracked `deploy/bastion/` directory | The platform import records `e35ab81` as the source snapshot. The local untracked directory is user data and is not part of that snapshot. The bastion change identifies itself as Gold-policy-only; AgentHub, Shippin Prox and host safety contracts must be reviewed separately. | Preserve the untracked directory. Compare the imported snapshot and any useful later remote feature by behavior, dependencies, license, authorization and tests. Re-author accepted behavior under canonical `internal/`, host-agent and `web/` ownership; do not merge this checkout wholesale. | **Preserve.** Archive only after local user data is separately retained and every useful feature is accounted for. |
| `shippin-mesh` | `main` `26cce35bd58b`; `origin/main` matches; `go.work.sum` is dirty; 790 tracked files recorded in the capability inventory | No root license was observed in the reviewed tree; licenses and provenance for subprojects, dependencies, templates and assets remain unresolved. Root instructions identify internal/site-specific infrastructure and require customer contracts to use Shippin IDs. | Inventory the Warden, Proxmox, agent/MCP, templates, registries, scripts, docs and infrastructure families by path and revision. Review dependencies and rights before reuse. Convert site values and shell automation into configurable contracts only after security and test review. No code/artifact is accepted from this checkout. | **Preserve.** Dirty `go.work.sum` is user work; archive only after full feature/rights review and its owner reviews the disposition. |
| Historical `shippin` / WardenClyffe checkout | `feat/clyffe-code-live-engine` `3fd18ac026fb`; `origin/main` is `637c5ecb04cb`; 88 dirty entries, including 62 tracked modifications/deletions and 26 untracked paths | Root `LICENSE` is MIT. That does not establish provenance or rights for all dirty contributions, bundled assets, vendored code or external tools. The dirty Proxmox backup/S3 work has a focused rejection of its current implementation in [`proxmox-capability-absorption.md`](proxmox-capability-absorption.md). | Preserve every dirty path. Review remaining Shippin API, Warden Devstation/Infisical, Proxmox, backup, storage and UI behavior independently. Do not carry Warden naming, local approval state, plaintext secrets, host-specific paths, unsigned/unpinned tools or a parallel panel into the product. Re-author only approved capabilities inside canonical bounded contexts. | **Preserve.** Do not merge, cherry-pick, reset or archive this dirty checkout. Archive only after all 88 dirty entries are retained and the capability/license/security review is complete. |
| `third_party/warden-ct102` | Reference tree inside `shippin-platform`; 188 tracked files and a 216-row capture matrix | Its source/license and every asset/dependency require the dispositions recorded in the capture matrix. It is not served by the product. | Reconcile every `to-author` row with current owner and behavior evidence; distinguish absent screenshots and stale notes from implementation. Keep it out of the shipped panel and runtime. | **Retain as reference** until its matrix is closed; then decide its disposition separately. |
| `agent-hub` | Separate checkout on `feat/devspace-bridge-contract`, with user-modified and untracked work (see capability ledger) | AgentHub owns human policy, change tickets, decisions and reviews. Shippin must consume its supported contract and use installation-configured least-privilege credentials. | Do not absorb a second policy/review engine. Keep Shippin's mapping, adapter, provenance and configuration in `shippin-platform`; preserve AgentHub work. Verify the live service principal and integration before claiming deployment decisions work in an installation. | **Keep as an external system.** Not an archive or code-import candidate. |

## Release decision gates

No historical checkout is archive-ready until all of these have evidence:

1. Every useful source capability has an exact source revision/path, current
   behavior, rights/dependency/security review and `adopt`, `re-author`,
   `reject`, or `defer` disposition linked to its canonical platform owner.
2. Adopted behavior has platform-owned configuration, persistence, API/UI,
   runtime behavior and positive/negative/recovery evidence. A similarly named
   file, screen, manifest or test is not proof of absorption.
3. Every local dirty/untracked file is retained and accounted for, or its owner
   explicitly confirms a separate backup. No historical checkout is cleaned
   as part of this task.
4. The standalone console and host-agent build, migrations, install/upgrade,
   backup/restore, secret/public-tree checks and supported disposable-provider
   workflows pass from `shippin-platform` alone.
5. The operator reviews this ledger and the remaining exceptions before making
   any archive or shutdown decision.

The detailed source rows live in [`capability-absorption.md`](capability-absorption.md)
and [`warden-capture-matrix.md`](warden-capture-matrix.md). This ledger does not
replace their per-feature evidence or authorize destructive cleanup.
