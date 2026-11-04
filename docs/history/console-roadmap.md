# Roadmap

Historical snapshot imported from the earlier console. Its status and workflow
instructions are not current Shippin guidance; see
[`platform-absorption-plan.md`](../design/platform-absorption-plan.md) and
[`approval.md`](../approval.md) for current state.

`main` is the only line. Every open pull request is merged. `b2-release` and `approve-p1-only` are joined into `main` as history only. Their trees were already here. Do not branch new work from them.

## Where we are

The console on `main` can:

- Sign people in with Nomen, run the operator shell, and enroll servers with a one-time token and a bearer heartbeat.
- Show an org its servers, and show guests on a server that org owns or is assigned.
- Attach a Proxmox VE API from **Hosts** (`/operator/hosts`). The token secret stays in an environment variable. TLS is verified.
- This snapshot listed create, clone, template, delete, resize, migrate, snapshot, rollback, node reads and Proxmox task actions as available. Provider mutations are currently blocked with `503 agenthub_review_unavailable` until AgentHub review is integrated; the former local factory-board flow is not supported.
- Browse a tenant database that already exists. It does not create that database.

Texas and Platinum are **no-go**. Evidence is `docs/design/deploy-readiness-texas-platinum.md`. No deploy is part of this roadmap until that checklist is pass.

## The path

Do these in order. Do not start a later step by opening a side branch off an old release.

1. **CI on `main`.** There is no `.github` workflow. Add one that runs `go build ./...`, `go vet ./...`, `go test ./...`, and `web` `npm run build`. Database tests stay gated on `SHIPPIN_TEST_DATABASE_URL`.
2. **Release only from a clean `main`.** `deploy/release/build.sh` builds whatever checkout is current, including a dirty tree. It should refuse that.
3. **Take Gold addresses out of the install path.** `config/shippin.json`, `deploy/hostd.json`, and `deploy/console-container.sh` still carry site addresses and org ids. A Texas file must get those from the environment. Turn probe and operator-host TLS verification on with the site CA.
4. **Stop the reconciler from logging the same error every tick.** Then, and only then, consider `jobs.enabled`. It ships false.
5. **Texas installer.** One config file, migrations on empty Postgres, no embedded host. Platinum does not get a join script until Texas exists. Enroll is a token exchange, not mTLS.
6. **Hypervisor phase 2.** Specified in `docs/design/proxmox-full-coverage.md` and not routed: node disks, apt read, certificate read, DNS and time, syslog, task list, firewall write, backup jobs, pools read, HA read, replication read, and a per-server rate limit. This is also where the 13 `to-author` rows in `docs/design/warden-capture-matrix.md` go (backup copy jobs, disks, packages, certs, logs). Do not copy the panel's shell scripts.
7. **Hypervisor phase 3.** SDN, Ceph, ACME, subscription, notifications, metrics, wake-on-lan, cloud-init, guest agent, consoles, move-disk, prune. Consoles need a session broker. They are not a copied terminal page.
8. **Hypervisor phase 4.** One pool per org, tenant guests filtered by that pool, PBS verify, prune, and sync. PVE users are not a second directory. Shippin roles stay in Nomen. `IAM_INFRA` maps to `op_infra` and is created in Nomen, not in this repo.
9. **Managed data services.** Design only, in `docs/design/managed-services.md`. Postgres, Dragonfly, and Redis are not provisioned by the console yet. Build them on the server registry and the infra role after Texas, not as a second control plane.

## How a change ships

Branch from `main`. Mutations that the approval policy marks `hv.lifecycle` file one hub card and return `409` with `code` `waiting_on_board` and `card_key`. Approve it, then send the same request again. The roles that may ask are `founder`, `op_infra`, `org_owner`, and `org_admin`.

An agent follows `docs/agents/hypervisor.md`. A person attaching a cluster follows `docs/design/proxmox-connector.md`.

## What not to revive

- Panel login, PAM, the env file, and the HTML templates under `third_party/warden-ct102`. The capture matrix says where each file went.
- `b2-release` and `approve-p1-only` as a place to commit. They exist so the old pull requests could close. `main` has the files.
- A deploy of this tree to Texas or Platinum before steps 1–5 pass.
