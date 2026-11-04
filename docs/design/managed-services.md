# Managed data services (control plane)

Status: design for review. The product catalog now persists validated deployment source metadata
for GitHub, custom HTTPS Git, and digest-pinned OCI sources (migration 043), editable in the
existing operator catalog UI. This is source configuration only: there is still no fetcher,
builder, reconciler, or managed data-service provisioning implementation.

the operator wants the console UI to create and operate Postgres, Dragonfly, Redis, and similar data
services. The control plane owns them: a registry, one reconciler, and one infra role. They are
not containers someone starts by hand.

`shippinAI/shippin-cloud` and `shippinAI/shippin-platform` did not resolve for this environment
(`gh repo view` returned "Could not resolve to a Repository"). Nothing below is claimed from
those trees. References inside this repo to `gold-homebase/SHIPPIN-CLOUD/` and `agent-hub/docs/`
are documentation URLs in the catalog seed, not code that was read.

## What exists in shippin-console

| Area | What it actually is | Not |
|---|---|---|
| Product catalog | Plans and sellable services (`vaultix`, `identity`, `agent_hub`, `chatmaton`, `devspace`, `mail`, `dns`) in `internal/db/migrations/010_catalog.sql`, `011_catalog_seed.sql`, `034_canonical_plans.sql`. Admin UI: `web/src/features/admin/ui/catalog/`. Catalog rows can now carry a validated pinned GitHub/custom Git/OCI source (`043_catalog_deployment_source.sql`). | A runtime registry of Postgres/Redis/Dragonfly instances or an executor for the saved source. |
| Managed instances | `service_instances` / `service_bindings`. Kind check allows only `rrflow-state` (`internal/db/migrations/020_services.sql`). Store: `internal/managed/store.go`. Routes only list and attach rrflow (`internal/api/managed_routes.go`). | Create/status/backup/scale/delete for data services. |
| Starter Postgres | hostd `EnsureStarterDB` runs SQL inside an existing `pg` Incus instance and stores the role password with `sops` (`internal/hostd/starter_db.go`). The address is config (`StarterDBConfig.Addr`), with `203.0.113.20:5432` as the comment's example. | A service the console creates. It assumes the cluster is already there. |
| Tenant SQL browser | `internal/tenantdb` connects to a database the tenant already has. Passwords come from a secret map. `sslmode=require` sets a verifying TLS config (`internal/tenantdb/session.go`). | Provisioning. |
| Vaultix | `deploy/vaultix/compose.yaml` is a hand-run Podman/Docker compose file: Postgres + Redis + API. Passwords are files under `VAULTIX_SECRETS_DIR`, not console-managed. | A reconciler. |
| OpenFGA | `deploy/openfga/load-ownership-model.sh` uploads a model to whatever `authz.url` is in config. `config/shippin.json` does not set `authz`. | A managed database. |
| Platform probes | Overview "Platform services" cards poll probe URLs (`web/src/features/overview/ui/services-grid.tsx`). The committed probe for Database is `tcp://203.0.113.20:5432` (`config/shippin.json`). | Control actions. |
| Dragonfly | No matches in this repo. | — |
| Redis | Only the Vaultix compose service above. | A console kind. |
| Servers | CP-P1 registry, single-use enroll token, bearer heartbeat (`internal/api/server_routes.go`, `internal/enroll`). | An mTLS host agent for data services. |
| Approval | `approval` in config plus the hub board (`docs/approval.md`). | Wired to data-service delete. |
| Reconciler | `API.runReconciler` re-queues dead workspace/tenant jobs when `jobs.enabled` is true (`internal/api/jobs_start.go`, `internal/api/reconcile.go`). Shipped config has `jobs.enabled` false. | A data-service loop. |

## Data model

Extend the registry. Do not overload the rrflow-only check.

New migration (number after `035_hub_external_key.sql`):

