# Deploy the panel

Two ways, with the same settings. Containers are the quickest; the installer gives you a plain Debian or Ubuntu service.

## With one command

```sh
curl -fsSL https://raw.githubusercontent.com/shippinAI/shippin/main/install.sh | sudo bash
```

This works on Debian and Ubuntu. It writes `/etc/shippin/site.env` the first time, installs the panel on port 8443 with a self-signed certificate for this machine's address, and prints the address and a setup code. Use `--host` for a name or an address other than the first one it finds, and `--port` for another port. Running it again updates the panel and keeps its settings.

## With containers

```sh
git clone https://github.com/shippinAI/shippin && cd shippin
cp config/site.env.example .env   # optional; edit it for a real site
docker compose up -d --build
docker compose logs panel | grep setup_code
```

With no `.env` it serves `https://localhost:8443` with a generated certificate and a database on a private network. For a real site, set at least `SHIPPIN_CONSOLE_HOST`, `SHIPPIN_DB_PASSWORD` and `SHIPPIN_BIND` in `.env`, and either `SHIPPIN_TLS_CERT` and `SHIPPIN_TLS_KEY` or `SHIPPIN_TLS=proxy` behind your own TLS proxy. State lives in the `db` and `state` volumes, so back those up.

To upgrade, run `git pull --ff-only && docker compose up -d --build`. The panel's top bar shows when a newer build exists, and this command.

Add each Incus or Proxmox connection under **Infrastructure → Hosts & nodes**. The form is provider-configured, tests the connection before saving, and seals credentials in the platform database. If an installation manages provider secrets outside the panel, the optional **Token environment variable** field can reference a `SHIPPIN_HV_*` value in `/etc/shippin/console.env`; mount any CA or client certificate files named by the connection. No global Proxmox endpoint or Warden configuration file is used.

## On a Debian or Ubuntu machine

One command installs the panel on a fresh Debian or Ubuntu machine (Debian 13 is tested). It installs Postgres, fetches the Go and Node.js toolchains the build needs when the machine does not already have new enough ones, builds the control plane and the web UI, writes a root-only env file, starts the process, checks `GET /healthz`, and prints the one-time setup code for the first owner.

```sh
sudo apt-get install -y git
git clone https://github.com/shippinAI/shippin && cd shippin
sudo install -d -m 0700 /etc/shippin
sudo install -m 0600 config/site.env.example /etc/shippin/site.env   # then edit it
sudo ./install/panel/install.sh --site /etc/shippin/site.env
```

Copy [config/site.env.example](../config/site.env.example) to `/etc/shippin/site.env` first (`chmod 600`). That file holds the names and secrets for this installation. The repository copy is an example with `example.invalid` and no secrets.

The installer writes:

- `/etc/shippin/shippin.json`, from [config/shippin.example.json](../config/shippin.example.json), with domain and identity fields taken from the site file
- `/etc/shippin/console.env` (mode 0600) with `SHIPPIN_DATABASE_URL` and `SHIPPIN_SESSION_KEY`
- `/etc/shippin/templates.json`, from [config/templates.json](../config/templates.json), when the site does not already have one
- `/usr/local/bin/shippin-console`

When systemd is running and the command is not `--example`, it enables `shippin-console.service`. Otherwise it starts the binary in the background and records the pid in `/var/lib/shippin/console.pid`.

The machine needs `git` (to clone this repository) and outbound HTTPS. Debian and Ubuntu ship Go and Node.js releases older than `go.mod` and `web/` require, so when `go` is missing or older than `go.mod`, or `node` is older than 24, the installer downloads the release pinned in `install/panel/install.sh` from go.dev or nodejs.org, checks its SHA-256, and unpacks it under `/usr/local/lib/shippin-toolchains`. It does not change the system `PATH`.

A passing health check looks like this:

```json
{"status":"ok","db":true,"uptime_s":1}
```

`db` must be true. The process listens on `SHIPPIN_LISTEN` (default `127.0.0.1:8080`).

## TLS

A browser can only use the panel over https: the session cookie is `Secure`, and state-changing requests must carry an `https://` origin that matches the host the browser used (including a non-default port). Pick one:

- Put a TLS reverse proxy in front of it and keep `SHIPPIN_LISTEN` on loopback.
- Let the panel serve TLS. Set `SHIPPIN_TLS_CERT` and `SHIPPIN_TLS_KEY` to a certificate and key you already have. The panel rereads them when either file changes, so a renewal job only has to replace the files.
- Set `SHIPPIN_TLS=self-signed` to have the installer generate a pair for `SHIPPIN_CONSOLE_HOST` under `/etc/shippin/tls/`. Browsers warn until you trust it.

The panel does not ship a certificate or a hostname. `SHIPPIN_CONSOLE_HOST` should be the name people type, because sign-in redirects are built from it.

