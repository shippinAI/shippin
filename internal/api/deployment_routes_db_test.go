package api

import (
	"context"
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
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/servers"
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
		VALUES($1,$2,'deployment-api-test','Deployment API target','test','active','test','incus')`, targetID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), `INSERT INTO server_assignments(server_id,tenant_id,role,created_by)
		VALUES($1,$2,'owner','test')`, targetID, tenantID); err != nil {
		t.Fatal(err)
	}
	schema := `{"type":"object","properties":{"password":{"type":"string","format":"shippin-secret-ref","description":"sops://runtime/schema-secret"}},"required":["password"],"additionalProperties":false}`
	defaults := `{"password":"vaultix://production/services/postgres/password"}`
	source := `{"kind":"oci","reference":"registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","credential_ref":"sops://runtime/secrets.yaml#/registry/token"}`
	runtime := `{"version":2,"kind":"incus_oci_container","providers":["incus"],"resources":{"cpu_cores":2,"memory_mib":2048},"storage":{"size_gib":20,"mount_path":"/var/lib/postgresql/data"},"network":{"exposure":"private","ports":[{"name":"postgres","protocol":"tcp","container_port":5432}]},"health":{"kind":"tcp","port":5432,"interval_seconds":30,"timeout_seconds":5,"failure_threshold":3}}`
	if _, err := database.Pool.Exec(t.Context(), `INSERT INTO catalog_services(id,label,kind,config_schema,default_config,deployment_source,runtime_contract)
		VALUES('postgres','Postgres','database',$1::jsonb,$2::jsonb,$3::jsonb,$4::jsonb)`, schema, defaults, source, runtime); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), `INSERT INTO catalog_services(id,label,kind,config_schema,default_config,deployment_source)
		VALUES('metadata-only','Metadata only','database',$1::jsonb,$2::jsonb,$3::jsonb)`, schema, defaults,
		`{"kind":"github","reference":"https://github.com/acme/metadata-only","commit":"0123456789abcdef0123456789abcdef01234567"}`); err != nil {
		t.Fatal(err)
	}
	a := &API{cfg: testCfg(), db: database, deployments: deployments.NewStore(database.Pool)}
	a.cfg.Jobs.IdempotencyHours = 24
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
		return &caller{id: &auth.Identity{Sub: "route-test-user"}, managers: testCfg().Roles.Managers,
			tenants: []tenantView{{Tenant: db.Tenant{ID: tenantID}, Roles: []string{"org_owner"}}}}, true
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
	a.hvServers = &memHV{items: []servers.Endpoint{{ID: targetID, Kind: "incus"}}}
	openedDrivers := 0
	a.openHV = func(servers.Endpoint) (hv.Driver, error) {
		openedDrivers++
		return deploymentStorageInventoryDriver{}, nil
	}
	services := httptest.NewRecorder()
	a.listDeployableServices(services, deploymentRequest(http.MethodGet, tenantID, "", "", "", ""))
	if services.Code != http.StatusOK || strings.Contains(services.Body.String(), "credential_ref") || strings.Contains(services.Body.String(), "vaultix://") || strings.Contains(services.Body.String(), "sops://") || strings.Contains(services.Body.String(), "metadata-only") || !strings.Contains(services.Body.String(), `"kind":"incus_oci_container"`) || !strings.Contains(services.Body.String(), `"version":2`) || !strings.Contains(services.Body.String(), `"mount_path":"/var/lib/postgresql/data"`) {
		t.Fatalf("tenant catalog options leaked references or failed: %d %s", services.Code, services.Body)
	}
	targets := httptest.NewRecorder()
	a.listDeploymentTargets(targets, deploymentRequest(http.MethodGet, tenantID, "", "", "", ""))
	if targets.Code != http.StatusOK || !strings.Contains(targets.Body.String(), `"provider":"incus"`) || !strings.Contains(targets.Body.String(), `"can_request":true`) {
		t.Fatalf("assigned deployment target not exposed: %d %s", targets.Code, targets.Body)
	}
	managerResolver := a.resolveCaller
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
		return &caller{id: &auth.Identity{Sub: "route-test-viewer"}, tenants: []tenantView{{Tenant: db.Tenant{ID: tenantID}, Roles: []string{"org_member"}}}}, true
	}
	viewerTargets := httptest.NewRecorder()
	a.listDeploymentTargets(viewerTargets, deploymentRequest(http.MethodGet, tenantID, "", "", "", ""))
	if viewerTargets.Code != http.StatusOK || !strings.Contains(viewerTargets.Body.String(), `"can_request":false`) {
		t.Fatalf("member target view: %d %s", viewerTargets.Code, viewerTargets.Body)
	}
	a.resolveCaller = managerResolver
	missingPool := httptest.NewRecorder()
	a.createDeployment(missingPool, deploymentRequest(http.MethodPost, tenantID, "",
		`{"service_id":"postgres","target_system_id":"`+targetID+`","desired_state":"present","config":{"password":"vaultix://production/services/postgres/password"},"target_config":{"storage":{"pool":"not-on-target"},"network":{"name":"private-apps"}}}`,
		uuid.NewString(), ""))
	if missingPool.Code != http.StatusConflict || openedDrivers != 1 {
		t.Fatalf("a pool absent from live inventory must be rejected after one provider read: status=%d opened=%d body=%s", missingPool.Code, openedDrivers, missingPool.Body)
	}
	missingNetwork := httptest.NewRecorder()
	a.createDeployment(missingNetwork, deploymentRequest(http.MethodPost, tenantID, "",
		`{"service_id":"postgres","target_system_id":"`+targetID+`","desired_state":"present","config":{"password":"vaultix://production/services/postgres/password"},"target_config":{"storage":{"pool":"tenant-data"},"network":{"name":"not-on-target"}}}`,
		uuid.NewString(), ""))
	if missingNetwork.Code != http.StatusConflict || openedDrivers != 2 {
		t.Fatalf("a network absent from live inventory must be rejected: status=%d opened=%d body=%s", missingNetwork.Code, openedDrivers, missingNetwork.Body)
	}
	createKey := uuid.NewString()
	body := `{"service_id":"postgres","target_system_id":"` + targetID + `","desired_state":"present","config":{"password":"vaultix://production/services/postgres/password"},"target_config":{"storage":{"pool":"tenant-data"},"network":{"name":"private-apps"}}}`
	rec := httptest.NewRecorder()
	a.createDeployment(rec, deploymentRequest(http.MethodPost, tenantID, "", body, createKey, ""))
	if rec.Code != http.StatusCreated || rec.Header().Get("ETag") != `"1"` || openedDrivers != 3 {
		t.Fatalf("create response: %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	var created deployments.Deployment
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Generation != 1 || created.TargetProvider != deployments.ProviderIncus || created.ObservedState != deployments.ObservedUnknown || len(created.ConfigSchema) == 0 || created.TargetConfig.Storage == nil || created.TargetConfig.Storage.Pool != "tenant-data" || created.TargetConfig.Network == nil || created.TargetConfig.Network.Name != "private-apps" {
		t.Fatalf("created intent: %+v err=%v", created, err)
	}
	if strings.Contains(string(created.ConfigSchema), "vaultix://") || strings.Contains(string(created.ConfigSchema), "sops://") {
		t.Fatalf("pinned config schema leaked a secret reference: %s", created.ConfigSchema)
	}

	replayed := httptest.NewRecorder()
	a.createDeployment(replayed, deploymentRequest(http.MethodPost, tenantID, "", body, createKey, ""))
	if replayed.Code != rec.Code || replayed.Body.String() != rec.Body.String() || replayed.Header().Get("Idempotent-Replayed") != "true" || replayed.Header().Get("ETag") != `"1"` {
		t.Fatalf("create replay differs: %d %v %s", replayed.Code, replayed.Header(), replayed.Body)
	}
	if openedDrivers != 3 {
		t.Fatalf("idempotent replay should use its stored response without another provider read; opened=%d", openedDrivers)
	}

	updateBody := `{"desired_state":"present","config":{"password":"vaultix://production/services/postgres/password"}}`
	updated := httptest.NewRecorder()
	a.updateDeployment(updated, deploymentRequest(http.MethodPatch, tenantID, created.ID, updateBody, uuid.NewString(), `"1"`))
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") != `"2"` {
		t.Fatalf("update response: %d %v %s", updated.Code, updated.Header(), updated.Body)
	}
	movePool := httptest.NewRecorder()
	a.updateDeployment(movePool, deploymentRequest(http.MethodPatch, tenantID, created.ID,
		`{"desired_state":"present","target_config":{"storage":{"pool":"other-pool"},"network":{"name":"private-apps"}}}`, uuid.NewString(), `"2"`))
	if movePool.Code != http.StatusUnprocessableEntity {
		t.Fatalf("in-place storage pool change must be rejected: %d %s", movePool.Code, movePool.Body)
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

type deploymentStorageInventoryDriver struct{ hv.Driver }

func (deploymentStorageInventoryDriver) Kind() string { return "incus" }
func (deploymentStorageInventoryDriver) Storage(context.Context) ([]hv.Volume, error) {
	return []hv.Volume{{Name: "tenant-data", Type: "zfs"}, {Name: "fast", Type: "btrfs"}}, nil
}
func (deploymentStorageInventoryDriver) Networks(context.Context) ([]hv.Network, error) {
	return []hv.Network{{Name: "private-apps", Type: "bridge"}, {Name: "tenant-ovn", Type: "ovn"}, {Name: "uplink0", Type: "physical"}}, nil
}

type deploymentWrongKindInventoryDriver struct{ hv.Driver }

func (deploymentWrongKindInventoryDriver) Kind() string { return string(deployments.ProviderProxmox) }

func TestDeploymentTargetStorageIsTenantAssignmentScoped(t *testing.T) {
	a, tenantID, targetID := deploymentRouteAPI(t)
	a.hvServers = &memHV{items: []servers.Endpoint{{ID: targetID, Kind: "incus"}}}
	opened := 0
	a.openHV = func(servers.Endpoint) (hv.Driver, error) {
		opened++
		return deploymentStorageInventoryDriver{}, nil
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("tenantId", tenantID)
	request.SetPathValue("systemId", targetID)
	rec := httptest.NewRecorder()
	a.listDeploymentTargetStorage(rec, request)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"tenant-data"`) || opened != 1 {
		t.Fatalf("assigned target storage: status=%d opened=%d body=%s", rec.Code, opened, rec.Body)
	}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("tenantId", tenantID)
	request.SetPathValue("systemId", uuid.NewString())
	rec = httptest.NewRecorder()
	a.listDeploymentTargetStorage(rec, request)
	if rec.Code != http.StatusNotFound || opened != 1 {
		t.Fatalf("unassigned target storage must not reach a provider: status=%d opened=%d body=%s", rec.Code, opened, rec.Body)
	}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("tenantId", tenantID)
	request.SetPathValue("systemId", targetID)
	rec = httptest.NewRecorder()
	a.listDeploymentTargetNetworks(rec, request)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"private-apps"`) || !strings.Contains(rec.Body.String(), `"name":"tenant-ovn"`) || strings.Contains(rec.Body.String(), `"name":"uplink0"`) || opened != 2 {
		t.Fatalf("assigned managed target networks: status=%d opened=%d body=%s", rec.Code, opened, rec.Body)
	}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("tenantId", tenantID)
	request.SetPathValue("systemId", uuid.NewString())
	rec = httptest.NewRecorder()
	a.listDeploymentTargetNetworks(rec, request)
	if rec.Code != http.StatusNotFound || opened != 2 {
		t.Fatalf("unassigned target networks must not reach a provider: status=%d opened=%d body=%s", rec.Code, opened, rec.Body)
	}
	a.openHV = func(servers.Endpoint) (hv.Driver, error) { return deploymentWrongKindInventoryDriver{}, nil }
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("tenantId", tenantID)
	request.SetPathValue("systemId", targetID)
	rec = httptest.NewRecorder()
	a.listDeploymentTargetNetworks(rec, request)
	if rec.Code != http.StatusServiceUnavailable || opened != 2 {
		t.Fatalf("Incus network inventory must reject a mismatched provider driver before access: status=%d opened=%d body=%s", rec.Code, opened, rec.Body)
	}
}
