// Package runfleet is the legacy standalone fleet-file prototype from PR #18.
// The platform's tenant-scoped runners are owned by internal/runtime/runner and
// internal/runtime/runner_job. The console must not import this stateful package.
//
// It owns the fleet file (ci_runners.json), onboarding plans for GitHub user
// accounts and organizations, log redaction, retention, and the usage meter.
// cmd/shippin-run is the process that exposes this package over HTTP and MCP.
// Both call these functions; neither keeps a second copy of the rules.
//
// internal/services/jobs is the console's durable worker and is not a GitHub runner.
// third_party/warden-ct102 remains a reference import. Its health pill rule is
// re-authored here as Compose. Its HTML panel is not mounted.
//
// Creating a Proxmox or Incus guest is not implemented. Provision reports that
// explicitly and still records the runner the fleet file should hold.
package runfleet