## Reaching a panel inside a VM

When the panel runs in a guest and the hypervisor forwards one host address to it, listen on the guest's own address (for example `SHIPPIN_LISTEN=0.0.0.0:8443` with TLS on) and forward only the host address you want. With Incus, a virtual machine needs a NAT proxy device and a fixed guest address on the bridge:

```sh
incus config device override <vm> eth0 ipv4.address=<guest address>
incus config device add <vm> panel proxy nat=true listen=tcp:<host address>:443 connect=tcp:<guest address>:8443
```

`nat=true` forwards only the listen address given, so binding a single host address keeps the panel off the others.

## First owner

After the health check passes, the installer runs `shippin-console setup` and prints `setup_code=...` while no owner exists. The code is valid for 24 hours. Running the installer again before it is redeemed issues a new code and invalidates the old one; once an owner exists, nothing is issued. Open `https://<console host>/setup` and enter the code with your name and email (or `POST /v1/setup` with `code`, `email`, `name` from the panel's https origin). That creates the owner, signs you in, and shows eight recovery codes once. The sign-in page links to `/setup` while no owner exists and offers "Use a recovery code" afterwards.

The owner is the platform admin without any upstream IdP: a local session maps the owner to the `roles.iam` source of `operator.role`, and each directory org to an org membership. Tenant onboarding (`/welcome`) still creates its org through the upstream IdP, so a panel with no IdP client cannot create tenants yet. See [identity and access](identity-and-access.md).

The `--example` install redeems its own code as part of the smoke test.

## Connect a hypervisor

Add a host under **Infrastructure → Hosts & nodes**. The panel needs two things from it.

**Incus.** A trust token, which the panel exchanges for its own certificate. On the server:

```sh
incus config set core.https_address :8443
incus config trust add shippin-panel
```

**Proxmox VE.** An API token whose user holds these roles. `PVEVMAdmin` and `PVEAuditor` are enough to read and run guests. Creating one also needs `PVEDatastoreUser` for its disk and `PVESDNUser` for its network, and Proxmox refuses without them. On a node:

```sh
pveum user add shippin@pve
pveum acl modify / --users shippin@pve --roles PVEVMAdmin,PVEAuditor,PVEDatastoreUser,PVESDNUser
pveum user token add shippin@pve panel --privsep 0
```

Copy the secret it prints; Proxmox shows it once. With privilege separation left on, give the same roles to the token as well. To grant less, scope the roles to a pool or storage instead of `/`.

New guests take their disk from the first storage that allows it, and their network from the first bridge on the node. A Proxmox container needs a template on that node first. Incus downloads images itself, from the list under `ui.providers` in your configuration.

## Disposable proof

`--example` is only for a throwaway machine. It uses the committed example JSON, creates a local database, and generates a session key.

```sh
sudo ./install/panel/install.sh --example
```

## What the site file must set

| Variable | Required | Purpose |
| --- | --- | --- |
| `SHIPPIN_DOMAIN` | yes for a real site | Public domain written into the JSON config |
| `SHIPPIN_CONSOLE_HOST` | yes for a real site | Hostname of this panel |
| `SHIPPIN_OPERATOR_HOST` | when the operator UI is separate | Operator hostname |
| `SHIPPIN_IDP_ISSUER` | before sign-in works | OIDC issuer URL |
| `SHIPPIN_OIDC_CLIENT_ID` | before sign-in works | OIDC client id. Empty leaves login disabled |
| `SHIPPIN_IDP_DIALECT` | when the issuer is upstream Zitadel | `zitadel` spells scopes, claims and the org header the upstream way. Default `nomen`. Sets `idp.dialect` |
| `SHIPPIN_IDP_PROVIDERS` | for Google, GitHub... buttons | `id:Label:idp_id`, comma separated. Sets `idp.providers` |
| `SHIPPIN_OPERATOR_PLATFORM_ORG_ID` | for operator and founder roles | IdP org that runs the platform. Sets `operator.platform_org_id` |
| `SHIPPIN_STAFF_PROJECT_ID` | for staff and founder roles | IdP project whose roles count. Sets `staff.project_id` |
| `SHIPPIN_IDP_SERVICE_TOKEN` | for tenant and workspace creation | Machine token that can create IdP orgs. Kept in `console.env` across re-runs |
| `SHIPPIN_IDP_READ_TOKEN`, `SHIPPIN_IDP_TEAM_TOKEN` | for operator pages and the Team page | Read-only and staff machine tokens. Kept in `console.env` across re-runs |
| `SHIPPIN_LISTEN` | no | Bind address. Default `127.0.0.1:8080` |
| `SHIPPIN_TLS_CERT`, `SHIPPIN_TLS_KEY` | no | Serve TLS from this pair. Both or neither |
| `SHIPPIN_TLS` | no | `self-signed` generates a pair for `SHIPPIN_CONSOLE_HOST` |
| `SHIPPIN_SIGNUP` | no | `open`, `invite_only`, or `waitlist`. Sets `directory.signup` |
| `SHIPPIN_CAPTCHA_PROVIDER` | no | `none` or `turnstile`. Sets `directory.captcha.provider` |
| `SHIPPIN_SERVERS_ENABLED` | no | `1` turns on server enrollment (`servers.enabled`) |
| `SHIPPIN_DATABASE_URL` | no | Postgres URL. Empty means the installer creates a local database |
| `SHIPPIN_SELF_GUEST` | no | The guest this panel runs on, as a hypervisor names it. Bulk stop always skips it and a single stop asks first. Empty uses the machine's hostname |
| `SHIPPIN_SESSION_KEY` | no | 64 hex characters. Empty means the installer generates one and keeps it across re-runs. It signs sessions and seals secrets saved in the panel: changing it signs everyone out and those secrets must be entered again |
| `SHIPPIN_SMTP_PASSWORD` | only if mail is enabled in JSON | Named by `mail.password_env` |
| `SHIPPIN_NOMEN_ISSUER` | when Nomen is the upstream issuer | Sets `idp.issuer` |
| `SHIPPIN_OIDC_CLIENT_SECRET` | when that issuer requires a secret | Not stored in the JSON file |
| `SHIPPIN_CAPTCHA_SECRET` | when captcha provider is `turnstile` | Siteverify secret |
| `SHIPPIN_MAIL_SINK` | disposable installs only | File that receives mail bodies. Leave unset on a real site |

Host agent settings stay in `/etc/shippin/hostd.json`, copied from [deploy/hostd.example.json](../deploy/hostd.example.json). SMTP passwords and optional externally managed provider-token environment variables stay in the root-only `/etc/shippin/console.env`. Provider endpoints and their per-server credentials are configured through **Infrastructure → Hosts & nodes**; TLS verification remains enabled.

## What is switched on

A fresh install offers hosts, guests, fleets, approvals and the provider panel, and nothing that needs something else. The rest appears when you configure it:

| Feature | Turn it on with |
| --- | --- |
| Workspaces, projects, templates, the host and edge pages | `SHIPPIN_HOSTD_URL`, with the host agent installed |
| Customers, plans and the service catalog | `SHIPPIN_CATALOG_ENABLED=1` |
| Enrolling servers from a tenant | `SHIPPIN_SERVERS_ENABLED=1` |
| Chat, AI subscriptions, bridge sessions, secrets, orgs | `chat`, `ai`, `bridge`, `vaultix` and `orgs` set to `enabled` in the config file |

## Watching other services

**Platform health** shows a card for each service you ask it to watch, and none until you do. Add them under **Provider panel → Platform settings**, or in `probes` in the config file, each with a name, a kind and either an HTTP `url` or a `tcp` address. A kind of `identity`, `secrets`, `mail`, `dns`, `proxy` or `database` picks a matching icon, and anything else gets a plain one. Set `instance` to tie a card to an Incus instance and show its memory and address.

## The audit log

Every run of the installer hands `audit_log` to a database role nobody can log in as, so the panel can read and append but not change or delete, and adds a hash chain that can be re-walked. **Provider panel → Tenants** shows whether it is protected and intact.

This needs a superuser, so it only happens on the database the installer created. If you brought your own, run `internal/db/audit_owner.sql` and `internal/db/audit_chain.sql` as a superuser with `SET LOCAL shippin.app_role = '<your role>'`, after the panel has applied its migrations. Under `docker compose` the panel's database user is a superuser, so the log is not protected there, and the panel says so.

## Updates

From the checkout the panel was installed from:

```sh
git pull --ff-only
sudo ./install/panel/install.sh --site /etc/shippin/site.env
```

That rebuilds the panel, applies database migrations and restarts it. The owner, sessions, hosts and saved secrets stay. `scripts/check-upgrade.sh <older commit> <target commit>` proves this on a disposable machine, and CI runs it under systemd. The panel's top bar shows when a newer build exists.

Migrations run forward only, so going back to an older build means restoring a database backup taken before the update. Take one first.

If the new process fails its health check, the installer says so and stops. Start the previous binary again, or fix what it reports and re-run. The older Incus release scripts under `deploy/` can still roll a release back when `CONSOLE_IP`, `DOMAIN`, and `SECRETS_FILE` are set in the environment. They no longer default to a site address.

GitHub Actions on pushes and pull requests runs the Go tests, the web tests and build, reference-tree checks for the retained Proxmox source capture, the public-tree literal scan, gitleaks, this installer with `--example`, and an update from an older build under systemd. The reference-tree job does not build or ship a second panel or runtime.
