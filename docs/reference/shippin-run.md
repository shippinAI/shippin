# Shippin Run configuration

`internal/runfleet` and `cmd/shippin-run` own the runner control plane.
`internal/jobs` is the console worker and is not a GitHub runner. The Warden
reference files under `third_party/warden-ct102` are not served.
[Health](../specs/runner-fleet-health-and-stats.md) describes the probes.
[Deploy](shippin-run-deploy.md) is the host and DNS procedure.
The MCP client notes are in [cmd/shippin-run/README.md](../../cmd/shippin-run/README.md).

Nothing here is collected as a card charge. The meter reports a list-price amount.

## Environment

Copy [config/shippin-run.env.example](../../config/shippin-run.env.example) to
the host. Empty values stay empty until the operator fills them on that machine.

| Variable | Role |
| --- | --- |
| `SHIPPIN_RUN_STATE_DIR` | Directory for `ci_runners.json` and `state.json`. Required. |
| `SHIPPIN_RUN_CONFIG` | Optional JSON for non-secret settings. |
| `SHIPPIN_RUN_LISTEN` | Listen address. Default `:8080`. |
| `SHIPPIN_RUN_API_TOKEN` | Bearer token. `serve` exits if this is empty. |
| `SHIPPIN_RUN_FREE_MINUTES` | Included minutes for `linux-x64-2`. `0` means no free pool. Unset uses the config file, then 3000. |
| `SHIPPIN_RUN_GITHUB_APP_ID` | GitHub App id. |
| `SHIPPIN_RUN_GITHUB_APP_SLUG` | Used only by `shippin-run install-url`. |
| `SHIPPIN_RUN_GITHUB_PRIVATE_KEY_FILE` | PEM RSA key. Not stored in the state directory. |
| `SHIPPIN_RUN_GITHUB_INSTALLATION_ID` | The one installation this process calls. |
| `SHIPPIN_RUN_GITHUB_TOKEN` | An installation token, instead of minting one. |
| `SHIPPIN_RUN_GITHUB_WEBHOOK_SECRET` | Verifies `X-Hub-Signature-256`. If empty, the webhook route returns 503. |
| `SHIPPIN_RUN_PROVISIONER` | `none`, `proxmox`, or `incus`. |
| `SHIPPIN_RUN_PROXMOX_API_URL` | API base URL for health probes. |
| `SHIPPIN_RUN_PROXMOX_TOKEN` | Proxmox API token (`user@realm!id=secret`). |
| `SHIPPIN_RUN_PROXMOX_NODE` | Node name used in probe paths. |

GitHub calls are configured only when a token is set, or when the app id, private
key, and installation id are all set. A partial set is a startup error.
Registration tokens are not requested, because guest creation is not implemented
and a token would be a credential with nowhere safe to go.

## JSON config

[config/shippin-run.example.json](../../config/shippin-run.example.json) is a
valid file. It sets the list prices used for comparison: Linux x64 `0.004`,
Linux ARM `0.0025`, Windows `0.008`, and macOS M4 `0.08` USD per minute. Those
rows do not mean Windows or macOS images exist. An installation that wants
different prices edits this file. The process does not read a price from code
when the file lists sizes.

`free_minutes` applies only to `free_size` (default `linux-x64-2`). Other sizes
are fully metered. Duration rounds up to the next second, then the price is
`seconds * micro_usd_per_minute / 60`, floored.

`default_log_retention_days` is 14 unless a repository sets its own value,
including 0. A zero in this JSON file is treated as “use 14”. The environment
variable `SHIPPIN_RUN_FREE_MINUTES=0` is the switch for no free pool; a zero
`free_minutes` in the JSON file is treated as 3000.

## ci_runners.json

[config/ci_runners.schema.json](../../config/ci_runners.schema.json) is the
fleet file. [config/ci_runners.example.json](../../config/ci_runners.example.json)
is an empty fleet. A file written by the Warden reference registry still loads:
`id`, `name`, `kind`, `vmid`, `host`, and `repos` are the original fields.
Shippin Run adds `size`, `labels`, `account`, `account_kind`, `status`, and
`provision_detail`.

The product does not invent default runners when the file is missing. A missing
file is an empty fleet. Invalid JSON is an error.

`state.json` in the same directory holds connections, jobs, and GitHub App
installations. Logs in that file are redacted before the write. The file mode
is `0600`.

## Sizes and labels

| id | runs-on | Guest image in this build |
| --- | --- | --- |
| `linux-x64-2` | `shippin-linux-x64-2` | Recorded only. No image is built here. |
| `linux-arm-2` | `shippin-linux-arm-2` | Recorded only. |
| `windows-x64-2` | `shippin-windows-x64-2` | Not built. |
| `macos-m4` | `shippin-macos-m4` | Not built. |

## HTTP

`GET /healthz` and `POST /v1/github/webhook` are unauthenticated. The webhook
checks the signature. Every other route, including `POST /mcp`, requires
`Authorization: Bearer`.

`POST /v1/connect` and `POST /v1/runs-on` match the MCP tools. Workflow edits
stay a dry-run unless the JSON sets the apply flag.

## Lovable

When the GitHub App can read the repository, a file at
`.lovable/project.json` marks it as Lovable-synced. If it has no workflow,
the plan adds `.github/workflows/shippin-run.yml`. `apply_workflow: true` on
`connect_repo`, or `apply: true` on `set_runs_on`, opens the pull request.
Without GitHub credentials the preview still works from facts you pass, and
apply is refused.

## Panel

These settings are not in the `web/` panel yet. That is unfinished relative to
CFG-01. The files and environment above are the operator surface for this build.
