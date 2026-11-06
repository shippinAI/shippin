# Historical source archive readiness

Status: **not ready to archive any source checkout**. This ledger is an
evidence-backed preservation and disposition record; it is not authorization to
delete, reset, move, or shut down a source tree. The `shippin-platform` checkout
on `main` is the only writable product source. Archive readiness is evaluated
per checkout after capability, rights, dependency, security, tests, dirty-work
and destination evidence are complete.

## Current checkout ledger

State observed 2026-11-06 from the local Git worktrees. Historical trees were
inspected read-only. A clean checkout or a root license alone does not close its
capability review.

| Checkout | Observed branch / revision / worktree | Rights and provenance | Absorption and archive gate | Decision |
| --- | --- | --- | --- | --- |
| `shippin-platform` | `main` `06ca323`; clean and synchronized with `origin/main`; exact-head Checks run `42295590228` passed all 12 jobs | Canonical product source. Repository rules and source ownership are in `AGENTS.md` and `docs/source-of-truth.md`. | Continue authoring in existing Go, migration, API, provider and `web/` owners. Keep the one panel. The full deployment, service, integration, rollout and source-disposition gates remain open. | **Keep active.** Not a historical archive candidate. |
| `shippin-cloud` | `main` `49475d3b0b57`; clean and synchronized with `origin/main`; 182 tracked paths observed at `49475d3b0b5797c7644e20821fc3f38b46a15c54` | README names `shippin-platform` as destination and labels its deployment notes historical. No root license/NOTICE was found; nested panel is private React/Vite with tests, and source/asset rights remain unresolved. | Packet A1 family dispositions are now recorded in the capability ledger: panel workbench **reject**, OpenAPI naming/docs **adopt** in canonical `api/openapi/`, MCP/SSH orchestration **defer** pending rights/security review, and historical docs **re-author** in canonical docs owners. Keep one panel in `web/` and no runtime code imports from this checkout. | **Preserve.** Archive only after deferred families are resolved and rights evidence is complete. |
| `shippin-console` | Local `main` `93dd3413024b`; remote `main` `e35ab817bbfedc626f35aa2f42760df49d4684c2`; local tree is 164 commits behind and has untracked `deploy/bastion/` | Remote tree has no root LICENSE/NOTICE/COPYING. Go modules are pinned and web has a lockfile; dependency and bundled asset licenses still need review. Remote README/docs retain Gold-era authorization and installation state. Local untracked directory is user data and is not part of the imported snapshot. | Packet A1 family dispositions are now recorded in the capability ledger: imported control-plane/runtime families **adopt** under existing owners, Gold-era policy assumptions **reject**, site/bastion defaults **reject**, and remaining unproven host operations **re-author** only through canonical `internal/*` + `web/` owners with AgentHub. Preserve untracked local user data and do not merge this checkout wholesale. | **Preserve.** Archive only after re-authored operations have live packet 2-4 evidence and rights review is complete. |
| `shippin-mesh` | `main` `26cce35bd58b`; `origin/main` matches; `go.work.sum` is dirty; 926 tracked paths observed at `26cce35bd58bf3d7d14bf3ba1df5f684b7a9a2e1` | No root license was observed in the reviewed tree; licenses and provenance for subprojects, dependencies, templates and assets remain unresolved. Root instructions identify internal/site-specific infrastructure and require customer contracts to use Shippin IDs. | Packet A2 family dispositions are now recorded in the capability ledger: mission-control UI and MCP helper surfaces **reject**, provider observation intent **adopt** only through canonical Proxmox owners, site-coupled scripts/templates/automation **reject**, and documentation intent **re-author** in canonical docs. Rights/dependency review remains required before any deferred reuse, and no runtime code is accepted from this checkout. | **Preserve.** Dirty `go.work.sum` is user work; archive only after deferred/re-authored families are resolved with rights evidence. |
| Historical `shippin` / WardenClyffe checkout | `feat/clyffe-code-live-engine` `3fd18ac026fb`; `origin/main` is `637c5ecb04cb`; 88 dirty entries, including 62 tracked modifications/deletions and 26 untracked paths | Root `LICENSE` is MIT. That does not establish provenance or rights for all dirty contributions, bundled assets, vendored code or external tools. The dirty Proxmox backup/S3 work has a focused rejection of its current implementation in [`proxmox-capability-absorption.md`](proxmox-capability-absorption.md). | Packet A2 family dispositions are now recorded in the capability ledger: backup/S3 and host shell-control paths **reject**, Warden naming/policy model **reject**, remaining potentially useful API/provider behavior **defer** pending file-level review, and docs intent **re-author** in canonical ledgers. Preserve every dirty path and keep this checkout read-only evidence only. | **Preserve.** Do not merge, cherry-pick, reset or archive this dirty checkout. Archive only after deferred/re-authored families are resolved and all 88 dirty entries are retained. |
| `third_party/warden-ct102` | Reference tree inside `shippin-platform`; 188 tracked files and a 216-row capture matrix | Its source/license and every asset/dependency require the dispositions recorded in the capture matrix. It is not served by the product. | The current matrix has one open imported-file row (`BACKLOG.md`), 12 reviewed implementation dispositions, 28 absent screenshot rows and 188 tracked files; distinguish source dispositions from still-open product capability backlog items. Keep it out of the shipped panel and runtime. | **Retain as reference** until its matrix is closed; then decide its disposition separately. |
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
