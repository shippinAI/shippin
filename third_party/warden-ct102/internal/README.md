# warden/internal/

Private Go packages, importable only from within the `warden` module.
This is Go's enforced equivalent of Rust's `pub(crate)`.

See [`docs/decisions/0008-repo-foundation.md`](../../docs/decisions/0008-repo-foundation.md)
for the full foundation spec.

## Boundary rules

- One concern per package directory.
- One bounded context per package tree.
- No cross-imports between sibling subtrees of `host/kinds/` or
  `platforms/` — siblings communicate through interfaces defined one level up.
- New flat `warden/<feature>.go` files are forbidden. Every new feature
  lands in an `internal/` package.

## Package map

| Package | Purpose | Status |
|---|---|---|
| `version` | Build-time identity (`Version`, `Commit`, `BuildDate`) populated by ldflags | active (Phase 1) |
| `host` | Multi-host abstraction: `Host`, `Connector`, registry | skeleton (Phase 1) |
| `host/kinds/proxmox` | Stock Proxmox VE connector | stub (Phase 2 imports `warden/proxmox.go`) |
| `host/kinds/wardenclyffe` | The Wardenclyffe-fork connector | stub (initial alias for proxmox) |
| `host/kinds/bare` | SSH-only non-Proxmox hosts | stub |
| `client` | Tenant abstraction: `Client`, registry, policy | skeleton (Phase 1) |
| `deploy` | Deploy state machine | future (Phase 3 — moves from `warden/deploy*.go`) |
| `catalog` | Compose template catalog | future (Phase 3 — moves from `warden/catalog*.go`) |
| `auth` | PAM, sessions, JWT, RBAC | future (Phase 3 — moves from `warden/auth*.go`) |
| `memory` | Postgres + SurrealDB clients | future (Phase 3 — extends current `warden/supabase.go`) |
| `platforms` | Connectors for Authentik, Caddy, Traefik, OPNsense, Cloudflare | future (Phase 3 — moves from `warden/platform_*.go`) |
| `ui` | HTML templates + static asset registry | future (Phase 3) |

## Migration phases

This directory is being filled in across multiple commits. See the ADR
for the phase plan. The flat `warden/*.go` files coexist with the new
internal packages during the transition.
