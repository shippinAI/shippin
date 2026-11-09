# Vaultix deploy bundle (hosted, multi-tenant)

Vaultix = stock Infisical (pinned by digest in `env.example`) behind the Shippin console. People sign in to the
console with Nomen (OIDC); the console provisions one Vaultix organization + workspace per person
(`internal/vaultixhost`) and is the only UI. The keyring CLI (`vaultix`) gets its own per-person machine identity.

| File | What |
|---|---|
| `compose.yaml` + `env.example` | Postgres, Redis, Vaultix api (podman-compose / docker compose); values from `.env` |
| `secrets-render.sh` | renders `secrets/` (0600) from sops (`SOPS_FILE`) or fresh random values (local) |
| `admin-init.sh` + `admin-init.ts` | first instance admin = platform operator; writes `secrets/operator.json` for the console |
| `edge-route.sh` | `secrets.<domain>` site on the existing Caddy edge (TLS there) |

Infisical Cloud import: console → Secrets → Import (a read-only machine identity on the Cloud project; one
environment per run; nothing about it is stored). Local proof: `deploy/screenshots/vaultix-e2e.mjs` against
`cmd/vaultix-demo` (`scripts/vaultix-demo/`).
