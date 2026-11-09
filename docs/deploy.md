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
python install/docker/configure.py # generates missing credentials; keeps existing values
docker compose up -d --build
docker compose logs panel | grep setup_code
```

With generated credentials and no other settings it serves `https://localhost:8443` with a generated certificate. PostgreSQL and authenticated Dragonfly are private services in the same Compose project, with no database ports published. `install/docker/postgres.compose.yaml` and `install/docker/dragonfly.compose.yaml` own those definitions; root `compose.yaml` assembles the installation. PostgreSQL holds platform data, encrypted sessions and durable jobs. Dragonfly holds expiring shared password/passkey/public-form rate-limit counters. Its snapshot volume does not replace PostgreSQL backups. The platform health page reports live connectivity for both. Loss of a configured Dragonfly connection denies limited authentication attempts and returns HTTP 503 from `/readyz` and `/healthz`.

For a real site, configure the hostname, bind address and TLS. State lives in `db`, `state` and `dragonfly` volumes. Keep the same Compose project name on upgrade; older installations created as `shippin` must continue using `docker compose -p shippin` to retain their volumes. The current default project name is `shippin-platform`.

For hosted dependencies, set `SHIPPIN_DATABASE_URL` and/or `SHIPPIN_REDIS_URL` in `.env` (use PostgreSQL TLS settings and `rediss://` where required). Explicit URLs override the bundled defaults. Start only the selected services: `docker compose up -d --no-deps panel` for both external, or include `db` or `dragonfly` when that dependency is bundled. Run the credential initializer before Compose even for unused local definitions. Use a unique `SHIPPIN_REDIS_PREFIX` per installation when sharing an external Redis database. Connection credentials are bootstrap configuration; the live health table does not edit or migrate databases. Tenant-managed PostgreSQL, Dragonfly and ClickHouse deployment, backups and upgrades are separate unfinished capabilities.

Images and capacity are configurable through `SHIPPIN_POSTGRES_IMAGE`, `SHIPPIN_DRAGONFLY_IMAGE`, `SHIPPIN_DRAGONFLY_MAXMEMORY` and `SHIPPIN_DRAGONFLY_THREADS`. This Docker path includes Dragonfly; the systemd installer still uses process-local limits unless `SHIPPIN_REDIS_URL` is configured.

To upgrade, run `git pull --ff-only && docker compose up -d --build`. The panel's top bar shows when a newer build exists, and this command.

Before upgrading, back up PostgreSQL and the `state` volume (including the session
encryption key), and retain the current image. Migrations 064–066 add password
credentials, explicit internal organization ownership, and default organization
workspaces. They preserve existing customer memberships, project/environment IDs,
and compute devspace IDs. Existing passwordless owners keep their sign-in methods
and can establish a password in **Account → Security** after signing in. Do not
reset setup or recreate the owner to upgrade. Reverting schema requires restoring
the matching database/state backups; changing the image alone is not a schema rollback.

After first installation, the owner lands in the operator area. Enter staff mode,
open **Organizations**, and create a customer with an explicitly selected owner and
plan. This creates a draft organization and its default workspace without
provisioning a server. Organization members open **Workspaces → Open projects**;
owners/admins can add/edit workspaces and create multiple projects in each. A
project without compute stays pending. Compute instance management is under
**Devspaces**; existing `/workspaces/{id}` compute links remain compatible.

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

After the health check passes, the installer runs `shippin-console setup` and prints `setup_code=...` while no owner exists. The code is valid for 24 hours. Running the installer again before it is redeemed issues a new code and invalidates the old one; once an owner exists, nothing is issued. Open `https://<console host>/setup` and enter the one-time code, then choose a username and password and provide your full name, email, organization name, and installation branding. The password is chosen by the owner; the installer does not ship a shared default credential. Setup atomically creates the owner, local organization and tenant boundary, and branding settings, signs the owner in, and shows eight recovery codes once. Sign-in accepts username or email with the chosen password, passkeys, and recovery codes.

The owner is the platform admin without any upstream IdP: a local session maps the owner to the `roles.iam` source of `operator.role`, and the first local organization maps directly to a customer tenant boundary. No compute is provisioned during first-run setup. Existing passwordless accounts can set their first password under **Account → Security** after signing in; accounts with a password must verify it to change it. See [identity and access](identity-and-access.md).

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

New guests take their disk from the first storage that allows it, and their network from the first bridge on the node. A Proxmox container needs a template on that node first. Incus downloads images itself. `ui.providers` in your configuration lists the images and, under `image_remotes`, the servers they come from.

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

GitHub Actions on pushes and pull requests runs the Shippin Go tests and builds, the web tests and build, the public-tree literal scan, gitleaks, installer smoke tests, and an update from an older build under systemd. Historical Proxmox source captures are retained for review but are not built, installed, or executed by product CI.

## Site tooling

Scripts written for one site, such as creating its identity organization or its live tests, are not part of the panel and are not in this repository. What remains in `deploy/` sets up the host agent, its edge and its images on any site.
