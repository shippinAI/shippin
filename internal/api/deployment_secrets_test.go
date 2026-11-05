package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/vaultix"
)

func TestRevealTenantDeploymentSecretDerivesTenantProjectAndAuditsBeforeReveal(t *testing.T) {
	a, tenantID, targetID := deploymentRouteAPI(t)
	deployment, _, err := a.deploymentStore().Create(context.Background(), tenantID, "deployment-test-worker", deployments.CreateInput{
		ServiceID: "postgres", TargetSystemID: targetID, DesiredState: deployments.DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"password":"vaultix://production/services/postgres/password"}`),
		TargetConfig: json.RawMessage(`{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	deploymentID := deployment.ID
	jobID, leaseOwner := uuid.NewString(), "deployment-test-worker"
	_, err = a.db.Pool.Exec(context.Background(), `INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,status,created_by,lease_owner,lease_until)
		VALUES($1,$2,'deployment.reconcile','deployment_generation',$3,'running','deployment-test-worker',$4,now()+interval '1 minute')`,
		jobID, tenantID, deploymentID+":1", leaseOwner)
	if err != nil {
		t.Fatal(err)
	}
	var revealCount int
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
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
			} else if action != "deployment.secret.reveal" || !strings.Contains(detail, "deployment="+deploymentID) || !strings.Contains(detail, "generation=1") {
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

	if _, err := a.revealTenantDeploymentSecret(context.Background(), tenantID, deploymentID, 1,
		"deployment-test-worker", "vaultix://production/services/postgres/db-password", jobID, leaseOwner); err == nil {
		t.Fatal("secret resolution must be denied until the operator accepts Incus administrator visibility")
	}
	if requestCount != 0 || revealCount != 0 {
		t.Fatalf("Vaultix was called before target trust acceptance: requests=%d reveals=%d", requestCount, revealCount)
	}
	if _, err := a.db.Pool.Exec(context.Background(), `UPDATE servers SET hv_allow_tenant_deployment_secrets=true WHERE id=$1`, targetID); err != nil {
		t.Fatal(err)
	}
	value, err := a.revealTenantDeploymentSecret(context.Background(), tenantID, deploymentID, 1,
		"deployment-test-worker", "vaultix://production/services/postgres/db-password", jobID, leaseOwner)
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
	if _, err := a.db.Pool.Exec(context.Background(), `UPDATE jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.revealTenantDeploymentSecret(context.Background(), tenantID, deploymentID, 1,
		"deployment-test-worker", "vaultix://production/services/postgres/db-password", jobID, leaseOwner); err == nil {
		t.Fatal("expired lease must not authorize another secret reveal")
	}
	if revealCount != 1 {
		t.Fatalf("Vaultix Reveal called after lease expiry: count=%d", revealCount)
	}
}

func TestRevealTenantDeploymentSecretRejectsUnconfiguredEnvironmentBeforeNetwork(t *testing.T) {
	a := &API{cfg: testCfg()}
	a.cfg.Vaultix.Enabled = true
	a.cfg.Vaultix.Envs = []string{"production"}
	a.cfg.Vaultix.ProjectPrefix = "tenant-"
	a.n2.vx = &vaultixDeps{c: vaultix.New("http://127.0.0.1:1", "unused", "unused", time.Second), cfg: a.cfg.Vaultix}
	if _, err := a.revealTenantDeploymentSecret(context.Background(), "e90b1445-8b45-4ba5-9e6e-9b5381cd044f",
		"e90b1445-8b45-4ba5-9e6e-9b5381cd044f", 1, "worker", "vaultix://other/database/password", uuid.NewString(), "worker"); err == nil {
		t.Fatal("unconfigured environment was resolved")
	}
}
