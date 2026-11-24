# Shippin

Shippin is a self-hosted control panel, built for people who are not infrastructure engineers. Install it on a server you already have, and manage from it:
- **Your hosts:** Incus and Proxmox servers side by side, with live and historical resources.
- **What runs on them:** virtual machines, services deployed with one click, and published apps.
- **Where you build:** development workspaces on your own hosts, with a terminal anyone can use and your own AI, and a sync API so a desktop app such as DevPulse can work locally against them.
- **Your team:** roles, invitations and an audit log.

Everything is set up and changed in the panel.

This repository is the panel: the control plane, the web panel, the host agent and the installers. Running a multi-tenant hosted cloud on it, with customers, plans and sign-up, is the enterprise edition, a separate module ([Editions and licensing](docs/editions.md)). [What Shippin is](docs/objective.md) defines both, and the [roadmap](docs/roadmap.md) says what works today. Much of the above is still to be built.

## Try it

On a Debian or Ubuntu machine, one command installs it and prints where to go. Running it again updates it:

```sh
curl -fsSL https://raw.githubusercontent.com/shippinAI/shippin/main/install.sh | sudo bash
```

Or with Docker and generated installation credentials:

```sh
git clone https://github.com/shippinAI/shippin.git
cd shippin
python install/docker/configure.py
docker compose up -d --build
docker compose logs panel | grep setup_code
```

Open <https://localhost:8443/setup> and enter that code to create the built-in `setup` account, which you, or an AI assistant installing for you, use to finish setting up. The panel then asks you to set your own password for it and create your own owner account. The browser warns about the certificate because it is self-signed for this machine. The panel opens on **Get started**, which walks you through the built-in services, how people reach it, adding your first Incus or Proxmox server, and making a recovery point.

## Run it for real

Your installation's values never go in this repository. The goal is that a small bootstrap file holds only database addresses and keys, and everything else is set in the panel; until the [roadmap](docs/roadmap.md)'s settings stage is done, some settings still live in that file. [docs/deploy.md](docs/deploy.md) covers TLS, names, tokens and upgrades.

- **Containers:** configure `.env`, run `python install/docker/configure.py` to generate missing credentials, then `docker compose up -d --build` for the panel, PostgreSQL, ClickHouse and Dragonfly.
- **A Debian or Ubuntu machine:** copy it to `/etc/shippin/site.env`, edit it, and run `sudo ./install/panel/install.sh --site /etc/shippin/site.env`.

The panel's top bar shows the build it is running, and tells you when a newer one has been pushed, with the exact command to take it.

## Develop it

```sh
docker compose up -d --build     # the backend
cd web && npm ci && npm run dev  # the panel, with hot reload, at http://localhost:5173
```

[docs/development.md](docs/development.md) has the backend loop, the checks to run, and how to point a fork at its own repository. Work on Linux, macOS or Windows.

## What is where

| Path | What it is |
| --- | --- |
| `web/` | **The panel.** React, served by the console. Operator pages, tenant pages and the provider panel are one app. |
| `cmd/console`, `internal/` | The control plane: the API the panel talks to. |
| `internal/hv/` | Hypervisor drivers: Proxmox and Incus. |
| `cmd/shippin-hostd`, `internal/infrastructure/hostd/` | The host agent. |
| `agent/` | A small host-side diagnostic for Incus and Proxmox. |
| `.agents/library/` | Canonical assistant skills. Nested in git, exported flat for an assistant. |
| `.agents/skills/` | Workflow skills for agents editing this repository. |
| `api/openapi/` | The published API contract. |
| `config/` | Examples. An installation's real values stay out of git. |
| `install/`, `install.sh`, `compose.yaml`, `Dockerfile` | The ways to install it. |
| `deploy/` | Images and helpers the panel builds and runs on hosts. |
| `docs/` | The product definition, roadmap, design system and reference. |
| `governance/` | Local tooling that records plans and evidence. |
| `third_party/warden-ct102/` | The older Proxmox panel, kept as a reference until its remaining features are ported. It is not served. See [the variants ledger](docs/panel-variants.md). |

## Documentation

- [What Shippin is](docs/objective.md)
- [Roadmap](docs/roadmap.md)
- [Design system](docs/design-system.md)
- [Documentation index](docs/README.md)
- [Deploy](docs/deploy.md) and [develop](docs/development.md)
- [Contributing](CONTRIBUTING.md)
- [Security reporting](SECURITY.md)

## License

[Functional Source License 1.1, Apache 2.0 future license](LICENSE.md) (FSL-1.1-ALv2): use, modify and self-host it for anything except a competing commercial service; each version becomes Apache 2.0 two years after release. The enterprise edition is a separate commercial module. See [Editions and licensing](docs/editions.md).
