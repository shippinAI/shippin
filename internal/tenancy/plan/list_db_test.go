package plan_test

import "testing"

// TestList reads the seeded plans in rank order, each with its own services, and hides prices from a
// caller without catalog.prices.read.
func TestList(t *testing.T) {
	h := serve(t, true)
	plans := h.Call("GET", base, "", 200)["items"].([]any)
	if len(plans) != 13 { // 011 canonical (5, includes founder) + 014 ladder except the founder row (8); 070 moved 069's tiers to coverage
		t.Fatalf("seed plans: %d", len(plans))
	}
	last := -1 << 31
	for _, raw := range plans {
		p := raw.(map[string]any)
		if r := int(p["rank"].(float64)); r < last {
			t.Fatalf("plans not in rank order: %v", plans)
		} else {
			last = r
		}
		if _, ok := p["price_cents"]; !ok {
			t.Fatalf("price hidden from a caller who may see it: %v", p)
		}
		for _, s := range p["services"].([]any) {
			if s.(map[string]any)["plan_id"] != p["id"] {
				t.Fatalf("a service under the wrong plan: %v", p)
			}
		}
		if p["id"] == "pro" && (p["price_cents"] != 2900.0 || len(p["services"].([]any)) == 0 || p["coverage"] == "") {
			t.Fatalf("pro: %v", p)
		}
	}
	if len(h.Audits) != 0 || len(h.Events) != 0 {
		t.Fatalf("a read was audited or emitted: %q %v", h.Audits, h.Events)
	}

	blind := serve(t, false)
	for _, p := range blind.Call("GET", base, "", 200)["items"].([]any) {
		if _, ok := p.(map[string]any)["price_cents"]; ok {
			t.Fatalf("price_cents returned without catalog.prices.read: %v", p)
		}
	}
}
