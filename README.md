# Shippin

Shippin is a self-hosted infrastructure panel. This repository is the control plane, the web UI, the host agent, and the installer.

Clone it, copy the site example, and install on a fresh Debian or Ubuntu machine:

```sh
git clone https://github.com/shippinAI/shippin.git
cd shippin
sudo install -d -m 0755 /etc/shippin
sudo cp config/site.env.example /etc/shippin/site.env
sudo chmod 600 /etc/shippin/site.env
# Edit /etc/shippin/site.env for this installation.
sudo ./install/panel/install.sh --site /etc/shippin/site.env
```

A passing install prints the `GET /healthz` body with `"status":"ok"` and `"db":true`. See [docs/deploy.md](docs/deploy.md).

On a disposable machine, `sudo ./install/panel/install.sh --example` does the same with the committed example config and a generated local database.

- [Deploy](docs/deploy.md)
- [What came from shippin-console](docs/merge-from-shippin-console.md)
- [Release objective](docs/objective.md)
- [Source boundaries](docs/source-of-truth.md)
- [API contract](api/openapi/openapi.yaml)
- [API workbench](panel/README.md) (not the installed panel)
- [Host-side Incus diagnostic](agent/README.md)
- [Contributing](CONTRIBUTING.md)

`web/` is the panel. `cmd/console` is the control plane. `cmd/shippin-hostd` is the host agent. `third_party/warden-ct102` is the Proxmox panel. `panel/` is the older API workbench. Site names, addresses, and secrets belong in `/etc/shippin/site.env`, not in git.

No open-source license has been selected yet. Public visibility is not a license to reuse this code.
