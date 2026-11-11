# Shippin

Shippin is an open cloud panel for your own servers. Install it once, add your Incus and Proxmox hosts in the panel, and run a private cloud on them: a control plane for the people who run the infrastructure, and tenant organizations whose workspaces, projects, apps and databases run on it. Everything is set up and changed in the panel.

This repository is the whole product: the control plane, the panel, the host agent and the installers. [What Shippin is](docs/objective.md) defines it; the [roadmap](docs/roadmap.md) says what works today.

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

Open <https://localhost:8443/setup> and enter that code to create the owner. The browser warns about the certificate because it is self-signed for this machine. The panel opens on **Get started**, which walks you through the built-in services, how people reach it, adding your first Incus or Proxmox server, and making a recovery point.

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
| `cmd/shippin-hostd`, `internal/hostd/` | The host agent. |
| `agent/` | A small host-side diagnostic for Incus and Proxmox. |
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

No license has been selected yet. Public visibility does not grant reuse rights.
