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
	// Plans and their services are tenancy/plan actions now (internal/tenancy/plan); this flow only
	// needs a plan that includes chatmaton with at most 3 seats.
	if _, err := lastStore.Pool.Exec(t.Context(), `INSERT INTO plans(id,label,rank,coverage) VALUES('scratch','Scratch',5,'foundational');
		INSERT INTO plan_services(plan_id,service_id,enabled,lock,limits) VALUES('scratch','chatmaton',true,'bounded','{"seats":3}')`); err != nil {
		t.Fatal(err)
	}
	c := do(t, mux, "POST", "/v1/operator/customers", `{"name":"Acme","plan_id":"scratch","contacts":[{"name":"Ann","email":"ann@acme.test","role":"owner"}]}`, 201)
	id := c["id"].(string)
	if c["status"] != "draft" || c["idp_linked"] != false {
		t.Fatalf("new customer: %v", c)
	}
	do(t, mux, "POST", "/v1/operator/customers", `{"name":"X","plan_id":"nope"}`, 400)
	if c["coverage"] != nil || c["coverage_effective"] != "foundational" {
		t.Fatalf("a new customer takes its plan's coverage: %v %v", c["coverage"], c["coverage_effective"])
	}
	do(t, mux, "PUT", "/v1/operator/customers/"+id, `{"name":"Acme","plan_id":"scratch","coverage":"nope"}`, 400)
	moved := do(t, mux, "PUT", "/v1/operator/customers/"+id, `{"name":"Acme","plan_id":"scratch","coverage":"managed"}`, 200)
	if moved["coverage_effective"] != "managed" {
		t.Fatalf("coverage override: %v", moved)
	}
	back := do(t, mux, "PUT", "/v1/operator/customers/"+id, `{"name":"Acme","plan_id":"scratch","coverage":""}`, 200)
	if back["coverage"] != nil || back["coverage_effective"] != "foundational" {
		t.Fatalf("an empty coverage returns to the plan's: %v", back)
	}
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
}
