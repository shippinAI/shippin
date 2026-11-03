# warden/ — Agent Context

The Go monolith — server-side management panel. Live at `warden.example.invalid`.
Runs on LXC 102 (`192.0.2.102`). Build target: Linux + CGO + real PAM.

The repo-root `AGENTS.md` is canonical for project-wide context. This
file is the rule reference for any Go authoring under `warden/`. The
operational specialist role is `docs/agent-roles/warden-go-engineer.md`.

## Read first when authoring Go

1. `REGISTRY.md` — what's real, what's pending
2. `docs/specs/07-dev-portal.md` — active spec
3. `docs/agent-roles/warden-go-engineer.md` — the operational role
4. `.claude/skills/warden-go-conventions/SKILL.md` — full convention reference

Patterns are inconsistent in this codebase (some files are legacy). When
a new file's style differs from an old one, follow the new style as
demonstrated by `warden/catalog.go`, `warden/catalog_handlers.go`, and
`warden/templates/catalog.html`.

## Constraints (non-negotiable)

### Logging

`log.Printf` from stdlib only. No zerolog, slog, or helper wrappers.

- Capital first word for failures (`"Template error: %v"`).
- Lowercase for routine state (`"started, listening on :8080"`).
- `%q` for user-supplied strings.
- `%v` for errors.
- `log.Printf("template error: %v", err)` verbatim in every template render failure.

### File layout

Split into `<feature>.go` (types + storage) and `<feature>_handlers.go`
(HTTP). Keep each file under 8 KB — larger files time out the MCP read
on Windows dev boxes.

### HTML templates

Full HTML shell (`<!DOCTYPE html>` …). Reference
`warden/templates/catalog.html`, NOT `warden/templates/proxmox.html`
(known bug: missing shell).

Template directives:
- `{{ template "head" }}` — no dot
- `{{ template "header" . }}` — with dot
- `{{ template "scripts" }}` — no dot

### Routes

Registered in `warden/main.go` after the existing block, wrapped with `requireAuth`:

```go
http.HandleFunc("/feature",      requireAuth(handleFeature))
http.HandleFunc("/api/feature",  requireAuth(handleAPIFeature))
http.HandleFunc("/api/feature/", requireAuth(handleAPIFeature)) // sub-resource
```

### Build

- `go vet ./...` clean before committing.
- Production: Linux + CGO + real PAM. Build on LXC 102, not Windows.
- Windows tests: CGO + clang via explicit non-Linux stubs. Never disable
  CGO to mask failures.

### LXC command execution

Do NOT use `POST /nodes/{node}/lxc/{vmid}/exec` — PVE 9.1.9 does not
implement it. Warden SSHes from LXC 102 to target LXC internal IPs
using `golang.org/x/crypto/ssh`. Proxmox API handles lifecycle, config,
and UPID polling only.

### Storage

- **Catalog templates:** live under `WARDEN_CATALOG_DIR` or `/etc/warden/templates/compose` in production.
- **Dashboard platform inventory:** `platform_systems.json` (see root `AGENTS.md`); operator CRUD from dashboard; merge/replace via config import; TCP checks on page render only.
- **Deployment state:** JSON files at `/etc/warden/deployments/<id>.json` for v0.1. Postgres target is `app_infra.deployments` per ADR 0007 when the persistence migration lands. Connection uses `SUPABASE_DB_URL` through the transaction pooler on port 6543; pool wrapper lives at `warden/supabase.go`.
- Supabase/Postgres is product/user truth per ADR 0007.
- SurrealDB is AI session/reasoning/RRO memory only. The active schema lives at
  `surreal/schema/001_ai_memory.surql`; Cursor hook capture lives under
  `.cursor/hooks/`.

### Dependencies

Check `go.mod` before adding any dependency. Ask before introducing new
deps. Current: `pty`, `gorilla/websocket`, `pam`, `yaml.v3`, `pgx/v5`.

## New-page checklist

- [ ] `warden/<feature>.go` — types + storage
- [ ] `warden/<feature>_handlers.go` — HTTP
- [ ] `warden/<feature>_test.go` — happy-path + not-found at minimum
- [ ] `warden/templates/<feature>.html` — full shell
- [ ] Patch `warden/main.go` — register routes
- [ ] Patch `warden/home.go` — add PanelCard tile
- [ ] Patch `warden/static/quicknav.js` — add to palette
- [ ] `go vet ./...` clean

## Key types already defined

- `CatalogTemplate`, `CatalogInventory` — `warden/catalog.go`
- `ProxmoxConfig`, `ProxmoxInventory`, `ProxmoxResource` — `warden/proxmox.go`
- `HeaderData`, `buildHeaderData(r *http.Request) HeaderData` — `warden/main.go`
