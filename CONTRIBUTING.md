# Contributing and resuming development

This repository owns the platform: see [source ownership](docs/source-of-truth.md), the
[roadmap](docs/roadmap.md) and [AGENTS.md](AGENTS.md). No license has been selected yet; public
visibility alone does not grant reuse rights.

[Develop](docs/development.md) has the dev loop and the rules. `scripts/check.sh` runs the checks.
Only use disposable VM disks for installer tests. These checks do not establish production readiness.

For substantial work, create a local session using [governance](governance/README.md),
review complete relevant files and current sources, write a schema-valid plan with
observable success and failure checks, and record actual outcomes and unresolved
issues. `.ai/data/` is ignored by Git and must be backed up separately; capture is
explicit, not automatic. Keep credentials and installation-specific private information
out of records and public commits. Review provenance, license, dependencies and
security before adopting historical code. Configure services and tenant-specific
settings rather than hardcoding them.

## Working with the public remote

Clone `https://github.com/shippinAI/shippin.git` and run `git config core.hooksPath .githooks` once.

**main only moves to a green commit.** Commit on `main` locally, run the checks above, and push to the
queue: `git push origin main:queue/main`. The `Checks` workflow runs there, and when every job passes,
the `Gate` workflow fast-forwards `main` to that exact commit. The pre-push hook refuses a direct push to
`main`. If `main` moved meanwhile, the Gate refuses to promote: rebase onto `main` and push the queue again.

**A failure is loud.** When Checks fail on `main` or a queue branch, or the Gate cannot promote, the Gate
opens one issue labelled `ci-red` (or adds to it), which notifies the owner, and closes it when that branch
is green again. For AI agents, `.claude/settings.json` adds hooks: a red `main` or queue is stated at the
start of every prompt as the first thing to fix, and after a `git push` a background watcher follows the
run and wakes the agent with the failed jobs and their logs.

**Fix the source, not the test.** A failing test means the code is wrong until shown otherwise. The
`integrity` check fails a push that removes or changes an assertion in an existing test, deletes a test
file, or adds a skip or an only, unless a commit touching that file says why in a trailer:

    Test-Change: the endpoint now answers 404 for a missing host, which the old test called a 500

Adding tests and assertions never needs one. See `scripts/ci/test-integrity.mjs`.

Anyone without administrator rights, including automation, pushes a branch and opens a
pull request to `main`. GitHub requires the `agent`, `panel` and `governance` checks
to pass on an up-to-date branch before merging, and no human approval. The other
`Checks` jobs are not required, so confirm they passed before merging. The baseline
auto-merge workflow has not enabled itself on any pull request so far, so merge by hand.

Panel names and labels belong in configuration, not code: navigation in `ui.nav`, wording in `ui.text` (register a key in `web/src/lib/ui-config/text-keys.ts`), and providers in `ui.providers`. Owners can change them from **Provider panel → Interface**. `web/scripts/check-hardcoding.mjs` fails the web tests on an installation-specific name in the panel.

File names must stay unique ignoring case. A pair differing only by case is one file on
Windows and macOS, and a checkout there overwrites one with the other.

Do not repoint a historical checkout with unrelated history at the public repository
and push it. Review and selectively adopt historical work into the canonical checkout
after checking provenance, security and licensing. See [security reporting](SECURITY.md).
