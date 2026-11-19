# Facts

Each fact is a dated claim. Re-check it when `verified:` is more than 90 days old. Do not copy these claims into `SKILL.md`.

## Direct host edits are out of policy

- claim: Infrastructure and configuration changes go into the owning repository and deploy from there. A direct edit on a server is not the change path.
- source: Owner instruction recorded with this skill on 2026-11-19. Repository working rules are in [AGENTS.md](../../../../AGENTS.md).
- verified: 2026-11-19

## Production hosts need explicit words

- claim: A change on a production host waits for Jesse's explicit words for that change. A plan or a pull request is not that approval.
- source: Owner instruction recorded with this skill on 2026-11-19.
- verified: 2026-11-19

## Main moves through the queue

- claim: On this repository, `main` advances only when the Gate fast-forwards a green `queue/main` commit. The pre-push hook refuses a direct push to `main`. People without administrator rights open a pull request.
- source: [CONTRIBUTING.md](../../../../CONTRIBUTING.md), section "Working with the public remote", read 2026-11-19.
- verified: 2026-11-19

## This repository does not hold installation values

- claim: Addresses, domains, tenants, and credentials of one installation are not committed. ENG-01 forbids them in code, defaults, migrations, and tests.
- source: [docs/objective.md](../../../../docs/objective.md) (ENG-01) and [docs/source-of-truth.md](../../../../docs/source-of-truth.md), read 2026-11-19.
- verified: 2026-11-19

## agent/ is the host diagnostic

- claim: `agent/` owns the read-only Incus and Proxmox diagnostic. The assistant skill library is `.agents/library/`.
- source: [agent/README.md](../../../../agent/README.md) and [docs/source-of-truth.md](../../../../docs/source-of-truth.md), read 2026-11-19.
- verified: 2026-11-19
