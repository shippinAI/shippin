# Legacy runner prototype configuration

`internal/runtime/runfleet` and `cmd/shippin-run` contain the earlier standalone prototype.
For the current panel's tenant-scoped build runners, use [Build runners](../runners.md).
`internal/services/jobs` is the console worker and is not a GitHub runner. The Warden
reference files under `third_party/warden-ct102` are not served.
[Health](../specs/runner-fleet-health-and-stats.md) describes the probes.
[Deploy](shippin-run-deploy.md) is the host and DNS procedure.
The MCP client notes are in [cmd/shippin-run/README.md](../../cmd/shippin-run/README.md).

Nothing here is collected as a card charge. The meter reports a list-price amount.

## Platform boundary

PR #18 originated at `49aa8c3e70382698fa91aeaba080942247617e37`, before the
platform's tenant-scoped runner implementation. Its code and tests are preserved
here, with security and correctness repairs. This is source consolidation, not a
claim that all prototype capabilities have been integrated into the panel.

| Original capability | Current disposition |
| --- | --- |
| Fleet file and Warden-shaped inventory | Standalone compatibility tool only. Platform machine state belongs to `internal/runtime/runner` and PostgreSQL. |
| Account onboarding, shared organization runners, automatic public-repo approval | Prototype behavior only. Platform repository assignments require tenant management permission and use one VM per job; no shared-runner policy is imported. |
| GitHub credentials, webhook, HTTP and hand-written MCP | Isolated prototype process only. Platform uses `internal/integrations/gh`, its one webhook and generated MCPL actions. Never route panel clients through this process. |
| Workflow previews and optional pull requests | Retained and tested in the prototype; not exposed by the panel. Review every proposed diff, including the placeholder build step in a new workflow. |
| Redacted logs, retention and deterministic explanations | Retained and tested in the prototype. Platform job logs remain unfinished. Expired bodies are purged on access; zero retention purges immediately. No background timer or guaranteed recognition of every possible secret. |
| Usage arithmetic and sample prices | Prototype comparison meter only; not billing authority. Platform job timing and plan limits retain their existing owners. |
| Proxmox health composition | Prototype compatibility probe for existing fleet rows. Platform provider operations remain in `internal/hv`; prototype guest creation is unimplemented. |
| Static site, local fonts and rack simulator | Preserved prototype demonstration, not a hosted-service offer or proof of a running runner. Font license remains with the assets. |
| Linux/Hyper-V install helpers | Optional prototype evaluation only. They do not install or configure the platform panel. Live installation is unverified in this PR. |

The bearer token grants control of the entire prototype state directory. There is
no tenant isolation, panel identity, or shared secret store in this process. Use a
separate directory and credentials for legacy evaluation. The console dependency
test fails if this stateful package is pulled into the panel's executable. New
platform work belongs in the owners listed in [source ownership](../source-of-truth.md).

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
