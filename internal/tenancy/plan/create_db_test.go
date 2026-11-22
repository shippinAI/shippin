package plan_test

import "testing"

// TestCreate makes a plan with its defaults, refuses what a plan may not be, and audits and announces
// only the change that happened.
func TestCreate(t *testing.T) {
	h := serve(t, true)
	h.Call("POST", base, `{"id":"scratch","label":"Scratch","rank":5}`, 400) // no reason
	created := h.Call("POST", base, `{"id":"scratch","label":"Scratch","rank":5,"reason":"a plan to test with"}`, 201)
	if created["coverage"] != "foundational" || created["public"] != true || created["active"] != true || created["price_cents"] != 0.0 ||
		len(created["services"].([]any)) != 0 {
		t.Fatalf("a plan without coverage takes the lowest tier, and is public and active at no price: %v", created)
	}
	for _, bad := range []string{
		`{"id":"nocover","label":"x","coverage":"nope"`, // unknown coverage tier
		`{"id":"Bad Id","label":"x"`,                    // not a lowercase id
		`{"id":"nolabel","label":""`,                    // no label
		`{"id":"negative","label":"x","price_cents":-1`, // negative price
		`{"id":"colour","label":"x","colour":"blue"`,    // unknown field
	} {
		h.Call("POST", base, bad+`,"reason":"validation test"}`, 400)
	}
	h.Call("POST", base, `{"id":"scratch","label":"Dup","reason":"duplicate id test"}`, 409)
	priced := h.Call("POST", base, `{"id":"hidden","label":"Hidden","price_cents":1500,"public":false,"coverage":"managed","reason":"a sales-only plan"}`, 201)
	if priced["price_cents"] != 1500.0 || priced["public"] != false || priced["coverage"] != "managed" {
		t.Fatalf("create kept what it was given: %v", priced)
	}
	if len(h.Audits) != 2 || h.Audits[0] != "shippin.tenancy.plan.create id=scratch · a plan to test with" {
		t.Fatalf("audits: %q", h.Audits)
	}
	if len(h.Events) != 2 || h.Events[0].ID != "shippin.tenancy.plan.created" || h.Events[0].Subject["id"] != "scratch" ||
		h.Events[0].Access != "staff" || h.Events[0].Perm != "catalog.plans.write" {
		t.Fatalf("events: %+v", h.Events)
	}

	// Without catalog.prices.read a price can't be set, and the plan is answered without one.
	blind := serve(t, false)
	blind.Call("POST", base, `{"id":"cheap","label":"Cheap","price_cents":100,"reason":"price without permission"}`, 403)
	plain := blind.Call("POST", base, `{"id":"plain","label":"Plain","price_cents":0,"reason":"plan without a price"}`, 201)
	if _, ok := plain["price_cents"]; ok {
		t.Fatalf("price_cents returned without catalog.prices.read: %v", plain)
	}
}
