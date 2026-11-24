# Vaultix deploy bundle (hosted, multi-tenant)

Vaultix = stock Infisical (pinned by digest in `env.example`) behind the Shippin console. People sign in to the
console with Nomen (OIDC); the console provisions one Vaultix organization + workspace per person
(`internal/integrations/vaultixhost`) and is the only UI. The keyring CLI (`vaultix`) gets its own per-person machine identity.

| File | What |
|---|---|
| `compose.yaml` + `env.example` | Postgres, Redis, Vaultix api (podman-compose / docker compose); values from `.env` |
| `secrets-render.sh` | renders `secrets/` (0600) from sops (`SOPS_FILE`) or fresh random values (local) |
| `admin-init.sh` + `admin-init.ts` | first instance admin = platform operator; writes `secrets/operator.json` for the console |
| `edge-route.sh` | `secrets.<domain>` site on the existing Caddy edge (TLS there) |
| `proof-secret-store.mjs` | disposable local Vaultix (Docker, images already present) with a project and machine identities, for `internal/platform/secret_store/rig_test.go` |

Secret store proof against a real Vaultix, not a fake. Credentials are made per run and written only to the
env file you name (0600); keep it outside the repository.

```sh
node deploy/vaultix/proof-secret-store.mjs up vaultix:<commit> /tmp/vxproof.env   # prints the run name and port
env $(cat /tmp/vxproof.env) SHIPPIN_TEST_DATABASE_URL=... go test -p 1 -run Rig -v ./internal/platform/secret_store/
node deploy/vaultix/proof-secret-store.mjs down                                    # removes every run's containers and network
```

The env file names the app as `host.docker.internal` for a test run in a container; pass `127.0.0.1` as a third
argument to `up` for a test run on the host.

Infisical Cloud import: console → Secrets → Import (a read-only machine identity on the Cloud project; one
environment per run; nothing about it is stored).
