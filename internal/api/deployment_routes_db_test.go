package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/deployments"
)

func deploymentRouteAPI(t *testing.T) (*API, string, string) {
	t.Helper()
	connection := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	database, err := db.Open(t.Context(), connection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		database.Pool.Close()
		t.Fatal(err)
	}
	database.Pool.Close()
	database, err = db.Open(t.Context(), connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Pool.Close)
	tenantID, targetID := uuid.NewString(), uuid.NewString()
	if _, err := database.Pool.Exec(t.Context(), `INSERT INTO tenants(id,idp_org_id,name,plan,status)
		VALUES($1,$2,'Deployment API test','starter','active')`, tenantID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), `INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by,hv_kind)
		VALUES($1,$2,'deployment-api-test','Deployment API target','test','active','test','proxmox')`, targetID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), `INSERT INTO server_assignments(server_id,tenant_id,role,created_by)
		VALUES($1,$2,'owner','test')`, targetID, tenantID); err != nil {
		t.Fatal(err)
	}
	schema := `{"type":"object","properties":{"password":{"type":"string","format":"shippin-secret-ref"}},"required":["password"],"additionalProperties":false}`
	defaults := `{"password":"vaultix://org/project/env/postgres"}`
	source := `{"kind":"github","reference":"https://github.com/acme/postgres","commit":"0123456789abcdef0123456789abcdef01234567"}`
	if _, err := database.Pool.Exec(t.Context(), `INSERT INTO catalog_services(id,label,kind,config_schema,default_config,deployment_source)
		VALUES('postgres','Postgres','database',$1::jsonb,$2::jsonb,$3::jsonb)`, schema, defaults, source); err != nil {
		t.Fatal(err)
	}
	a := &API{cfg: testCfg(), db: database, deployments: deployments.NewStore(database.Pool)}
	a.cfg.Jobs.IdempotencyHours = 24
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
		return &caller{id: &auth.Identity{Sub: "route-test-user"}, tenants: []tenantView{{Tenant: db.Tenant{ID: tenantID}, Roles: []string{"org_owner"}}}}, true
	}
	return a, tenantID, targetID
}

func deploymentRequest(method, tenantID, id, body, key, etag string) *http.Request {
	path := "/v1/tenants/" + tenantID + "/deployments"
	if id != "" {
		path += "/" + id
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.SetPathValue("tenantId", tenantID)
	if id != "" {
		r.SetPathValue("deploymentId", id)
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	if etag != "" {
		r.Header.Set("If-Match", etag)
	}
	return r
}

func TestDeploymentTenantAPIRecordsIntentAndHistory(t *testing.T) {
	a, tenantID, targetID := deploymentRouteAPI(t)
	createKey := uuid.NewString()
	body := `{"service_id":"postgres","target_system_id":"` + targetID + `","desired_state":"present","config":{"password":"vaultix://org/project/env/postgres"}}`
	rec := httptest.NewRecorder()
	a.createDeployment(rec, deploymentRequest(http.MethodPost, tenantID, "", body, createKey, ""))
	if rec.Code != http.StatusCreated || rec.Header().Get("ETag") != `"1"` {
		t.Fatalf("create response: %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	var created deployments.Deployment
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Generation != 1 || created.TargetProvider != deployments.ProviderProxmox || created.ObservedState != deployments.ObservedUnknown {
		t.Fatalf("created intent: %+v err=%v", created, err)
	}

	replayed := httptest.NewRecorder()
	a.createDeployment(replayed, deploymentRequest(http.MethodPost, tenantID, "", body, createKey, ""))
	if replayed.Code != rec.Code || replayed.Body.String() != rec.Body.String() || replayed.Header().Get("Idempotent-Replayed") != "true" || replayed.Header().Get("ETag") != `"1"` {
		t.Fatalf("create replay differs: %d %v %s", replayed.Code, replayed.Header(), replayed.Body)
	}

	updateBody := `{"desired_state":"present","config":{"password":"vaultix://org/project/env/postgres"}}`
	updated := httptest.NewRecorder()
	a.updateDeployment(updated, deploymentRequest(http.MethodPatch, tenantID, created.ID, updateBody, uuid.NewString(), `"1"`))
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") != `"2"` {
		t.Fatalf("update response: %d %v %s", updated.Code, updated.Header(), updated.Body)
	}
	stale := httptest.NewRecorder()
	a.updateDeployment(stale, deploymentRequest(http.MethodPatch, tenantID, created.ID, updateBody, uuid.NewString(), `"1"`))
	if stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale If-Match response: %d %s", stale.Code, stale.Body)
	}

	removed := httptest.NewRecorder()
	a.deleteDeployment(removed, deploymentRequest(http.MethodDelete, tenantID, created.ID, "", uuid.NewString(), `"2"`))
	if removed.Code != http.StatusOK || !strings.Contains(removed.Body.String(), `"desired_state":"absent"`) || !strings.Contains(removed.Body.String(), `"observed_state":"pending"`) {
		t.Fatalf("desired removal response: %d %s", removed.Code, removed.Body)
	}
	events := httptest.NewRecorder()
	a.listDeploymentEvents(events, deploymentRequest(http.MethodGet, tenantID, created.ID, "", "", ""))
	if events.Code != http.StatusOK || !strings.Contains(events.Body.String(), `"total_estimate":3`) {
		t.Fatalf("generation history response: %d %s", events.Code, events.Body)
	}
}
