package catalog

import (
	"net/http"
	"strings"
	"testing"
)

// limited mounts the same routes for a caller WITHOUT the given permissions (field checks only;
// route gates are the console's and are tested in internal/api).
func limited(t *testing.T, missing ...string) *http.ServeMux {
	testServer(t)
	h := &Handler{S: lastStore, Can: func(_ *http.Request, p string) bool {
		for _, m := range missing {
			if p == m {
				return false
			}
		}
		return true
	}}
	mux := http.NewServeMux()
	for _, rt := range h.Routes() {
		fn := rt.H
		mux.HandleFunc(rt.Pattern, func(w http.ResponseWriter, r *http.Request) { fn(w, r, "tester") })
	}
	return mux
}

func TestFieldPermissionsEnforcedByTheAPI(t *testing.T) {
	mux := limited(t, PermPricesRead, PermContactsRead, PermNotesRead)
	for _, p := range do(t, mux, "GET", "/v1/operator/catalog/plans", "", 200)["items"].([]any) {
		if _, ok := p.(map[string]any)["price_cents"]; ok {
			t.Fatal("price_cents returned without catalog.prices.read")
		}
	}
	do(t, mux, "POST", "/v1/operator/catalog/plans", `{"id":"cheap","label":"Cheap","price_cents":100}`, 403)
	do(t, mux, "PUT", "/v1/operator/catalog/plans/pro", `{"label":"Pro 2","price_cents":1}`, 200)
	var price int
	if err := lastStore.Pool.QueryRow(t.Context(), `SELECT price_cents FROM plans WHERE id='pro'`).Scan(&price); err != nil || price != 2900 {
		t.Fatalf("price must be kept on update without catalog.prices.read: %d %v", price, err)
	}
	do(t, mux, "POST", "/v1/operator/customers", `{"name":"Acme","plan_id":"pro","notes":"vip"}`, 403)
	c := do(t, mux, "POST", "/v1/operator/customers", `{"name":"Acme","plan_id":"pro"}`, 201)
	for _, k := range []string{"contacts", "endpoints", "notes"} {
		if _, ok := c[k]; ok {
			t.Fatalf("%s returned without permission: %v", k, c)
		}
	}
	id := c["id"].(string)
	if _, err := lastStore.Pool.Exec(t.Context(), `UPDATE tenants SET contacts='[{"name":"Ann"}]', notes='vip' WHERE id::text=$1`, id); err != nil {
		t.Fatal(err)
	}
	do(t, mux, "PUT", "/v1/operator/customers/"+id, `{"name":"Acme Inc","plan_id":"pro","contacts":[],"notes":""}`, 200)
	var contacts, notes string
	_ = lastStore.Pool.QueryRow(t.Context(), `SELECT contacts::text, notes FROM tenants WHERE id::text=$1`, id).Scan(&contacts, &notes)
	if !strings.Contains(contacts, "Ann") || notes != "vip" {
		t.Fatalf("hidden fields blanked by an update: %s %q", contacts, notes)
	}
	got := do(t, testMuxFor(lastStore), "GET", "/v1/operator/customers/"+id, "", 200)
	if got["notes"] != "vip" {
		t.Fatalf("a caller with every permission sees notes: %v", got)
	}
}
