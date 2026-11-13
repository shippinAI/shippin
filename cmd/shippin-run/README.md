# shippin-run

`shippin-run` is the runner control plane and its MCP server. The rules live in
`internal/runfleet`. This process does not keep a second copy.

```sh
shippin-run mcp      # stdio MCP
shippin-run serve    # HTTP API, webhook, and POST /mcp
shippin-run sizes    # size catalog as JSON
shippin-run install-url
```

`serve` refuses to start without `SHIPPIN_RUN_API_TOKEN`. `mcp` is local stdio
and trusts the user on that machine. A remote client, including Lovable, uses
`POST /mcp` with the bearer token. The static site does not serve MCP.

## Tools

| Tool | Behavior |
| --- | --- |
| `connect_repo` | Plans onboarding. User accounts are registered per repository. A public repository with a detected license is approved. Private repositories stay pending. |
| `list_runners` | Reads `ci_runners.json`. |
| `get_runner_health` | GitHub, substrate, and host load. |
| `list_jobs` | Stored jobs, without log bodies. |
| `get_job_logs` | Redacts secrets. Omits logs past retention. |
| `explain_failure` | Classifies a stored log. It does not call a model. |
| `set_runs_on` | Previews a `runs-on: shippin-<size>` edit. `apply: true` opens a pull request. |
| `get_usage` | Meters minutes. It does not collect payment. |
| `set_log_retention` | Days to keep logs. Zero discards the body. |
| `approve_repo` | Approves a pending repository. Requires a reason. |

Every tool result is JSON, both as text and as `structuredContent`.

## Cursor, Claude, Lovable

Stdio, for Cursor or Claude on the control-plane host:

```json
{
  "mcpServers": {
    "shippin-run": {
      "command": "shippin-run",
      "args": ["mcp"],
      "env": { "SHIPPIN_RUN_STATE_DIR": "/var/lib/shippin-run" }
    }
  }
}
```

Remote, for Lovable or another client that accepts an MCP URL:

```json
{
  "mcpServers": {
    "shippin-run": {
      "url": "https://runners.example/mcp",
      "headers": { "Authorization": "Bearer $SHIPPIN_RUN_API_TOKEN" }
    }
  }
}
```

Replace the URL with the control plane you host. Lovable-synced repositories are
detected from `.lovable/project.json` when the GitHub App can read the repo.
`connect_repo` with `apply_workflow: true` opens the pull request.

## What is real

- Fleet file load and save, including a Warden-shaped `ci_runners.json`.
- Onboarding plans, OSS auto-approval, workflow preview, and pull requests when GitHub credentials are configured.
- Webhook signature checks, log redaction, retention, and the usage meter.
- Proxmox health probes for a guest already listed in the fleet file.

Creating a Proxmox or Incus guest is not implemented. Windows and macOS images
are not built. Payment is not collected. The console panel has no form for
these settings yet.

Configuration: [docs/reference/shippin-run.md](../../docs/reference/shippin-run.md).
Deploy: [docs/reference/shippin-run-deploy.md](../../docs/reference/shippin-run-deploy.md).
