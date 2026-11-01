package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetManagedWorkspaceMapFromEnv(t *testing.T) {
	path := writeManagedWorkspaceFixture(t)
	t.Setenv("WARDEN_MANAGED_WORKSPACES_REGISTRY", path)

	view := getManagedWorkspaceMap()
	if !view.Available {
		t.Fatalf("workspace map unavailable: %s", view.Error)
	}
	if got := view.Summary.WorkspaceCount; got != 2 {
		t.Fatalf("workspace count = %d", got)
	}
	if got := view.Summary.ProjectCount; got != 2 {
		t.Fatalf("project count = %d", got)
	}
	if got := view.Summary.ConnectionCount; got < 5 {
		t.Fatalf("connection count = %d", got)
	}
	if view.Workspaces[0].Role == "" {
		t.Fatalf("expected normalized workspace role")
	}
}

func TestHandleManagedWorkspaces(t *testing.T) {
	path := writeManagedWorkspaceFixture(t)
	t.Setenv("WARDEN_MANAGED_WORKSPACES_REGISTRY", path)

	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	rec := httptest.NewRecorder()
	handleManagedWorkspaces(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"<body class=\"c-shell\">", "c-workspace-map", "Workspace Map", "wardenclyffe.infra", "node.master"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q", want)
		}
	}
	if strings.Contains(body, "Customer registry unavailable") {
		t.Fatalf("workspace route rendered the customer shell body")
	}
}

func writeManagedWorkspaceFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "managed-workspaces.yaml")
	content := []byte(`
version: 0.1
status: draft_seed
last_updated: 2026-06-01
workspaces:
  - workspace_uuid: 019e7184-cd51-7bfe-be5a-2533c98c9e9f
    workspace_slug: wardenclyffe.infra
    workspace_name: Wardenclyffe Infrastructure
    workspace_kind: operator-foundation
    status: active
    mcp_root: mcp.workspace.wardenclyffe.infra
    policy_root: policy.workspace.wardenclyffe.infra
    contributes_to_planes:
      - plane: intelligence
        surreal_ns: node
        surreal_db: ai_memory
  - workspace_uuid: bd64b9ff-701e-495a-9233-ae6b85668712
    workspace_slug: node.master
    workspace_name: Clyffy Master
    workspace_kind: master
    status: active
    mcp_root: mcp.workspace.node-master
    policy_root: policy.workspace.node-master
    baseline_for: [customer_bundle, specialist_team]
    contributes_to_planes:
      - plane: intelligence
        surreal_ns: node
        surreal_db: ai_memory
projects:
  - project_uuid: b7c7d7c3-df4b-4f96-b70e-25120f0fe4a9
    project_key: wardenclyffe-foundation
    workspace_slug: wardenclyffe.infra
    repo_dir: wardenclyffe
    status: active
  - project_uuid: 5fe7154e-6f9c-440a-aeb4-2726c5eaa856
    project_key: master-node
    workspace_slug: node.master
    repo_dir: Master-Clyffy
    status: active
service_instances:
  - service_instance_uuid: bf2cacd3-741d-430e-bf7a-ce10d7abb8fe
    service_slug: example-devstation
    display_name: Hades Devstation
    service_kind: ai_devstation
    primary_workspace_slug: wardenclyffe.infra
    attached_workspace_slugs: [wardenclyffe.infra, node.master]
    host:
      ssh_alias: devstation.node.ai
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
