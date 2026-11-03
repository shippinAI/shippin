# Merge from shippin-console

Source: `shippinAI/shippin-console` `main` at `e35ab817bbfedc626f35aa2f42760df49d4684c2`, imported from a `git archive` of that commit. History was not imported. The archive already omitted 212 doc and test screenshots. App media under `third_party/warden-ct102/static` is included.

The public tree is the squashed, cleaned snapshot. A full history import was not used because this repository is public and the private history was not available to scan.

## Where each part landed

| Console path | In this repository | Which side wins |
| --- | --- | --- |
| `cmd/console`, `internal/` | same paths | Console implementation. `api/openapi/` remains the published contract. |
| `cmd/shippin-hostd`, `internal/hostd` | same paths | Console host agent. `agent/` stays the local Incus diagnostic. |
| `cmd/shippin-bridge`, `cmd/shippin-credwriter`, `cmd/ai-launcher`, `cmd/rrflow-state`, `cmd/catalog-demo`, `cmd/hub-demo`, `cmd/chat-probe`, `cmd/vaultix-demo` | same paths | Console. |
| `web/` | `web/` | Console UI. This is the panel the installer serves. `panel/` stays the API workbench and is not the install path. |
| `deploy/` | `deploy/` | Kept after site addresses and site paths were removed. The supported install path is `install/panel/install.sh`. |
| `config/shippin.json` | `config/shippin.example.json` | Example only. A site writes `/etc/shippin/shippin.json` from `config/site.env`. |
| `deploy/hostd.json` | `deploy/hostd.example.json` | Example only. |
| `third_party/warden-ct102` | same path | Kept. Defaults no longer name a site, a guest, or a token. TLS verification defaults on. |
| `internal/goldsign`, `internal/goldverify`, `internal/goldexport`, `web/src/lib/gold-sign`, API routes | same paths | Kept. This is the in-panel signing flow, including approval actions. Site paths and tailnet addresses were removed. |
| `internal/hv/proxmox`, `internal/hv/incus` | same paths | Console drivers. Proxmox verifies TLS and has no skip flag. |
| `internal/approval` | same path | Console board flow, including `409` with `waiting_on_board` and `card_key`. |

## Dropped

- `cmd/gold-passkey-verify` and `cmd/gold-sign-export`. They run on the Gold host, read `/etc/gold-change`, and sync over a tailnet. They are not a generic install.
- `deploy/gold-ailaunch`, `deploy/gold-hub2`, `deploy/gold-passkey`, `deploy/gold-vaultix`. Site change tickets and a tailnet URL.
- `deploy/screenshots`. Live-site capture scripts. The archive already omitted the PNG files.
- `config/sparks.gold.json`. Replaced by `config/sparks.example.json` with generic node ids.
- Screenshot binaries. They were not in the archive.

## Made generic

- Committed JSON uses `example.invalid` and documentation addresses, or empty fields the site must set. `host_label` is `primary`. Mail is disabled in the example.
- Real org ids, the Spark tenant id, and lab addresses were removed.
- Shell deploy scripts require `CONSOLE_IP`, `DOMAIN`, `PG_ADDR`, and similar variables instead of defaulting them.
- Warden's platform list, CI runner list, SMB share, and Proxmox env example are site-neutral. `PROXMOX_VERIFY_TLS` defaults to on.
- Probe and operator certificate dials verify TLS against the system trust store.

## Wardenclyffe coverage

`third_party/warden-ct102` is still the Proxmox panel. The native drivers in `internal/hv/proxmox` and `internal/hv/incus` are what the console calls. They do not replace the Warden UI for firewall editing, backup coverage, DNS screens, or the deployment catalog. Those stay in the Warden module.

## Not done by this import

Sign-in still needs an identity provider and a client id in the site file. The installer health check proves Postgres, migrations, and `GET /healthz`. It does not enroll a host, open a hypervisor, or issue a certificate.
