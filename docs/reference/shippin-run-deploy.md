# Evaluate the legacy runner prototype

These instructions run the isolated PR #18 prototype, not the Shippin panel.
Read [its boundary](shippin-run.md#platform-boundary) first; platform deployments
use [Deploy](../deploy.md). These helpers have not been verified on a live host.

Two pieces, deployed separately:

- the control plane, `shippin-run serve`, on a Linux machine
- the site in [site/shippin-run](../../site/shippin-run), on Cloudflare Pages

Guest creation is not part of either deploy. After the control plane is up, an
operator who already has a runner machine adds it to `ci_runners.json`. Proxmox
health probes run when that file has a `vmid` and the Proxmox variables are set.

## Prototype process: operator-selected Linux host

Select a Linux host or VM and configure its hostname, storage and network.
The optional Hyper-V helper creates an empty VM from an ISO you supply.
No installation-specific hostname, address or hypervisor is assumed.

### Create the VM

On your Windows Hyper-V host, in elevated PowerShell, from a checkout:

```powershell
cd install\shippin-run\windows
.\New-ShippinRunVM.ps1 -VMName shippin-run-cp -IsoPath C:\ISOs\ubuntu-24.04-live-server-amd64.iso -SwitchName "Default Switch" -MemoryGB 4 -VCPU 2
```

`-IsoPath` and `-SwitchName` are required. The script does not download an ISO
and it does not choose an address. Finish the Ubuntu install in the console
Hyper-V opens. Give the VM a hostname and an address from your own network, and
install OpenSSH if you want a later SSH session.

### Install the process

Inside the Ubuntu VM, as a user who can use sudo, from a checkout:

```sh
sudo bash install/shippin-run/deploy-control-plane.sh
sudoedit /etc/shippin-run/env
sudoedit /etc/shippin-run/shippin-run.json
sudo systemctl enable --now shippin-run
```

The script builds `shippin-run` if `go` is on `PATH`, installs it to
`/usr/local/bin/shippin-run`, creates a system user, and writes a systemd unit.
It copies the example environment and JSON only when those files are absent.
Fill `SHIPPIN_RUN_API_TOKEN` and, when you have a GitHub App, the app id, the
private key path, the installation id, and the webhook secret. Put the private
key in `/etc/shippin-run` and give group `shippin-run` read access. See
[configuration](shippin-run.md).

Check:

```sh
curl -fsS http://127.0.0.1:8080/healthz
systemctl status shippin-run --no-pager
```

`healthz` returns `{"status":"ok"}`. Any other route without the bearer token
returns 401. The webhook returns 503 until the secret is set, and 401 for a bad
signature after that.

Point GitHub's webhook at `https://<your control-plane host>/v1/github/webhook`
for the `installation` and `workflow_job` events. Put TLS in front of the
process yourself. This unit listens in cleartext on the address in
`SHIPPIN_RUN_LISTEN`.

`shippin-run install-url` prints the GitHub App install link when
`SHIPPIN_RUN_GITHUB_APP_SLUG` is set.

## Site: Cloudflare Pages

Choose a zone and Pages project in your account. This repository does not establish
that either exists or create DNS records. Set `SITE_URL` to the HTTPS origin you own
when building; without it, the static preview omits canonical URLs.

1. In the Cloudflare dashboard, open Workers & Pages and create a Pages project connected to this Git repository.
2. Set the production branch to `main` after this work is merged. Until then, a preview deployment of the pull-request branch is enough to test.
3. Root directory: `site/shippin-run`.
4. Build command: `npm ci && npm run build`.
5. Build output directory: `dist`.
6. Node.js version: 24. Environment variable `NODE_VERSION` = `24` if the project does not inherit it.
7. Save and let the first build finish. The project URL is `<project>.pages.dev`.
8. Open the project, Custom domains, and add the apex domain and `www`.
9. Because the zone is on the same Cloudflare account, accept the DNS records Pages offers. They are proxied CNAME records to `<project>.pages.dev`. Cloudflare flattens the apex CNAME, so the apex does not need an A record.
10. Wait until both custom domains show as active. Then resolve the apex from outside Cloudflare and confirm it answers the Pages site.

Do not point the site hostname at the control plane. The site is static. MCP and
webhooks belong on the control-plane hostname you chose in the VM's DNS, which
is a separate record you add by hand if you want one.

## What you can and cannot do after this

You can call the API, connect a repository in the fleet file, preview a
`runs-on` change, store a redacted log, and read health for a Proxmox guest
that is already listed. You cannot ask this deploy to create the guest, to
charge a card, or to attach a Docker layer cache.
