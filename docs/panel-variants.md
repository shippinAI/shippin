# Panel variants

This repository owns the Shippin panel. There is one: `web/`, served by `cmd/console`. This ledger records every other place a panel lived, what was compared, and what became of it, so nobody has to re-derive that. Compared on 2026-11-03.

| Variant | Last commit compared | What it is | Result |
| --- | --- | --- | --- |
| `shippinAI/shippin-console` `main` | `e35ab81` | The console this panel came from | **Absorbed.** Identical to the [import](merge-from-shippin-console.md); no commits since. |
| `shippin-console` `feat/spark-endpoints-gold`, `backup/recovery-worktree-20261030` | `f8649df`, `a51b493` | Workspace and job lifecycle hardening, with tests | **Absorbed.** 56 of 62 files are byte-identical here. The other 6 are newer here. |
| `shippin-console` `backup/cloud-console-alternatives-20261030` | `9511e38` | The same snapshot plus six changes to docs and a release script | **Superseded.** The script and docs are site-specific notes. |
| `panel/` in this repository | removed | A static API workbench, a second React app | **Absorbed.** The workbench is now **Provider panel > API explorer**. Its hypervisor list was replaced by Hosts & nodes. |
| `shippinAI/shippin-cloud` `panel/` | `49475d3` | An earlier copy of that workbench | **Superseded.** Older than the copy this repository already had. |
| `third_party/warden-ct102` in this repository | import of 2026-08-27 | The Go and HTML Proxmox panel | **Kept as a reference, not served.** [The capture matrix](design/warden-capture-matrix.md) records every file: most are ported, replaced or rejected, and the rest are still to author, mainly backup copy jobs, ACME and disk management. Delete it when those are written or rejected. |
| `EonsofStupid/shippin-mesh` `warden/` | `26cce35` | The same panel earlier in its history | **Superseded.** Its last feature commit is 2026-07-28, older than the import, which has more. |
| `EonsofStupid/wardenclyffe` | `e5c71ab` | The same panel earlier still | **Superseded.** |
| `EonsofStupid/shippin-platform` | `3fd18ac0` | WardenClyffe: React views for a server manager and a customer portal | **Superseded, by date and design.** Reviewed by its list of views, not line by line. It stays public under MIT, so nothing is lost. Nothing imported. |

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
| `shippinAI/gold-homebase` | Notes and tooling for one installation. Private, and it stays that way. |

## How anything comes in

This repository is public. Bring code in as a reviewed snapshot, never by merging private history. Check its provenance, license, dependencies, security and tests, and let `install/check-public-tree.sh` and the secret scan run on it first. Add a row here.
