# Deploy the panel

One command installs the panel on a fresh Debian or Ubuntu machine (Debian 13 is tested). It installs Postgres, fetches the Go and Node.js toolchains the build needs when the machine does not already have new enough ones, builds the control plane and the web UI, writes a root-only env file, starts the process, checks `GET /healthz`, and prints the one-time setup code for the first owner.

```sh
sudo ./install/panel/install.sh --site /etc/shippin/site.env
```

Copy [config/site.env.example](../config/site.env.example) to `/etc/shippin/site.env` first (`chmod 600`). That file holds the names and secrets for this installation. The repository copy is an example with `example.invalid` and no secrets.

The installer writes:

- `/etc/shippin/shippin.json`, from [config/shippin.example.json](../config/shippin.example.json), with domain and identity fields taken from the site file
- `/etc/shippin/console.env` (mode 0600) with `SHIPPIN_DATABASE_URL` and `SHIPPIN_SESSION_KEY`
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

After the health check passes, the installer runs `shippin-console setup` and prints `setup_code=...` while no owner exists. The code is valid for 24 hours. Running the installer again before it is redeemed issues a new code and invalidates the old one; once an owner exists, nothing is issued. Redeem it once with `POST /v1/setup` (`code`, `email`, `name`) from the panel's https origin. That creates the owner and returns eight recovery codes, shown once. See [identity and access](identity-and-access.md).

The `--example` install redeems its own code as part of the smoke test.

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
| `SHIPPIN_LISTEN` | no | Bind address. Default `127.0.0.1:8080` |
| `SHIPPIN_TLS_CERT`, `SHIPPIN_TLS_KEY` | no | Serve TLS from this pair. Both or neither |
| `SHIPPIN_TLS` | no | `self-signed` generates a pair for `SHIPPIN_CONSOLE_HOST` |
| `SHIPPIN_SIGNUP` | no | `open`, `invite_only`, or `waitlist`. Sets `directory.signup` |
| `SHIPPIN_CAPTCHA_PROVIDER` | no | `none` or `turnstile`. Sets `directory.captcha.provider` |
| `SHIPPIN_SERVERS_ENABLED` | no | `1` turns on server enrollment (`servers.enabled`) |
| `SHIPPIN_DATABASE_URL` | no | Postgres URL. Empty means the installer creates a local database |
| `SHIPPIN_SESSION_KEY` | no | 64 hex characters. Empty means the installer generates one |
| `SHIPPIN_SMTP_PASSWORD` | only if mail is enabled in JSON | Named by `mail.password_env` |
| `SHIPPIN_NOMEN_ISSUER` | when Nomen is the upstream issuer | Sets `idp.issuer` |
| `SHIPPIN_OIDC_CLIENT_SECRET` | when that issuer requires a secret | Not stored in the JSON file |
| `SHIPPIN_CAPTCHA_SECRET` | when captcha provider is `turnstile` | Siteverify secret |
| `SHIPPIN_MAIL_SINK` | disposable installs only | File that receives mail bodies. Leave unset on a real site |

Host agent settings, hypervisor tokens, and SMTP passwords stay in that uncommitted file or in `/etc/shippin/hostd.json` copied from [deploy/hostd.example.json](../deploy/hostd.example.json). The Proxmox panel reads `/etc/warden/warden.env`, copied from [third_party/warden-ct102/warden.env.example](../third_party/warden-ct102/warden.env.example), with TLS verification left on.

## Updates

Build a new binary from `main`, replace `/usr/local/bin/shippin-console`, and restart the service. If the new process fails its health check, start the previous binary again. The older Incus release scripts under `deploy/` can still roll a release back when `CONSOLE_IP`, `DOMAIN`, and `SECRETS_FILE` are set in the environment. They no longer default to a site address.

GitHub Actions on pull requests and `main` runs the Go tests, the web tests and build, the Warden module tests, the public-tree literal scan, gitleaks, and this installer with `--example`.
