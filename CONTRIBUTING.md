# Contributing and resuming development

The public repository owns accepted platform code. Start with [source ownership](docs/source-of-truth.md),
the [roadmap](docs/roadmap.md), and [AGENTS.md](AGENTS.md). No license has been
selected yet; public visibility alone does not grant reuse rights.

- `api/openapi/`: draft API contract.
- `panel/`: static API-workbench prototype and its deployment instructions.
- `install/primary-image/`: development installer image builder.
- `governance/`, `.agents/skills/`: project work records and rules.
- `docs/`: project-level architecture, ownership and release direction.

For a clean local check, install the Node version used by `.github/workflows/checks.yml`
and run `npm ci`, `npm test`, and `npm run build` in `panel/`; run `npm ci`,
`npm test`, and `npm run check-records` in `governance/`. Follow the image builder
README to build and test its development ISO separately. Only use disposable VM
disks for installer tests. These checks do not establish production readiness.

For substantial work, create a local session using [governance](governance/README.md),
review complete relevant files and current sources, write a schema-valid plan with
observable success and failure checks, and record actual outcomes and unresolved
issues. `.ai/data/` is ignored by Git and must be backed up separately; capture is
explicit, not automatic. Keep credentials and installation-specific private information
out of records and public commits. Review provenance, license, dependencies and
security before adopting historical code. Configure services and tenant-specific
settings rather than hardcoding them.

## Working with the public remote

Clone `https://github.com/shippinAI/shippin.git` and work from a topic branch.
Push that branch and open a pull request to `main`. GitHub requires the `panel`
and `governance` checks to pass on an up-to-date branch before merging. No human
approval is required by the current branch rule. Administrators can bypass the
rule, so use the checks for ordinary changes and document any emergency bypass.
After merging, switch to `main` and run `git pull --ff-only`.

Do not repoint a historical checkout with unrelated history at the public repository
and push it. Review and selectively adopt historical work into the canonical checkout
after checking provenance, security and licensing. See [security reporting](SECURITY.md).
