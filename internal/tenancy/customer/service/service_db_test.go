package service_test

import (
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

func TestCustomerServiceActions(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.tenancy.customer.", mcpltest.As{Actor: "tester", Perms: func(string) bool { return true }})
	id := h.Call("POST", "/v1/operator/tenancy/customers", `{"name":"Acme","plan_id":"pro","reason":"customer for services"}`, 201)["id"].(string)
	svc := "/v1/operator/tenancy/customers/" + id + "/services"
	const nobody = "/v1/operator/tenancy/customers/00000000-0000-0000-0000-000000000000/services"

	if n := len(h.Call("GET", svc, "", 200)["items"].([]any)); n != 0 {
		t.Fatalf("a new customer has %d service settings", n)
	}
	h.Call("GET", nobody, "", 404)
	h.Call("PUT", nobody+"/chatmaton", `{"enabled":true,"reason":"missing customer test"}`, 404)
	h.Call("PUT", svc+"/nope", `{"enabled":true,"reason":"missing service test"}`, 404)
	h.Call("PUT", svc+"/chatmaton", `{"enabled":true}`, 400) // no reason
	h.Call("PUT", svc+"/chatmaton", `{"overrides":[1],"reason":"overrides are an object"}`, 400)
	h.Call("PUT", svc+"/chatmaton", `{"tenant_id":"`+id+`","enabled":true,"reason":"ids belong in the path"}`, 400)

	got := h.Call("PUT", svc+"/chatmaton", `{"overrides":{"seats":99},"reason":"more seats agreed"}`, 200)
	if got["tenant_id"] != id || got["service_id"] != "chatmaton" || got["enabled"] != nil || got["overrides"].(map[string]any)["seats"] != 99.0 {
		t.Fatalf("override: %v", got)
	}
	got = h.Call("PUT", svc+"/chatmaton", `{"enabled":false,"reason":"switched off for now"}`, 200)
	if got["enabled"] != false || len(got["overrides"].(map[string]any)) != 0 {
		t.Fatalf("a second update replaces the first: %v", got)
	}
	if items := h.Call("GET", svc, "", 200)["items"].([]any); len(items) != 1 || items[0].(map[string]any)["enabled"] != false {
		t.Fatalf("list: %v", items)
	}
	h.Call("DELETE", svc+"/chatmaton", `{"reason":"plan decides again"}`, 204)
	h.Call("DELETE", svc+"/chatmaton", `{"reason":"nothing left to drop"}`, 404)

	if len(h.Audits) != 4 || h.Audits[1] != "shippin.tenancy.customer.service.update customerId="+id+" serviceId=chatmaton · more seats agreed" ||
		h.Audits[3] != "shippin.tenancy.customer.service.delete customerId="+id+" serviceId=chatmaton · plan decides again" {
		t.Fatalf("audits: %q", h.Audits)
	}
	if len(h.Events) != 4 || h.Events[1].ID != "shippin.tenancy.customer.service.updated" || h.Events[1].Perm != "customers.services.write" ||
		h.Events[3].ID != "shippin.tenancy.customer.service.deleted" || h.Events[3].Subject["serviceId"] != "chatmaton" {
		t.Fatalf("events: %+v", h.Events)
	}
}
