# Plans

Status: canonical tier list, 2026-10-22. Decision owner: the operator Hall.

This file is the source of truth for tier **names** and for the numbers below. `deploy/hostd.json`
`plans`, the console default plan in `config/shippin.json`, and the catalog seed
(`internal/db/migrations/011_catalog_seed.sql`, re-applied to existing databases by
`034_canonical_plans.sql`) are checked against the JSON block at the bottom. The check fails
if they diverge.

The reference host's live hostd is not this file. Nothing here is deployed.

## Tiers

| Id | Sold | Devspace | Identity (Nomen) | Mail | Vaultix |
|---|---|---|---|---|---|
| `free` | yes | Container on the org pool. No KVM. | Org in a shared instance | 2 mailboxes, 1 domain, 1 GiB, 50 messages/day | Shared Postgres, per-tenant section |
| `starter` | yes | Per-org VM | Org in a shared instance | 10 mailboxes, 3 domains, 10 GiB, 500/day | Shared Postgres, per-tenant section |
| `pro` | yes | Per-org VM | Dedicated virtual instance | 50 mailboxes, 20 domains, 100 GiB, 5000/day | Dedicated |
| `business` | yes | Per-org VM | Dedicated deployment | Dedicated deployment | Dedicated |
| `founder` | no (staff / owner) | Per-org VM | Dedicated deployment | Dedicated deployment | Dedicated |

Mail numbers and the identity / Vaultix placement are the operator's. They are not proposals.
Business mail has no mailbox count because the deployment is dedicated.

`founder` is not in the public catalog. New sign-ups in `config/shippin.json` use `free`
(`workspaces.default_plan` and `onboarding.plan`). The reference host's live console config is a different
file and still says founder until someone changes it. This repo does not change the reference host.

## Coverage tiers and sign-in

A new installation also starts with three coverage tiers. Like every plan they are rows the
owner renames, reprices, edits or removes in **Customers > Plans & tiers**.

| Id | Members must sign in with | Meant for |
|---|---|---|
| `foundational` | `password`: a password or anything stronger | Self-service hosting on shared capacity |
| `advanced` | `second_factor`: a passkey or an identity provider | More capacity and services |
| `managed` | `identity_provider`: the organization's identity provider | Run for the customer, with support |

Any plan can carry any of the three rules (`plans.sign_in`). The rule applies to members of
organizations on that plan, on every organization request, and is checked against how the current
session signed in. A session that is not strong enough gets 403 with the reason, and the panel shows
it in place of the organization with a sign-out button. Tightening a plan takes effect on the next
request; nobody is signed out. Staff who reach an organization through an access grant follow the
staff rules (a role and a second factor), not the plan's.

The numbers for these three tiers are not set yet: their services and limits come from the plan x
service matrix the owner fills in.

## Devspace size (proposal)

Every CPU, RAM, disk, workspace, boundary, and price figure in this section is a **proposal
for the operator to confirm**, except the rules in the table above and these two constraints:

- A plan with no `vm` block is a container. `free` is that plan. `starter` and above have a VM.
- Cowork's disk is about 25 GiB and has to sit on the org **data** volume, never on the 20 GiB
  VM root. Cowork also wants at least 8 GiB of RAM on the machine that runs it. `cowork: true`
  is only set where both are true. No desktop package is installed. Where that one install
  lives (control plane, or one copy inside the tenant VM) is still open. See
  `docs/design/devspace-vm.md`.

`instances` in hostd counts every Incus instance in the tenant project, including the tenant VM.
Catalog `workspaces` is `instances` on `free` and `instances - 1` on a VM tier. Catalog `cpu` /
`memory_gib` are the per-workspace cap (`workspace_cpu` / `workspace_memory`), not the VM size.

