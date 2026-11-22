---
name: shippin-infra-change-control
description: Use when proposing or carrying out a change to a server, host, deployment, infrastructure configuration, production host, backup, rollback, or git history.
---

# Change control

Follow this list before an infrastructure or configuration change. One change at a time. When a choice is open, use the usual commercial default (supported, reversible, boring) and say which default you assumed.

1. Do not change a server directly. No live edit over SSH, no hotfix on a host, and no click that skips the owning repository.
2. Put the change in the repository that owns it, then deploy from that repository. For this repo, read [source ownership](https://github.com/shippinAI/shippin/blob/main/docs/source-of-truth.md) and follow the current owner. Do not keep a private copy of that map.
3. Before the change, take a backup and write the rollback. If you cannot name both, stop.
4. A production host needs the owner's explicit authorization for that change. A plan or pull request alone does not authorize it. Honor authorization already given in the current task; do not ask again for the same action.
5. Ground every claim in a check you run now. If you did not run it, say that. Do not guess a version, a disk state, or whether a deploy worked.
6. Never record secrets, tokens, keys, or connection strings. Name the file path or setting. Follow the repository's narrowly scoped setup-code delivery rule during an authorized initial installation; never persist that code.
7. Do not force-push, rewrite history, or archive a repository or branch without explicit OK for that action.
8. Verify the change and report what you ran, what it showed, and how to roll it back before continuing to the next authorized change. An ordered task continues until its requested work is complete.

Dated claims live in [facts.md](https://github.com/shippinAI/shippin/blob/main/.agents/library/infra/change-control/facts.md). Re-check any fact older than 90 days before you repeat it. How `main` moves today is in [CONTRIBUTING.md](https://github.com/shippinAI/shippin/blob/main/CONTRIBUTING.md). Deploy and upgrade steps are in [docs/deploy.md](https://github.com/shippinAI/shippin/blob/main/docs/deploy.md). Repository working rules are in [AGENTS.md](https://github.com/shippinAI/shippin/blob/main/AGENTS.md).

This skill does not authorize a deploy. Opening a review is not a production change.