- Widen `service_instances.kind` to `postgres`, `dragonfly`, `redis`, and keep `rrflow-state`.
- Add `owner` `control-plane` or `tenant`. Control-plane rows have `tenant_id` null and are visible only to the infra role. Tenant rows stay scoped to one tenant.
- `spec jsonb` desired: engine version, replicas, memory, disk, listen (no address compiled in), backup schedule.
- `status jsonb` observed: phase (`pending`, `progressing`, `ready`, `degraded`, `deleting`), generation, endpoint host/port, `secret_ref`, last error, `next_reconcile_at`.
- `service_backups`: id, instance id, phase, `storage_ref` (Vaultix/sops ref or object URL, never bytes), started/finished.
- `external_key` text unique per instance, used as the hub approval key.

`endpoint` and `secret_ref` hold references only. A password, URI with a password, or sops plaintext is a bug.

Adopted rows (the migration of Gold's current machines) set `status.adopted = true` and `spec.import_only = true` until a human clears import-only. The reconciler must not recreate or delete an import-only instance.

## API

All routes require the infra role from config (`roles.infra`, one name, not a new hardcoded role). Tenant members do not get these routes.

| Method | Path | Effect |
|---|---|---|
| GET | `/v1/infra/services` | List control-plane instances and status. |
| POST | `/v1/infra/services` | Create. Body is the spec. If the matching approval rule has `needs_approval`, the call posts a hub card and stays `pending` until that card is `approved`. |
| GET | `/v1/infra/services/{id}` | Spec, status, backup list, `secret_ref`. |
| POST | `/v1/infra/services/{id}/restart` | Desired restart generation. |
| POST | `/v1/infra/services/{id}/scale` | Desired replicas / size. |
| POST | `/v1/infra/services/{id}/backup` | Desired backup. |
| DELETE | `/v1/infra/services/{id}` | Always requires an approval card (`managed.delete`). The row stays until the card is `approved` and the agent reports the instance gone. |

Credentials response is `{ "secret_ref": "vaultix://... or sops://..." }`. No secret value.

Approval actions, added to `config/shippin.json` `approval.rules` when this is built: `managed.create`, `managed.scale`, `managed.restart`, `managed.backup`, `managed.delete`. Delete is `needs_approval: true`. The board driver stays the one in `docs/approval.md` (`hub` or `http`). The card key is `managed.delete:<instance id>:<generation>`.

## Reconciler

One loop, same leader lock idea as `reconcileLock` in `internal/api/reconcile.go`. It runs only for rows whose `next_reconcile_at` is due.

For each due row:

1. Read desired spec and last observed status from the host agent.
2. If they match, set phase `ready` and the next look far out (minutes, from config).
3. If they differ, call one agent action. On failure set phase `degraded`, store the error, and set `next_reconcile_at = now + backoff`.
4. Backoff doubles from `managed.backoff_seconds` to `managed.backoff_max_seconds`, same shape as `internal/jobs/runner.go` `backoff`.

Logging: one line when the phase changes, and one line when an error string changes. A repeated failure on the same generation must not log every pass. The current reconciler logs every tick (`reconcile: workspaces: %v` in `internal/api/reconcile.go`); this loop must not copy that.

The job runner stays for one-shot work (a backup job). The reconciler is the only thing that converges spec to status.

## Host agent actions

hostd (or a small sibling next to it) gains a local API the console calls over the existing hostd URL. No new IP in code. Actions:

| Action | Behavior |
|---|---|
| `ensure` | Create the Incus instance from the spec if missing. Idempotent. |
| `status` | Phase, endpoint, version. No secrets. |
| `restart` | Restart the instance. |
| `scale` | Apply replica/size. Refuse to shrink disk. |
| `backup` | Run the engine's dump into the configured store. Return `storage_ref`. |
| `delete` | Delete only when the console presents an approved generation. |

The agent writes a new password into Vaultix or the sops file named by `spec.secret_store` and returns the ref. It does not return the password to the console.

Dragonfly and Redis share one "key-value" driver with an engine field (`dragonfly` or `redis`) so the registry does not grow a third copy of the same lifecycle. Postgres stays its own driver because backups and roles differ.

## UI

Under the operator app, not the tenant overview probe grid:

- `/admin/infra` list: name, kind, phase, endpoint host (no secret), age.
- `/admin/infra/new` spec form. Submit shows "waiting on the factory board" when the rule requires it.
- `/admin/infra/:id` status, scale, restart, backups, and a credential panel that shows only `secret_ref` plus a link to Vaultix.
- Delete is a confirm dialog that states the approval card id and stays disabled until `board_status` is `approved`.

The existing probe cards stay. They are health checks, not this registry.

## RBAC

One infra role, configured:

```json
"roles": { "infra": "op_infra" }
```

`op_superadmin` is not implied. Map it from Nomen in `roles.iam` the same way other operator roles are mapped (`internal/config/roles.go`). The API checks that role and nothing else. Approval policy still decides which of those people must wait for a board card.

## Secrets

Allowed: Vaultix path (`vaultix://org/project/env/key`) or sops key path (`sops://file#/key`). The console stores that string.

Not allowed: secret values in Postgres, logs, audit detail, job payloads, or API responses. reqlog already avoids bodies (`internal/api/reqlog.go`); these routes must not put a URI with a password into an error string.

Starter-db's current pattern (generate a password, `sops set`, never return it) is the behavior to keep. The ref replaces the raw sops key once Vaultix is the store.

## Moving what Gold runs by hand

Record, do not recreate, in the first migration of live state:

| Today's hand-run thing | Adopted row |
|---|---|
| Postgres container whose probe is `203.0.113.20:5432` (`config/shippin.json` probe `Database`, instance `pg`) | `kind=postgres`, `import_only=true`, endpoint from config at adopt time, not from a literal in Go. `secret_ref` points at the existing sops key hostd already uses. |
| OpenFGA's database | `kind=postgres`, name `openfga`, `import_only=true`, parent is the adopted platform Postgres. `deploy/openfga/load-ownership-model.sh` keeps loading models; it does not start the database. |
| Vaultix Postgres and Redis (`deploy/vaultix/compose.yaml`) | Two rows, `postgres` and `redis`, `import_only=true`. Compose remains until the agent reports the same endpoint healthy. Then compose is stopped by a reviewed change, not by the first reconcile. |

Adopt is an operator API `POST /v1/infra/services:adopt` with the observed endpoint and secret ref. The reconciler's first passes are status-only while `import_only` is set. Clearing `import_only` is itself an approved action.

## Backups (warden panel, 2026-08-27)

The older panel (`third_party/warden-ct102`) grew a backups subsystem that is not in git history: S3, SMB, Infisical, a local secrets file, an estate preflight, a coverage checklist, and an event log. That work belongs here, not in a second panel.

Two different backups must stay separate:

| Kind | Owner | What it copies |
|---|---|---|
| Hypervisor snapshot | `docs/design/hypervisor-drivers.md` | One guest, taken through the Proxmox or Incus API, then read back. |
| Estate / file backup | this section | Paths and managed-service dumps to S3, SMB, or a Vaultix/sops ref. |

Estate destination rows store a kind (`s3`, `smb`, `infisical`), endpoint, bucket or share, and a `secret_ref`. They do not store access keys. The panel's `backups-secrets.json` (raw keys on disk) is not ported.

Coverage is a function of a configured catalog plus observations (path present, job listed, last status). The panel's catalog hardcodes the original site paths (`/var/lib/shippin/warden-src`, `//192.0.2.117/warden-storage`, `/var/lib/shippin/operator`). Those paths are evidence in the import, not console defaults. A deployment supplies its own catalog.

Events are an append-only ring on the backup row. A repeated failure updates `next_reconcile_at` and does not log the same line on every pass.

S3, SMB, and Infisical transfer executors from the panel shell out or keep long-lived sessions. They are specified here and not wired in the driver PR. The pure checks (destination shape, coverage summary, event ring) are the part that can land with tests.

## Explicitly not implemented yet

The source contract does not fetch, build, inspect, or deploy artifacts. It does not provision
Postgres, Dragonfly, Redis, or ClickHouse; create an instance registry; reconcile desired and
observed state; resolve Vaultix/sops references; or add host-agent actions. The hypervisor driver
library is a separate concern and does not provision these services.
