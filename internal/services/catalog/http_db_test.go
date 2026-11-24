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
	// Plans, catalog services and customers are actions now (internal/tenancy/plan,
	// internal/services/catalog_service, internal/tenancy/customer); this package still serves the
	// layered defaults. A tenant default names a customer, which is a row here.
	const acme = "01a14149-0000-7000-8000-0000000000aa"
	if _, err := lastStore.Pool.Exec(t.Context(), `INSERT INTO plans(id,label,rank,coverage) VALUES('scratch','Scratch',5,'foundational')`); err != nil {
		t.Fatal(err)
	}
	if _, err := lastStore.Pool.Exec(t.Context(), `INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES($1,$2,'Acme','scratch','draft')`, acme, "unlinked:"+acme); err != nil {
		t.Fatal(err)
	}
	do(t, mux, "PUT", "/v1/operator/catalog/defaults", `{"layer":"tenant","ref":"`+acme+`","key":"workspace.idle_shutdown_min","value":9999}`, 200)
	do(t, mux, "PUT", "/v1/operator/catalog/defaults", `{"layer":"tenant","key":"x","value":1}`, 400)
}
