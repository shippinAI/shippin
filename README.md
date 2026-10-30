# Shippin

Shippin is an early-stage, self-hosted infrastructure platform. This repository contains a panel prototype, a draft API contract, agent-workflow tooling and a Debian/Incus development image builder.

**Status:** The control plane, first-owner setup, remote node enrollment, shared UI configuration and production ISO are not implemented here. The panel is an API workbench, not a production management system. Incus is the first target; Proxmox, OpenTofu and Coder are future integrations.

- [Panel](panel/README.md)
- [Primary image development](install/primary-image/README.md)
- [Source boundaries](docs/source-of-truth.md)
- [Agent workflow](governance/README.md)

The current source history begins from a selected public snapshot. Older private development material is maintained separately. No open-source license has been selected yet; public visibility alone is not a license to reuse code.
