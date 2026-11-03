# Contributing and resuming development

The public repository owns accepted platform code. Start with [source ownership](docs/source-of-truth.md),
the [roadmap](docs/roadmap.md), and [AGENTS.md](AGENTS.md). No license has been
selected yet; public visibility alone does not grant reuse rights.

- `api/openapi/`: draft API contract.
- `panel/`: static API-workbench prototype and its deployment instructions.
- `agent/`: local host-side provider diagnostics, not yet an enrolled agent.
- `install/primary-image/`: development installer image builder.
- `governance/`, `.agents/skills/`: project work records and rules.
- `docs/`: project-level architecture, ownership and release direction.

For a clean local check, run `go test -race ./...` and `go vet ./...` in `agent/`.
Install the Node version used by `.github/workflows/checks.yml` and run `npm ci`, `npm test`, and `npm run build` in `panel/`; run `npm ci`,
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

Clone `https://github.com/shippinAI/shippin.git`. During baseline development,
repository administrators commit directly to `main` and push; topic branches and
pull requests are optional. Run the checks above first, because GitHub records an
administrator's push as bypassing the branch rule and nothing stops a broken commit.
The `Checks` workflow still runs on every push, and only then covers the console,
installer, public-tree and secret-scanning jobs that have no local command above.
Fix a red `main` before adding anything else.

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