| | free | starter | pro | business | founder |
|---|---:|---:|---:|---:|---:|
| Org quota GiB | 40 | 80 | 160 | 360 | 450 |
| Instances (hostd) | 2 | 4 | 7 | 17 | 11 |
| Workspaces | 2 | 3 | 6 | 16 | 10 |
| Project CPU / RAM | 2 / 4 GiB | 2 / 4 GiB | 8 / 16 GiB | 16 / 32 GiB | 16 / 32 GiB |
| Workspace CPU / RAM | 2 / 4 GiB | 2 / 3 GiB | 4 / 8 GiB | 8 / 16 GiB | 4 / 8 GiB |
| Home GiB | 20 | 20 | 40 | 80 | 50 |
| VM CPU / RAM | — | 2 / 4 GiB | 4 / 16 GiB | 8 / 32 GiB | 16 / 32 GiB |
| VM root / data GiB | — | 20 / 40 | 20 / 100 | 20 / 280 | 20 / 300 |
| Cowork room | no | no | yes | yes | yes |
| Boundary VMs | 0 | 0 | 0 | 1 | 1 |
| Price, cents / month | 0 | 900 | 2900 | 9900 | 0 |

Quota is the ZFS quota on `tank/tenants/<tenant>` and the Incus project limit on pool `p-<tenant>`.
It is at least VM root + data, with headroom. Founder's 16 / 32 GiB / 300 GiB data / 450 GiB quota
is the size the reference host's live founder plan already uses. The previous copy of `deploy/hostd.json` had
founder at data 150 / quota 200.

Prices: `free` is $0. `pro` at 2900 is the number already in the catalog. `starter` at 900 and
`business` at 9900 are the old micro and team guesses. Confirm all three.

Vaultix project counts (proposal): free 1, starter 3, pro 10. Business and founder are dedicated
and have no project cap in the seed. Agent Hub seats (proposal, carried from the old seed): pro
1 seat / 3 sessions, business and founder 10 / 10. Chatmaton 25 seats / 365 days and DNS 3 zones
on business and founder only (the old team numbers, not the old enterprise numbers).

## What hostd does not enforce

- Nomen shared vs dedicated is recorded on the `identity` catalog service. This repo does not
  provision a Nomen instance.
- Mail quotas are recorded here so the catalog matches shippin-mail PR #1. SMTP enforcement is
  not in this repo.
- Paid Vaultix "dedicated" is the catalog placement. hostd still has one shared starter Postgres
  (`starter_db`) for every tenant when that feature is on. That shared cluster is the free/starter
  exception. There is no per-org Postgres for pro and above yet.
- `shippinAI/agent-hub` is one Postgres database with no tenant column. Per-org placement is in
  `docs/design/devspace-vm.md`. It is not wired up.

## Contradictions still in the tree

- The reference host's live hostd (probed 2026-10-22) has `free` and `founder` only, and founder's numbers
  match the proposal above. It does not have starter, pro, or business. Do not copy this file
  onto the reference host as part of this change.
- Migration `014_boundary_tiers.sql` still inserts `micro`, `single`, `dual`, `b1`–`b8`, and
  `custom`. `028` marks those rows, plus `team` and `enterprise`, inactive and not public.
  `tenants.plan` is plain text, so an old tenant can still be named `micro`.
- `config/shippin.json` `workspaces.sizes` (small / medium / large) are create-flow presets, not tiers.
- The inner Incus pool inside a tenant VM is named `default`. The bytes are the zvol `tvd-<tenant>`
  on pool `p-<tenant>`, dataset `tank/tenants/<tenant>`. See `docs/architecture/org-storage.md`.
- The platform image cache under `state_dir/images` is a template cache, not tenant data.
- `volume_shifted` stays false until the host's ZFS accepts `security.shifted`. Steps are in the
  storage doc.

