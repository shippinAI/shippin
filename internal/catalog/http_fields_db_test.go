package catalog

import (
	"net/http"
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
	mux := limited(t, PermPricesRead)
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
}
