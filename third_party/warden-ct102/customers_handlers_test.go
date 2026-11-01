package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleCustomersIndexUsesCustomerShellBody(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`
schema_version: 1
customer_account_slug: hades
last_updated: 2026-06-01
services:
  - service_id: claude-code
    enabled: true
    tier_eligibility: [T1]
    host:
      host_id: example-devstation
    auth_override:
      kind: oauth
    catalog:
      sort_priority: 1
`)
	if err := os.WriteFile(filepath.Join(dir, "hades.services.yaml"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WARDEN_CUSTOMER_SERVICES_DIR", dir)
	loadCustomersFromDisk()
	t.Cleanup(func() {
		customers.mu.Lock()
		defer customers.mu.Unlock()
		customers.byAccount = make(map[string]*CustomerServicesFile)
		customers.loadedFrom = ""
		customers.loadError = ""
	})

	req := httptest.NewRequest(http.MethodGet, "/customers", nil)
	rec := httptest.NewRecorder()
	handleCustomersIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"<body class=\"c-shell\">", "Customers", "Hades", "c-entity-list"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q", want)
		}
	}
	if strings.Contains(body, "Workspace Boundaries") {
		t.Fatalf("customers route rendered the workspace shell body")
	}
}
