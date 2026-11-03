package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDomainRegistryFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.yaml")
	content := []byte(`
version: 1
last_verified: "2026-05-31"
workspace_id: wardenclyffe.infra
project_key: domain-dns-control-plane
public_edge:
  id: node-edge-lxc-115
  public_ip: 192.0.2.20
domains:
  - name: example.invalid
    status: active_dns_public_path_blocked
    internal_zone: true
    records:
      - host: warden
        type: A
        target: 192.0.2.20
        status: internal_live_public_blocked
routes:
  - id: warden-public
    fqdn: warden.example.invalid
    status: internal_live_public_blocked
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WARDEN_DOMAIN_REGISTRY", path)

	reg := getDomainRegistry()
	if !reg.Available {
		t.Fatalf("registry unavailable: %s", reg.Error)
	}
	if reg.WorkspaceID != "wardenclyffe.infra" {
		t.Fatalf("workspace mismatch: %q", reg.WorkspaceID)
	}
	if reg.Summary.DomainCount != 1 || reg.Summary.RecordCount != 1 || reg.Summary.RouteCount != 1 {
		t.Fatalf("unexpected summary: %+v", reg.Summary)
	}
	if got := reg.Domains[0].Records[0].FQDN; got != "warden.example.invalid" {
		t.Fatalf("expected fqdn to be derived, got %q", got)
	}
}
