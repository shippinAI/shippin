---
name: shippin-infra-change-control
description: Use when proposing or carrying out a change to a server, host, deployment, infrastructure configuration, production host, backup, rollback, or git history.
---

# Change control

Follow this list before an infrastructure or configuration change. One change at a time. When a choice is open, use the usual commercial default (supported, reversible, boring) and say which default you assumed.

1. Do not change a server directly. No live edit over SSH, no hotfix on a host, and no click that skips the owning repository.
2. Put the change in the repository that owns it, then deploy from that repository. For this repo, read [source ownership](../../../../docs/source-of-truth.md) and follow the current owner. Do not keep a private copy of that map.
3. Before the change, take a backup and write the rollback. If you cannot name both, stop.
4. A production host needs Jesse's explicit words for that change. A plan, a pull request, or a general go-ahead is not those words.
5. Ground every claim in a check you run now. If you did not run it, say that. Do not guess a version, a disk state, or whether a deploy worked.
6. Never paste a secret, token, key, or connection string. Name the file path or the setting, and stop there.
7. Do not force-push, rewrite history, or archive a repository or branch without explicit OK for that action.
8. Stop after the one change. Report what you ran, what it showed, and how to roll it back.

Dated claims live in [facts.md](facts.md). Re-check any fact older than 90 days before you repeat it. How `main` moves today is in [CONTRIBUTING.md](../../../../CONTRIBUTING.md). Deploy and upgrade steps are in [docs/deploy.md](../../../../docs/deploy.md). Repository working rules are in [AGENTS.md](../../../../AGENTS.md).

This skill does not authorize a deploy. Opening a review is not a production change.
