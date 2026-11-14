package plan_test

import "testing"

// TestUpdate changes a plan, keeps a price it wasn't given or may not set, and refuses a missing plan.
func TestUpdate(t *testing.T) {
	h := serve(t, true)
	h.Call("PUT", base+"/pro", `{"label":"Pro 2"}`, 400) // no reason
	got := h.Call("PUT", base+"/pro", `{"label":"Pro 2","price_cents":900,"coverage":"managed","active":false,"reason":"reprice pro"}`, 200)
	if got["label"] != "Pro 2" || got["price_cents"] != 900.0 || got["coverage"] != "managed" || got["active"] != false || got["public"] != true {
		t.Fatalf("update: %v", got)
	}
	if price(t, h, "pro") != 900 {
		t.Fatal("price not stored")
	}
	kept := h.Call("PUT", base+"/pro", `{"label":"Pro 3","coverage":"managed","reason":"rename without a price"}`, 200)
	if kept["price_cents"] != 900.0 || kept["active"] != true {
		t.Fatalf("a price left out is kept, and active defaults to true: %v", kept)
	}
	h.Call("PUT", base+"/pro", `{"id":"pro","label":"x","reason":"id belongs in the path"}`, 400)
	h.Call("PUT", base+"/pro", `{"label":"","reason":"no label"}`, 400)
	h.Call("PUT", base+"/pro", `{"label":"x","coverage":"nope","reason":"unknown coverage"}`, 400)
	h.Call("PUT", base+"/nope", `{"label":"x","reason":"missing plan test"}`, 404)
	if len(h.Audits) != 2 || h.Audits[0] != "shippin.tenancy.plan.update planId=pro · reprice pro" {
		t.Fatalf("audits: %q", h.Audits)
	}
	if len(h.Events) != 2 || h.Events[1].ID != "shippin.tenancy.plan.updated" || h.Events[1].Subject["planId"] != "pro" {
		t.Fatalf("events: %+v", h.Events)
	}

	// Without catalog.prices.read the stored price is kept, whatever is sent.
	blind := serve(t, false)
	blind.Call("PUT", base+"/pro", `{"label":"Pro 2","price_cents":1,"reason":"try to reprice"}`, 200)
	if p := price(t, blind, "pro"); p != 2900 {
		t.Fatalf("price must be kept on update without catalog.prices.read: %d", p)
	}
}
