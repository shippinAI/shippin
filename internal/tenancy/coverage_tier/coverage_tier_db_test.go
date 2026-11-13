package coverage_tier_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/mcpl"
)

// serve mounts this entity's generated actions on their derived routes, for a caller whose price
// permission is prices; audits are collected.
func serve(t *testing.T, prices bool, audits *[]string) *http.ServeMux {
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
	mux := http.NewServeMux()
	for _, g := range actions.All {
		if !strings.HasPrefix(g.ID, "shippin.tenancy.coverage_tier.") {
			continue
		}
		mux.HandleFunc(g.Route, func(w http.ResponseWriter, r *http.Request) {
			g.Serve(&mcpl.Call{Ctx: r.Context(), Pool: d.Pool, Actor: "tester",
				Perms: func(p string) bool { return prices || p != "catalog.prices.read" },
				Audit: func(action, detail string) error { *audits = append(*audits, action+" "+detail); return nil }}, w, r)
		})
	}
	return mux
}

func call(t *testing.T, mux *http.ServeMux, method, path, body string, want int) map[string]any {
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

const base = "/v1/operator/tenancy/coverage-tiers"

func TestCoverageTierActions(t *testing.T) {
	var audits []string
	mux := serve(t, true, &audits)
	tiers := call(t, mux, "GET", base, "", 200)["items"].([]any)
	if len(tiers) != 3 || tiers[0].(map[string]any)["id"] != "foundational" || tiers[2].(map[string]any)["sign_in"] != "identity_provider" {
		t.Fatalf("seed coverage: %v", tiers)
	}
	enterprise := `"label":"Enterprise","support_hours":"always","channels":["email","video"],"critical_minutes":10,"uptime_target":99.99,"operations":"managed","consulting":"included"`
	call(t, mux, "POST", base, `{"id":"enterprise",`+enterprise+`}`, 400) // no reason
	created := call(t, mux, "POST", base, `{"id":"enterprise",`+enterprise+`,"reason":"sell an enterprise tier"}`, 201)
	if created["public"] != true || created["sign_in"] != "password" || created["price_cents"] != 0.0 {
		t.Fatalf("defaults not applied: %v", created)
	}
	call(t, mux, "POST", base, `{"id":"enterprise","label":"Again","reason":"duplicate id test"}`, 409)
	call(t, mux, "POST", base, `{"id":"Bad Id","label":"x","reason":"bad id test"}`, 400)
	call(t, mux, "PUT", base+"/enterprise", `{"label":"Enterprise","colour":"blue","reason":"unknown field test"}`, 400)
	call(t, mux, "PUT", base+"/enterprise", `{"id":"enterprise","label":"Enterprise","reason":"id belongs in the path"}`, 400)
	for _, bad := range []string{`"support_hours":"weekends"`, `"channels":["fax"]`, `"critical_minutes":0`, `"uptime_target":100`, `"remote_operations":"free"`} {
		call(t, mux, "PUT", base+"/enterprise", `{"label":"Enterprise",`+bad+`,"reason":"validation test"}`, 400)
	}
	got := call(t, mux, "PUT", base+"/enterprise", `{"label":"Enterprise+","sign_in":"second_factor","price_cents":150000,"spend_percent":10,"reason":"price the tier"}`, 200)
	if got["label"] != "Enterprise+" || got["spend_percent"] != 10.0 || got["critical_minutes"] != nil {
		t.Fatalf("coverage update: %v", got)
	}
	call(t, mux, "PUT", base+"/nope", `{"label":"x","reason":"missing tier test"}`, 404)
	call(t, mux, "DELETE", base+"/foundational", `{"reason":"tier a plan includes"}`, 409)
	call(t, mux, "DELETE", base+"/enterprise", ``, 400) // no reason
	call(t, mux, "DELETE", base+"/enterprise", `{"reason":"no longer sold"}`, 204)
	call(t, mux, "GET", base+"/enterprise", "", 404)
	// Only changes that happened are audited, each with its reason; refused attempts are not.
	if len(audits) != 3 || audits[0] != "shippin.tenancy.coverage_tier.create · sell an enterprise tier" ||
		audits[1] != "shippin.tenancy.coverage_tier.update coverageTierId=enterprise · price the tier" ||
		audits[2] != "shippin.tenancy.coverage_tier.delete coverageTierId=enterprise · no longer sold" {
		t.Fatalf("audits: %q", audits)
	}

	// Without catalog.prices.read the price is hidden, can't be set, and is kept on update.
	var noAudits []string
	blind := serve(t, false, &noAudits)
	for _, tier := range call(t, blind, "GET", base, "", 200)["items"].([]any) {
		if _, ok := tier.(map[string]any)["price_cents"]; ok {
			t.Fatalf("price shown without catalog.prices.read: %v", tier)
		}
	}
	call(t, blind, "POST", base, `{"id":"priced","label":"Priced","price_cents":100,"reason":"price without permission"}`, 403)
	call(t, blind, "POST", base, `{"id":"plain","label":"Plain","reason":"tier without a price"}`, 201)
	call(t, blind, "PUT", base+"/plain", `{"label":"Plain+","price_cents":999,"reason":"try to price it"}`, 200)
	var cents int
	if err := (func() error {
		d, err := db.Open(context.Background(), os.Getenv("SHIPPIN_TEST_DATABASE_URL"))
		if err != nil {
			return err
		}
		defer d.Pool.Close()
		return d.Pool.QueryRow(context.Background(), `SELECT price_cents FROM coverage_tiers WHERE id='plain'`).Scan(&cents)
	})(); err != nil || cents != 0 {
		t.Fatalf("a caller without prices changed the price: %d %v", cents, err)
	}
}
