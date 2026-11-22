package plan_test

import "testing"

// TestGet reads one plan with its services, and answers 404 for a plan that isn't there.
func TestGet(t *testing.T) {
	h := serve(t, true)
	pro := h.Call("GET", base+"/pro", "", 200)
	services := map[string]map[string]any{}
	for _, s := range pro["services"].([]any) {
		services[s.(map[string]any)["service_id"].(string)] = s.(map[string]any)
	}
	if v := services["vaultix"]; v["lock"] != "overridable" || v["enabled"] != true || v["limits"].(map[string]any)["projects"] != 10.0 {
		t.Fatalf("pro's vaultix: %v", v)
	}
	h.Call("GET", base+"/nope", "", 404)

	if _, ok := serve(t, false).Call("GET", base+"/pro", "", 200)["price_cents"]; ok {
		t.Fatal("price_cents returned without catalog.prices.read")
	}
}
