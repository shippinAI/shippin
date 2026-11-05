package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"shippin.cloud/console/internal/db"
)

// testServer opens SHIPPIN_TEST_DATABASE_URL (disposable: the schema is dropped and re-migrated,
// so the seed migration's plans and services are present) or skips.
func testServer(t *testing.T) *http.ServeMux {
	t.Helper()
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	d.Pool.Close()
	if d, err = db.Open(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	lastStore = &Store{Pool: d.Pool}
	return testMuxFor(lastStore)
}

// lastStore is the store of the most recent testServer (field-permission tests reuse it).
var lastStore *Store

func testMuxFor(s *Store) *http.ServeMux {
	h := &Handler{S: s}
	mux := http.NewServeMux()
	for _, rt := range h.Routes() {
		fn := rt.H
		mux.HandleFunc(rt.Pattern, func(w http.ResponseWriter, r *http.Request) { fn(w, r, "tester") })
	}
	return mux
}

func do(t *testing.T, mux *http.ServeMux, method, path, body string, want int) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	if rec.Code != want {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func TestCatalogAPIFlow(t *testing.T) {
	mux := testServer(t)
	plans := do(t, mux, "GET", "/v1/operator/catalog/plans", "", 200)["items"].([]any)
	if len(plans) != 13 { // 011 canonical (5, includes founder) + 014 ladder except the founder row already inserted (8)
		t.Fatalf("seed plans: %d", len(plans))
	}
	do(t, mux, "POST", "/v1/operator/catalog/plans", `{"id":"scratch","label":"Scratch","rank":5}`, 201)
	do(t, mux, "POST", "/v1/operator/catalog/plans", `{"id":"scratch","label":"Dup"}`, 409)
	do(t, mux, "POST", "/v1/operator/catalog/plans", `{"id":"Bad Id","label":"x"}`, 400)
	do(t, mux, "PUT", "/v1/operator/catalog/plans/scratch", `{"label":"Scratch+","price_cents":900}`, 200)
	do(t, mux, "PUT", "/v1/operator/catalog/plan-services/scratch/chatmaton", `{"enabled":true,"lock":"bounded","limits":{"seats":3}}`, 200)
	do(t, mux, "PUT", "/v1/operator/catalog/plan-services/starter/chatmaton", `{"lock":"weird"}`, 400)
	do(t, mux, "POST", "/v1/operator/catalog/services", `{"id":"backup","label":"Backups","default_config":{"days":7}}`, 201)
	do(t, mux, "GET", "/v1/operator/catalog/services/backup", "", 200)
	source := `"deployment_source":{"kind":"oci","reference":"registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","credential_ref":"vaultix://production/runtime/registry-token"}`
	runtime := `"runtime_contract":{"version":1,"kind":"incus_oci_container","providers":["incus"]}`
	configSchema := `"config_schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"port":{"type":"integer","minimum":1,"maximum":65535},"password":{"type":"string","format":"shippin-secret-ref"}},"required":["password"],"additionalProperties":false},"default_config":{"port":5432,"password":"vaultix://production/postgres/password"}`
	do(t, mux, "POST", "/v1/operator/catalog/services", `{"id":"unsafe-schema","label":"Unsafe","kind":"database",`+source+`,"config_schema":{"type":"object"}}`, 400)
	metadataOnly := do(t, mux, "POST", "/v1/operator/catalog/services", `{"id":"metadata-only","label":"Metadata only","kind":"database",`+configSchema+`,`+source+`}`, 201)
	if metadataOnly["runtime_contract"] != nil {
		t.Fatalf("metadata-only service unexpectedly declares a runtime: %v", metadataOnly["runtime_contract"])
	}
	if _, err := lastStore.Pool.Exec(t.Context(), `INSERT INTO catalog_services(id,label,kind,deployment_source,runtime_contract)
		VALUES('invalid-runtime','Invalid','app',$1::jsonb,$2::jsonb)`,
		`{"kind":"github","reference":"https://github.com/acme/app","commit":"0123456789abcdef0123456789abcdef01234567"}`,
		`{"version":1,"kind":"incus_oci_container","providers":["incus"]}`); err == nil {
		t.Fatal("database accepted an Incus OCI declaration for a Git source")
	}
	do(t, mux, "POST", "/v1/operator/catalog/services", `{"id":"unsupported-runtime","label":"Unsafe","kind":"database",`+configSchema+`,`+source+`,"runtime_contract":{"version":1,"kind":"proxmox_cloud_init","providers":["proxmox"]}}`, 400)
	do(t, mux, "POST", "/v1/operator/catalog/services", `{"id":"plaintext-secret","label":"Unsafe","kind":"database",`+source+`,`+configSchema[:strings.Index(configSchema, `,"default_config"`)]+`,"default_config":{"port":5432,"password":"plaintext-password"}}`, 400)
	configured := do(t, mux, "POST", "/v1/operator/catalog/services", `{"id":"postgres","label":"Postgres","kind":"database",`+configSchema+`,`+source+`,`+runtime+`}`, 201)
	if configured["deployment_source"].(map[string]any)["kind"] != "oci" {
		t.Fatalf("source not returned after create: %v", configured["deployment_source"])
	}
	if configured["runtime_contract"].(map[string]any)["kind"] != "incus_oci_container" {
		t.Fatalf("runtime contract not returned after create: %v", configured["runtime_contract"])
	}
	loaded := do(t, mux, "GET", "/v1/operator/catalog/services/postgres", "", 200)
	if loaded["deployment_source"].(map[string]any)["credential_ref"] != "vaultix://production/runtime/registry-token" {
		t.Fatalf("source not persisted: %v", loaded["deployment_source"])
	}
	do(t, mux, "PUT", "/v1/operator/catalog/services/postgres", `{"label":"Postgres","deployment_source":{"kind":"github","reference":"https://github.com/acme/postgres","commit":"main"}}`, 400)
	do(t, mux, "PUT", "/v1/operator/catalog/services/postgres", `{"label":"Postgres","kind":"database",`+configSchema+`,"deployment_source":{"kind":"oci","reference":"registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},`+runtime+`}`, 200)
	loaded = do(t, mux, "GET", "/v1/operator/catalog/services/postgres", "", 200)
	if loaded["deployment_source"].(map[string]any)["credential_ref"] != nil {
		t.Fatalf("update did not replace the source contract: %v", loaded["deployment_source"])
	}

	c := do(t, mux, "POST", "/v1/operator/customers", `{"name":"Acme","plan_id":"scratch","contacts":[{"name":"Ann","email":"ann@acme.test","role":"owner"}]}`, 201)
	id := c["id"].(string)
	if c["status"] != "draft" || c["idp_linked"] != false {
		t.Fatalf("new customer: %v", c)
	}
	do(t, mux, "POST", "/v1/operator/customers", `{"name":"X","plan_id":"nope"}`, 400)
	do(t, mux, "DELETE", "/v1/operator/catalog/plans/scratch", "", 409)
	do(t, mux, "PUT", "/v1/operator/customers/"+id+"/services/chatmaton", `{"overrides":{"seats":99}}`, 200)
	do(t, mux, "PUT", "/v1/operator/catalog/defaults", `{"layer":"tenant","ref":"`+id+`","key":"workspace.idle_shutdown_min","value":9999}`, 200)
	do(t, mux, "PUT", "/v1/operator/catalog/defaults", `{"layer":"tenant","key":"x","value":1}`, 400)

	eff := do(t, mux, "GET", "/v1/operator/customers/"+id+"/effective-config", "", 200)
	vals := map[string]map[string]any{}
	for _, s := range append(eff["settings"].([]any), eff["services"].([]any)...) {
		m := s.(map[string]any)
		vals[m["key"].(string)] = m
	}
	if v := vals["svc.chatmaton.seats"]; v["value"].(float64) != 3 || v["source"] != "tenant" {
		t.Fatalf("seats should clamp to plan max 3: %v", v)
	}
	if v := vals["workspace.idle_shutdown_min"]; v["value"].(float64) != 480 {
		t.Fatalf("idle should clamp to system max 480: %v", v)
	}
	if v := vals["policy.sandbox"]; v["lock"] != "locked" || v["source"] != "system" {
		t.Fatalf("sandbox: %v", v)
	}
	if vals["svc.vaultix.enabled"]["value"] != true {
		t.Fatal("vaultix is required but scratch has no row: plan lock should not override system lock")
	}

	in := do(t, mux, "POST", "/v1/operator/customers/"+id+"/provision-intent", `{"note":"first"}`, 202)
	if in["status"] != "pending" || in["created_by"] != "tester" {
		t.Fatalf("intent: %v", in)
	}
	if n := len(do(t, mux, "GET", "/v1/operator/customers/"+id+"/provision-intents", "", 200)["items"].([]any)); n != 1 {
		t.Fatalf("intents: %d", n)
	}
	do(t, mux, "GET", "/v1/operator/customers/00000000-0000-0000-0000-000000000000/effective-config", "", 404)
	do(t, mux, "PUT", "/v1/operator/customers/"+id, `{"name":"Acme Inc","plan_id":"pro"}`, 200)
	do(t, mux, "DELETE", "/v1/operator/customers/"+id, "", 204)
	do(t, mux, "DELETE", "/v1/operator/catalog/plans/scratch", "", 204)
}
