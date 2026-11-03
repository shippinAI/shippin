# Shippin

Shippin is a self-hosted infrastructure panel. One web panel for your Incus and Proxmox hosts, the workspaces and services on them, and who can reach them. This repository is all of it: the control plane, the panel, the host agent and the installer.

## Try it

On a Debian or Ubuntu machine, one command installs it and prints where to go. Running it again updates it:

```sh
curl -fsSL https://raw.githubusercontent.com/shippinAI/shippin/main/install.sh | sudo bash
```

Or with Docker, with nothing else and no configuration:

```sh
git clone https://github.com/shippinAI/shippin.git
cd shippin
docker compose up -d --build
docker compose logs panel | grep setup_code
```

Open <https://localhost:8443/setup> and enter that code to create the owner. The browser warns about the certificate because it is self-signed for this machine. Then open **Infrastructure > Hosts & nodes** and add a host.

## Run it for real

Either way, everything about your installation lives in one file you keep, never in this repository. [docs/deploy.md](docs/deploy.md) covers TLS, names, tokens and upgrades.

- **Containers:** copy [config/site.env.example](config/site.env.example) to `.env`, edit it, and run `docker compose up -d --build`.
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
| `config/` | Examples. A site's real values stay out of git. |
| `install/`, `compose.yaml`, `Dockerfile` | The ways to deploy it. |
| `third_party/warden-ct102/` | The older Proxmox panel, kept as a reference until its remaining features are ported. It is not served. See [the variants ledger](docs/panel-variants.md). |

## Documentation

- [Documentation index](docs/README.md)
- [Deploy](docs/deploy.md)
- [Develop](docs/development.md)
- [Attach a hypervisor](docs/design/proxmox-connector.md)
- [Roadmap and known gaps](docs/roadmap.md)
- [Contributing](CONTRIBUTING.md)
- [Security reporting](SECURITY.md)

## License

No license has been selected yet. Public visibility does not grant reuse rights.
