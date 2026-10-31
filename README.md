# Shippin

Shippin is an early-stage, self-hosted infrastructure platform. This repository contains a panel prototype, a draft API contract, agent-workflow tooling and a Debian/Incus development image builder.

**Status:** The control plane, first-owner setup, remote node enrollment, shared UI configuration and production ISO are not implemented here. The panel is an API workbench, not a production management system. Incus is the first target; Proxmox, OpenTofu and Coder are future integrations.

To try the current panel from a fresh checkout with Docker and Docker Compose:

```sh
git clone https://github.com/shippinAI/shippin.git
cd shippin
docker compose -f panel/docker-compose.yml up -d --build
```

Open `http://127.0.0.1:8088/console/` for the customer workbench or
`http://127.0.0.1:8089/ops/` for the operator workbench. These commands start
only the static panel; they do not install an API, identity provider, host agent or
Incus. See the panel guide for configuration, updates and stopping the deployment.

- [Release objective](docs/objective.md)
- [Release roadmap](docs/roadmap.md)
- [Contributing and resuming work](CONTRIBUTING.md)
- [Security reporting](SECURITY.md)
- [Panel](panel/README.md)
- [Primary image development](install/primary-image/README.md)
- [Secondary-node enrollment design](docs/node-enrollment.md)
- [Source boundaries](docs/source-of-truth.md)
- [Service contexts](docs/service-contexts.md)
- [Agent workflow](governance/README.md)

The current source history begins from a selected public snapshot. Older private development material is maintained separately. No open-source license has been selected yet; public visibility alone is not a license to reuse code.