```plans
{
  "tiers": [
    {
      "id": "free",
      "label": "Free",
      "description": "Try Shippin: container devspaces on the org pool, shared identity and Vaultix.",
      "rank": 0,
      "price_cents": 0,
      "public": true,
      "hostd": {
        "quota_gib": 40,
        "instances": 2,
        "cpu": 2,
        "memory": "4GiB",
        "root_gib": 20,
        "home_gib": 20,
        "workspace_cpu": 2,
        "workspace_memory": "4GiB",
        "boundaries": 0,
        "cowork": false
      },
      "mail": {"placement": "shared", "mailboxes": 2, "domains": 1, "quota_gib": 1, "per_day": 50},
      "vaultix": {"placement": "shared", "projects": 1},
      "identity": {"placement": "shared_instance"}
    },
    {
      "id": "starter",
      "label": "Starter",
      "description": "A per-org VM. Cowork is not included at this size.",
      "rank": 10,
      "price_cents": 900,
      "public": true,
      "hostd": {
        "quota_gib": 80,
        "instances": 4,
        "cpu": 2,
        "memory": "4GiB",
        "root_gib": 20,
        "home_gib": 20,
        "workspace_cpu": 2,
        "workspace_memory": "3GiB",
        "boundaries": 0,
        "cowork": false,
        "vm": {"cpu": 2, "memory": "4GiB", "root_gib": 20, "data_gib": 40}
      },
      "mail": {"placement": "shared", "mailboxes": 10, "domains": 3, "quota_gib": 10, "per_day": 500},
      "vaultix": {"placement": "shared", "projects": 3},
      "identity": {"placement": "shared_instance"}
    },
    {
      "id": "pro",
      "label": "Pro",
      "description": "Dedicated VM with room for one Cowork disk on the org data volume.",
      "rank": 20,
      "price_cents": 2900,
      "public": true,
      "hostd": {
        "quota_gib": 160,
        "instances": 7,
        "cpu": 8,
        "memory": "16GiB",
        "root_gib": 20,
        "home_gib": 40,
        "workspace_cpu": 4,
        "workspace_memory": "8GiB",
        "boundaries": 0,
        "cowork": true,
        "vm": {"cpu": 4, "memory": "16GiB", "root_gib": 20, "data_gib": 100}
      },
      "mail": {"placement": "shared", "mailboxes": 50, "domains": 20, "quota_gib": 100, "per_day": 5000},
      "vaultix": {"placement": "dedicated", "projects": 10},
      "identity": {"placement": "dedicated_virtual_instance"},
      "agent_hub": {"seats": 1, "concurrent_sessions": 3}
    },
    {
      "id": "business",
      "label": "Business",
      "description": "Dedicated VM, plus dedicated mail, Vaultix, and identity.",
      "rank": 30,
      "price_cents": 9900,
      "public": true,
      "hostd": {
        "quota_gib": 360,
        "instances": 17,
        "cpu": 16,
        "memory": "32GiB",
        "root_gib": 20,
        "home_gib": 80,
        "workspace_cpu": 8,
        "workspace_memory": "16GiB",
        "boundaries": 1,
        "cowork": true,
        "vm": {"cpu": 8, "memory": "32GiB", "root_gib": 20, "data_gib": 280}
      },
      "mail": {"placement": "dedicated"},
      "vaultix": {"placement": "dedicated"},
      "identity": {"placement": "dedicated_deployment"},
      "agent_hub": {"seats": 10, "concurrent_sessions": 10},
      "chatmaton": {"seats": 25, "retention_days": 365},
      "dns": {"zones": 3}
    },
    {
      "id": "founder",
      "label": "Founder",
      "description": "Internal staff and owner plan. Not sold.",
      "rank": 5,
      "price_cents": 0,
      "public": false,
      "hostd": {
        "quota_gib": 450,
        "instances": 11,
        "cpu": 16,
        "memory": "32GiB",
        "root_gib": 20,
        "home_gib": 50,
        "workspace_cpu": 4,
        "workspace_memory": "8GiB",
        "boundaries": 1,
        "cowork": true,
        "vm": {"cpu": 16, "memory": "32GiB", "root_gib": 20, "data_gib": 300}
      },
      "mail": {"placement": "dedicated"},
      "vaultix": {"placement": "dedicated"},
      "identity": {"placement": "dedicated_deployment"},
      "agent_hub": {"seats": 10, "concurrent_sessions": 10},
      "chatmaton": {"seats": 25, "retention_days": 365},
      "dns": {"zones": 3}
    }
  ]
}
```
