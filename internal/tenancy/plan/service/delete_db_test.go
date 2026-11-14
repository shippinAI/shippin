package service_test

import "testing"

// TestDelete stops a plan from including a service, and answers 404 when it doesn't.
func TestDelete(t *testing.T) {
	h := serve(t)
	cell := plans + "/pro/services/vaultix"
	h.Call("DELETE", cell, ``, 400) // no reason
	h.Call("DELETE", cell, `{"reason":"vaultix leaves pro"}`, 204)
	for _, s := range h.Call("GET", plans+"/pro", "", 200)["services"].([]any) {
		if s.(map[string]any)["service_id"] == "vaultix" {
			t.Fatal("the service is still in the plan")
		}
	}
	h.Call("DELETE", cell, `{"reason":"already removed"}`, 404)
	h.Call("DELETE", plans+"/nope/services/vaultix", `{"reason":"missing plan"}`, 404)
	if len(h.Audits) != 1 || h.Audits[0] != "shippin.tenancy.plan.service.delete planId=pro serviceId=vaultix · vaultix leaves pro" {
		t.Fatalf("audits: %q", h.Audits)
	}
	if len(h.Events) != 1 || h.Events[0].ID != "shippin.tenancy.plan.service.deleted" || h.Events[0].Subject["serviceId"] != "vaultix" {
		t.Fatalf("events: %+v", h.Events)
	}
}
