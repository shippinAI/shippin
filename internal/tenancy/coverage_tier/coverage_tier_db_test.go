package coverage_tier_test

import (
	"context"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

// serve mounts this entity's generated actions for a caller whose price permission is prices.
func serve(t *testing.T, prices bool) *mcpltest.Harness {
	return mcpltest.Serve(t, actions.All, "shippin.tenancy.coverage_tier.", mcpltest.As{Actor: "tester",
		Perms: func(p string) bool { return prices || p != "catalog.prices.read" }})
}

const base = "/v1/operator/tenancy/coverage-tiers"

func TestCoverageTierActions(t *testing.T) {
	h := serve(t, true)
	tiers := h.Call("GET", base, "", 200)["items"].([]any)
	if len(tiers) != 3 || tiers[0].(map[string]any)["id"] != "foundational" || tiers[2].(map[string]any)["sign_in"] != "identity_provider" {
		t.Fatalf("seed coverage: %v", tiers)
	}
	enterprise := `"label":"Enterprise","support_hours":"always","channels":["email","video"],"critical_minutes":10,"uptime_target":99.99,"operations":"managed","consulting":"included"`
	h.Call("POST", base, `{"id":"enterprise",`+enterprise+`}`, 400) // no reason
	created := h.Call("POST", base, `{"id":"enterprise",`+enterprise+`,"reason":"sell an enterprise tier"}`, 201)
	if created["public"] != true || created["sign_in"] != "password" || created["price_cents"] != 0.0 {
		t.Fatalf("defaults not applied: %v", created)
	}
	h.Call("POST", base, `{"id":"enterprise","label":"Again","reason":"duplicate id test"}`, 409)
	h.Call("POST", base, `{"id":"Bad Id","label":"x","reason":"bad id test"}`, 400)
	h.Call("PUT", base+"/enterprise", `{"label":"Enterprise","colour":"blue","reason":"unknown field test"}`, 400)
	h.Call("PUT", base+"/enterprise", `{"id":"enterprise","label":"Enterprise","reason":"id belongs in the path"}`, 400)
	for _, bad := range []string{`"support_hours":"weekends"`, `"channels":["fax"]`, `"critical_minutes":0`, `"uptime_target":100`, `"remote_operations":"free"`} {
		h.Call("PUT", base+"/enterprise", `{"label":"Enterprise",`+bad+`,"reason":"validation test"}`, 400)
	}
	got := h.Call("PUT", base+"/enterprise", `{"label":"Enterprise+","sign_in":"second_factor","price_cents":150000,"spend_percent":10,"reason":"price the tier"}`, 200)
	if got["label"] != "Enterprise+" || got["spend_percent"] != 10.0 || got["critical_minutes"] != nil {
		t.Fatalf("coverage update: %v", got)
	}
	h.Call("PUT", base+"/nope", `{"label":"x","reason":"missing tier test"}`, 404)
	h.Call("DELETE", base+"/foundational", `{"reason":"tier a plan includes"}`, 409)
	h.Call("DELETE", base+"/enterprise", ``, 400) // no reason
	h.Call("DELETE", base+"/enterprise", `{"reason":"no longer sold"}`, 204)
	h.Call("GET", base+"/enterprise", "", 404)
	// Only changes that happened are audited, each with its reason; refused attempts are not.
	if len(h.Audits) != 3 || h.Audits[0] != "shippin.tenancy.coverage_tier.create id=enterprise · sell an enterprise tier" ||
		h.Audits[1] != "shippin.tenancy.coverage_tier.update coverageTierId=enterprise · price the tier" ||
		h.Audits[2] != "shippin.tenancy.coverage_tier.delete coverageTierId=enterprise · no longer sold" {
		t.Fatalf("audits: %q", h.Audits)
	}
	if len(h.Events) != 3 || h.Events[0].ID != "shippin.tenancy.coverage_tier.created" || h.Events[0].Subject["id"] != "enterprise" ||
		h.Events[2].ID != "shippin.tenancy.coverage_tier.deleted" || h.Events[2].Access != "staff" || h.Events[2].Perm != "catalog.plans.write" {
		t.Fatalf("events: %+v", h.Events)
	}

	// Without catalog.prices.read the price is hidden, can't be set, and is kept on update.
	blind := serve(t, false)
	for _, tier := range blind.Call("GET", base, "", 200)["items"].([]any) {
		if _, ok := tier.(map[string]any)["price_cents"]; ok {
			t.Fatalf("price shown without catalog.prices.read: %v", tier)
		}
	}
	blind.Call("POST", base, `{"id":"priced","label":"Priced","price_cents":100,"reason":"price without permission"}`, 403)
	blind.Call("POST", base, `{"id":"plain","label":"Plain","reason":"tier without a price"}`, 201)
	blind.Call("PUT", base+"/plain", `{"label":"Plain+","price_cents":999,"reason":"try to price it"}`, 200)
	var cents int
	if err := blind.Pool.QueryRow(context.Background(), `SELECT price_cents FROM coverage_tiers WHERE id='plain'`).Scan(&cents); err != nil || cents != 0 {
		t.Fatalf("a caller without prices changed the price: %d %v", cents, err)
	}
}
