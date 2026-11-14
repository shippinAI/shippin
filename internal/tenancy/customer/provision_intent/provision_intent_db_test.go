package provision_intent_test

import (
	"context"
	"encoding/json"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

func TestProvisionIntentActions(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.tenancy.customer.", mcpltest.As{Actor: "tester", Perms: func(string) bool { return true }})
	id := h.Call("POST", "/v1/operator/tenancy/customers", `{"name":"Acme","plan_id":"pro","reason":"customer to provision"}`, 201)["id"].(string)
	path := "/v1/operator/tenancy/customers/" + id + "/provision-intents"
	const nobody = "/v1/operator/tenancy/customers/00000000-0000-0000-0000-000000000000/provision-intents"

	h.Call("GET", nobody, "", 404)
	h.Call("POST", nobody, `{"reason":"missing customer test"}`, 404)
	h.Call("POST", path, `{"note":"first"}`, 400) // no reason
	h.Call("POST", path, `{"note":"first","status":"applied","reason":"status is not the caller's"}`, 400)
	if n := len(h.Call("GET", path, "", 200)["items"].([]any)); n != 0 {
		t.Fatalf("a new customer has %d intents", n)
	}

	in := h.Call("POST", path, `{"note":" first ","reason":"config reviewed"}`, 201)
	if in["status"] != "pending" || in["created_by"] != "tester" || in["tenant_id"] != id || in["ticket_ref"] != "" {
		t.Fatalf("intent: %v", in)
	}
	services := map[any]bool{}
	for _, s := range in["enabled_services"].([]any) {
		services[s] = true
	}
	if !services["vaultix"] {
		t.Fatalf("the intent snapshots the enabled services: %v", in["enabled_services"])
	}
	var raw []byte
	if err := h.Pool.QueryRow(context.Background(), `SELECT desired FROM provision_intents WHERE id::text=$1`, in["id"]).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var desired map[string]any
	if err := json.Unmarshal(raw, &desired); err != nil || desired["plan_id"] != "pro" || desired["note"] != "first" ||
		len(desired["services"].([]any)) == 0 || len(desired["settings"].([]any)) == 0 {
		t.Fatalf("desired state: %v %v", desired, err)
	}
	second := h.Call("POST", path, `{"reason":"recorded again without a note"}`, 201)
	items := h.Call("GET", path, "", 200)["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["id"] != second["id"] {
		t.Fatalf("newest first: %v", items)
	}
	if c := h.Call("GET", "/v1/operator/tenancy/customers/"+id, "", 200); c["last_intent"] != "pending" {
		t.Fatalf("the customer shows its latest intent: %v", c["last_intent"])
	}

	if len(h.Audits) != 3 || h.Audits[1] != "shippin.tenancy.customer.provision_intent.create customerId="+id+" id="+in["id"].(string)+" · config reviewed" {
		t.Fatalf("audits: %q", h.Audits)
	}
	if len(h.Events) != 3 || h.Events[1].ID != "shippin.tenancy.customer.provision_intent.created" || h.Events[1].Perm != "provision.intent.create" ||
		h.Events[1].Subject["customerId"] != id || h.Events[1].Subject["id"] != in["id"] {
		t.Fatalf("events: %+v", h.Events)
	}
}
