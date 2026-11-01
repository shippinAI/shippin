# Deploy the panel

One command installs the panel on a fresh Debian or Ubuntu machine. It installs Postgres, builds the control plane and the web UI, writes a root-only env file, starts the process, and checks `GET /healthz`.

```sh
sudo ./install/panel/install.sh --site /etc/shippin/site.env
```

Copy [config/site.env.example](../config/site.env.example) to `/etc/shippin/site.env` first (`chmod 600`). That file holds the names and secrets for this installation. The repository copy is an example with `example.invalid` and no secrets.

The installer writes:

- `/etc/shippin/shippin.json`, from [config/shippin.example.json](../config/shippin.example.json), with domain and identity fields taken from the site file
- `/etc/shippin/console.env` (mode 0600) with `SHIPPIN_DATABASE_URL` and `SHIPPIN_SESSION_KEY`
- `/usr/local/bin/shippin-console`

When systemd is running and the command is not `--example`, it enables `shippin-console.service`. Otherwise it starts the binary in the background and records the pid in `/var/lib/shippin/console.pid`.

A passing health check looks like this:

```json
{"status":"ok","db":true,"uptime_s":1}
```

`db` must be true. The process listens on `SHIPPIN_LISTEN` (default `127.0.0.1:8080`). Put a TLS reverse proxy in front of it before exposing it. The panel does not ship a certificate or a hostname.

The installer then runs `shippin-console setup` on a disposable `--example` install and prints `setup_code=...`. Redeem it with `POST /v1/setup`. That code creates the first owner. See [identity and access](identity-and-access.md).

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
