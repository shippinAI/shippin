# Develop

There is one panel (`web/`) and one control plane (`cmd/console`, `internal/`). You can work on either without the other.

You need Docker, and for the parts you change: Node.js 24 for the panel, Go 1.26 for the control plane.

## The panel

Run the backend in containers and the panel with hot reload:

```sh
docker compose up -d --build
docker compose logs panel | grep setup_code
cd web && npm ci && npm run dev
```

Open <http://localhost:5173/setup> and enter the code. The dev server proxies `/v1` and `/auth` to `https://localhost:8443`. Set `SHIPPIN_DEV_API` to point it somewhere else.

Chrome, Edge and Firefox keep the panel's `Secure` session cookie on `http://localhost`. Safari does not, so use one of those for this loop.

## The control plane

`scripts/dev-server.sh` starts the database in a container, makes a session key and a certificate under `.run/` (ignored by git), prints a setup code, and runs the console from source at <https://localhost:8443>:

```sh
scripts/dev-server.sh
```

If 8443 is taken, set `SHIPPIN_LISTEN=:18444` for the server and `SHIPPIN_DEV_API=https://localhost:18444` for the panel.

Use it with `cd web && npm run dev` to change both sides at once. It runs in Git Bash on Windows. The AI launcher needs a Unix pseudo-terminal, so that one feature reports it is unavailable on Windows.

## Check your work

`scripts/check.sh` runs what CI runs that does not need a dedicated machine. It
resolves either `go` or `go.exe`, so the same command works from Unix shells and
Git Bash on Windows:

```sh
scripts/check.sh
```

Before the first release CI stays fast and leaves these to you, when you change that code. Run `go test -p 1 ./...` with `SHIPPIN_TEST_DATABASE_URL` pointing at a disposable Postgres for the database tests, and set `SHIPPIN_TEST_CLICKHOUSE_URL` for the ClickHouse ones. Run the Debian installer only on a disposable machine.

## Review it

There is one review deploy, and each round of work overwrites it. For signing in as different kinds of user, list development accounts in its `.env`:

```sh
SHIPPIN_DEV_ACCOUNTS=admin@example.test=staff,user@example.test=customer
SHIPPIN_DEV_ACCOUNTS_PASSWORD=<at least 12 characters>
```

The panel keeps them in step on every start and logs a warning while they exist. A staff account gets every staff role except Owner, the per-person catalog and customer permissions as direct grants (shown under Team > Direct grants), and no second-factor requirement. A customer account owns a test organization and has no staff access; `user@example.test=customer:advanced` puts that organization on a chosen plan. The owner's account is never touched. Never set these on a real installation.

Then overwrite it:

```sh
git pull --ff-only
python install/docker/configure.py
docker compose up -d --build
```

## Rules that are easy to miss

- **Nothing about one installation goes in code.** Addresses, domains, names and tokens come from configuration. `install/check-public-tree.sh` and `web/scripts/check-hardcoding.mjs` fail CI when they find some.
- **Panel names and labels are configuration too.** Navigation is `ui.nav`, wording is `ui.text` (register a key in `web/src/lib/ui-config/text-keys.ts`), and providers are `ui.providers`. Owners can change them under **Provider panel > Interface**.
- **A provider's name does not appear in `web/src/features/infrastructure/`.** The panel learns providers, fields and actions from `GET /v1/infra/overview`. Add a driver by adding it to `internal/hv/catalog.go`.
- **File names stay unique ignoring case.** A pair differing only by case is one file on Windows and macOS.

## Add a hypervisor driver

Implement `hv.Driver` in `internal/hv/<kind>`, declare what it needs and offers in `internal/hv/catalog.go`, open it in `openDriver` in `internal/api/infra.go`, and add its names to `ui.providers` in `config/shippin.example.json`. The tabs, add form and actions follow from that with no panel change.

## Fork it

Set `updates.repository` and `updates.branch` in your config, or `SHIPPIN_UPDATE_REPOSITORY` and `SHIPPIN_UPDATE_BRANCH` in your environment, so the update notice follows your repository. Set `ui.text` and `brand` to your own wording. Nothing in the code needs to change.

## Migrations and the audit log

On every deployment the panel's database role does not own `audit_log` or `access_grants`, so a migration that alters them fails. Change them in `internal/db/audit_owner.sql` or `audit_chain.sql`, which run as a superuser.
