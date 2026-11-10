# Panel variants

This repository owns the Shippin panel. There is one: `web/`, served by `cmd/console`. This ledger records source checkouts and historical UI work so no second panel is created. Compared on 2026-11-04; capability-level reconciliation is tracked in the [absorption ledger](absorption/capability-absorption.md).

| Variant | Last commit compared | What it is | Result |
| --- | --- | --- | --- |
| `shippinAI/shippin-console` `origin/main` | `e35ab81` | The source snapshot recorded by the platform import | **Already represented in this repository.** No merge is pending from this remote tip. The local source checkout is older and has untracked `deploy/bastion/` work; preserve it and review separately. |
| `shippin-console` `spark-endpoints branch`, `backup/recovery-worktree-20261030` | `f8649df`, `a51b493` | Workspace and job lifecycle hardening, with tests | **Absorbed.** 56 of 62 files are byte-identical here. The other 6 are newer here. |
| `shippin-console` `backup/cloud-console-alternatives-20261030` | `9511e38` | The same snapshot plus six changes to docs and a release script | **Superseded.** The script and docs are site-specific notes. |
| `panel/` in this repository | removed | A static API workbench, a second React app | **Absorbed.** The workbench is now **Provider panel > API explorer**. Its hypervisor list was replaced by Hosts & nodes. |
| `shippinAI/shippin-cloud` `panel/` | `49475d3` | An earlier copy of that workbench | **Superseded.** Older than the copy this repository already had. |
| `third_party/warden-ct102` in this repository | import of 2026-08-27 | Historical Proxmox operator source files | **Reference only; not a product panel or runtime.** Its 216-row source matrix has 13 `to-author` and 5 `needs-review` rows; remove the reference only after every behavior has verified disposition and provenance is retained. |
| `EonsofStupid/shippin-mesh` `warden/` | `26cce35b` | Part of a 790-file internal infrastructure repository, not a standalone Shippin panel source | **Read-only source under review.** The checkout has local changes and no root license observed. No source code is imported. Shippin Prox owns re-authored Proxmox operator capabilities; provider identifiers remain `proxmox`. |
| `EonsofStupid/wardenclyffe` | `e5c71ab` | The same panel earlier still | **Superseded.** |
| Historical `EonsofStupid/shippin-platform` | `3fd18ac0` | Earlier server-manager/customer-portal proposal | **Not a second panel source.** Its views were not line-by-line accepted into the product; any reusable behavior stays unresolved until the capability ledger closes it. |

Not this panel, so they stay separate:

| Repository | What it is |
| --- | --- |
| `shippinAI/vaultix` | Secrets product. Its own `panel/` is its own UI. |
| `shippinAI/nomen`, `nomen-react-go` | Identity provider. |
| `shippinAI/agent-hub` | Agent task board. |
| `EonsofStupid/shippin-site` | Public website. |
| `shippinAI/wc`, `netbirdio/dashboard` | A NetBird dashboard. |
| `EonsofStupid/shippincloud`, `devforge-shippin` | Forks of Apache CloudStack and Eclipse Theia. |
| `EonsofStupid/shippin-host-console` | Empty. |


There is no private or per-installation repository (OPEN-01). Scripts that one installation used, removed from `main` in `424a25a`, are re-authored as configurable panel features; the [absorption ledger](absorption/capability-absorption.md) tracks them.

## How anything comes in

This repository is public. Do not bulk-merge history or copy matching files. Re-author useful behavior under the existing Shippin Prox, control-plane, host-agent, service, network or `web/` owner after reviewing provenance, license, dependencies, security and tests. Run public-tree and secret scans. Do not delete or archive the source checkouts in this task; the user can do that after reviewing the completed ledger.
