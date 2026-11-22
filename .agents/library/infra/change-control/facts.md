# Facts

Each fact is a dated claim. Re-check it when `verified:` is more than 90 days old. Do not copy these claims into `SKILL.md`.

## Direct host edits are out of policy

- claim: Infrastructure and configuration changes go into the owning repository and deploy from there. A direct edit on a server is not the change path.
- source: Owner instruction recorded with this skill on 2026-11-19. Repository working rules are in [AGENTS.md](../../../../AGENTS.md).
- verified: 2026-11-19

## Production hosts need explicit words

- claim: A production host change requires the owner's authorization for that action. Existing authorization in the task remains valid; a plan or pull request alone does not authorize production changes. Authorized initial installation also has a narrow setup-code delivery rule.
- source: Current owner task authorization and [AGENTS.md](../../../../AGENTS.md), read 2026-11-22. No production deployment is part of the PR consolidation task.
- verified: 2026-11-22

## Main moves through the queue

- claim: Main must remain green. CONTRIBUTING describes queue promotion and checked PR merges; AGENTS gives the working rule and the owner's current task selects the route. This task explicitly requests sequential existing-PR merges. A prior authorization does not need to be requested again.
- source: [CONTRIBUTING.md](../../../../CONTRIBUTING.md), section "Working with the public remote", [AGENTS.md](../../../../AGENTS.md), and the current owner request, reviewed 2026-11-22.
- verified: 2026-11-22

## This repository does not hold installation values

- claim: Addresses, domains, tenants, and credentials of one installation are not committed. ENG-01 forbids them in code, defaults, migrations, and tests.
- source: [docs/objective.md](../../../../docs/objective.md) (ENG-01) and [docs/source-of-truth.md](../../../../docs/source-of-truth.md), read 2026-11-19.
- verified: 2026-11-19

## agent/ is the host diagnostic

- claim: `agent/` owns the read-only Incus and Proxmox diagnostic. The assistant skill library is `.agents/library/`.
- source: [agent/README.md](../../../../agent/README.md) and [docs/source-of-truth.md](../../../../docs/source-of-truth.md), read 2026-11-19.
- verified: 2026-11-19
