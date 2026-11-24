# Builds and releases

How the panel is built, versioned and published. The owner decided on 2026-11-20:
- every green commit on `main` becomes a private dev build;
- tagged releases are public on GitHub.

The enterprise edition is built the same way in its own private repository, and is sold with a signed license key ([Editions and licensing](editions.md)).

## Channels

| Channel | Built from | Published | For |
| --- | --- | --- | --- |
| Local | The working tree, `docker build -t shippin-panel:local .` | Nowhere | The review deploy on a developer's machine |
| Edge | Every commit the Gate moves `main` to | `ghcr.io/shippinai/shippin:edge` and `:sha-<12>`, a **private** package | The owner's own installations and testing |
| Release | A tag `vX.Y.Z` on `main` | A **public** GitHub Release, and `ghcr.io/shippinai/shippin:X.Y.Z`, `:X.Y` and `:latest` | Anyone self-hosting the panel |

The first public release, and any change of a package's visibility, waits for the owner to say go.

## Versions

Versions are Semantic Versioning, `0.MINOR.PATCH` until the first stable release:
- a release takes its version from its tag;
- an edge build is `<next version>-edge.<UTC date>.<short commit>`;
- a local build is `0.0.0-dev`.

Every binary carries its version, channel, commit and build time (`internal/platform/version`, set with `-ldflags`; the Dockerfile takes `VERSION` and `CHANNEL` build arguments). The console logs them when it starts, `/v1/version` returns them to operators, and the panel's top bar shows them, so a person can always say exactly what they run. `/healthz` does not, because it needs no sign-in.

## What each release carries

- **Images.** The panel image for `linux/amd64` and `linux/arm64`.
- **Binaries.** The binaries a self-hosted installation runs outside the image, such as the host agent for a server the owner brings, for the same platforms.
- **`SHA256SUMS`.**
- **A software bill of materials.** One per image and archive, in SPDX.
- **Provenance attestations.** Signed by GitHub for every image and archive, so anyone can check where a file came from: `gh attestation verify <file> --repo shippinAI/shippin`.
- **The license, `LICENSE.md`,** and `THIRD_PARTY_NOTICES` listing every dependency's license.
- **Release notes,** from the commits since the last release.

## Checks before anything is published

All of these run in CI and fail the build:

| Check | Tool | What fails |
| --- | --- | --- |
| Go code | golangci-lint 2.15.0 (`.golangci.yml`: the standard linters plus gosec, bodyclose, errorlint, nilerr, noctx, misspell and gofmt) | A finding on a line the change touches (`--new-from-merge-base=origin/main`). The 300 or so that existed when it arrived, mostly unchecked errors, are fixed as their code is worked on. |
| Known vulnerabilities in Go and its dependencies | govulncheck 1.9.0 | A vulnerability in code the binaries actually call, the standard library included. The Go toolchain is pinned to a patch release for this reason (`toolchain` in `go.mod`, the same tag in the Dockerfile). |
| Secrets in the history | gitleaks 8.30.1, its download checked against the release's checksum | Anything that looks like a key, token or password in any commit. Findings checked by hand and found harmless are listed by fingerprint in `.gitleaksignore`. |
| The image | Trivy | A critical or high vulnerability that has a fixed version |
| Licenses | go-licenses and the web dependency licenses | A dependency whose license the FSL panel or the commercial enterprise edition cannot ship, such as GPL or AGPL, and a missing notice |
| The web code | `npm test` ([Panel framework](web-framework.md)) | Formatting, lint, unused code, boundaries and every test |

Tool versions are pinned, as GitHub Actions are pinned by commit, and each is a release at least two weeks old when it is adopted.

## Order of work

| Step | Result someone can see |
| --- | --- |
| R1. Build info | The version, channel, commit and build time in the binaries, the start-up log, `/v1/version` and the panel. |
| R2. Go checks in CI | golangci-lint, govulncheck and gitleaks run on every push (the `go-quality` and `gitleaks` jobs). Built 2026-11-20. |
| R3. Licenses | `THIRD_PARTY_NOTICES` generated and checked; disallowed licenses fail. |
| R4. Edge | Each green `main` publishes the private edge image, with its SBOM, provenance and scan. |
| R5. Releases | A tag publishes the public GitHub Release and image tags. The first one is confirmed with the owner. |
| R6. Updates | An installation sees that a newer release exists and what changed. |

The enterprise repository gets the same pipeline when it is created (roadmap S2). Its images and releases stay private, and its builds carry the license-key check.
