// Package runfleet is the GitHub Actions runner control plane.
//
// It owns the fleet file (ci_runners.json), onboarding plans for GitHub user
// accounts and organizations, log redaction, retention, and the usage meter.
// cmd/shippin-run is the process that exposes this package over HTTP and MCP.
// Both call these functions; neither keeps a second copy of the rules.
//
// internal/jobs is the console's durable worker and is not a GitHub runner.
// third_party/warden-ct102 remains a reference import. Its health pill rule is
// re-authored here as Compose. Its HTML panel is not mounted.
//
// Creating a Proxmox or Incus guest is not implemented. Provision reports that
// explicitly and still records the runner the fleet file should hold.
package runfleet
