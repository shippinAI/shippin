package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/vaultix"
)

func TestRevealTenantDeploymentSecretDerivesTenantProjectAndAuditsBeforeReveal(t *testing.T) {
	a, tenantID, _ := deploymentRouteAPI(t)
	deploymentID := "e90b1445-8b45-4ba5-9e6e-9b5381cd044f"
	var revealCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/universal-auth/login":
			json.NewEncoder(w).Encode(map[string]any{"accessToken": "test-token", "expiresIn": 3600})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v2/workspace/"):
			if got, want := strings.TrimPrefix(r.URL.Path, "/api/v2/workspace/"), "tenant-"+tenantID; got != want {
				t.Errorf("resolver used wrong Vaultix project slug: got %q want %q", got, want)
			}
			json.NewEncoder(w).Encode(map[string]any{"id": "vaultix-project-id"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/secrets/raw/db-password":
			revealCount++
			if got := r.URL.Query().Get("workspaceId"); got != "vaultix-project-id" {
				t.Errorf("Reveal workspaceId = %q", got)
			}
			if got := r.URL.Query().Get("environment"); got != "production" {
				t.Errorf("Reveal environment = %q", got)
			}
			if got := r.URL.Query().Get("secretPath"); got != "/services/postgres" {
				t.Errorf("Reveal secretPath = %q", got)
			}
			var action, detail string
			if err := a.db.Pool.QueryRow(context.Background(), `SELECT action,detail FROM audit_log WHERE tenant_id=$1 ORDER BY id DESC LIMIT 1`, tenantID).Scan(&action, &detail); err != nil {
				t.Errorf("audit must be written before Reveal: %v", err)
			} else if action != "deployment.secret.reveal" || !strings.Contains(detail, "deployment="+deploymentID) || !strings.Contains(detail, "generation=3") {
				t.Errorf("unexpected secret access audit: action=%q detail=%q", action, detail)
			}
			json.NewEncoder(w).Encode(map[string]any{"secret": map[string]any{"secretKey": "db-password", "secretPath": "/services/postgres", "secretValue": "test-secret-value"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	a.cfg.Vaultix.Enabled = true
	a.cfg.Vaultix.APIURL = server.URL
	a.cfg.Vaultix.Envs = []string{"production"}
	a.cfg.Vaultix.ProjectPrefix = "tenant-"
	a.n2.vx = &vaultixDeps{c: vaultix.New(server.URL, "client-id", "client-secret", time.Second), cfg: a.cfg.Vaultix, db: a.db}

	value, err := a.revealTenantDeploymentSecret(context.Background(), tenantID, deploymentID, 3,
		"deployment-test-worker", "vaultix://production/services/postgres/db-password")
	if err != nil || value != "test-secret-value" {
		t.Fatalf("Reveal result=%q err=%v", value, err)
	}
	if revealCount != 1 {
		t.Fatalf("Reveal count=%d want=1", revealCount)
	}
	var detail string
	if err := a.db.Pool.QueryRow(context.Background(), `SELECT detail FROM audit_log WHERE tenant_id=$1 AND action='deployment.secret.reveal' ORDER BY id DESC LIMIT 1`, tenantID).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(detail, "test-secret-value") || strings.Contains(detail, "db-password") {
		t.Fatalf("audit contains secret material or key: %q", detail)
	}
}

func TestRevealTenantDeploymentSecretRejectsUnconfiguredEnvironmentBeforeNetwork(t *testing.T) {
	a := &API{cfg: testCfg()}
	a.cfg.Vaultix.Enabled = true
	a.cfg.Vaultix.Envs = []string{"production"}
	a.cfg.Vaultix.ProjectPrefix = "tenant-"
	a.n2.vx = &vaultixDeps{c: vaultix.New("http://127.0.0.1:1", "unused", "unused", time.Second), cfg: a.cfg.Vaultix}
	if _, err := a.revealTenantDeploymentSecret(context.Background(), "e90b1445-8b45-4ba5-9e6e-9b5381cd044f",
		"e90b1445-8b45-4ba5-9e6e-9b5381cd044f", 1, "worker", "vaultix://other/database/password"); err == nil {
		t.Fatal("unconfigured environment was resolved")
	}
}
