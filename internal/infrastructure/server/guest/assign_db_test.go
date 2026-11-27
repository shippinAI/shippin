package guest_test

import (
	"context"
	"testing"

	"shippin.cloud/console/internal/infrastructure/servers"
)

const hostID = "019a0000-0000-7000-8000-0000000000a1"

// A guest that was running before the panel managed its host is placed under a customer: the customer
// of an email address, made when that person has none and reused when they do, or an existing one by
// id. The hypervisor is never touched, and unassigning leaves the guest unassigned again.
func TestAGuestIsAssignedToACustomer(t *testing.T) {
	h, hosts := serve(t, false)
	hosts.alias = hostID
	ctx := context.Background()
	if _, err := h.Pool.Exec(ctx, `INSERT INTO servers(id, tenant_id, slug, label, site, state, created_by) VALUES($1,$2,'host','Host','main','planned','test')`, hostID, servers.SystemTenantID); err != nil {
		t.Fatal(err)
	}
	at := base + "/" + hostID + "/guests"

	// Nothing is assigned to begin with, and an assignment says why and names exactly one customer.
	if g := h.Call("GET", at+"/web", "", 200); g["owner"] != nil {
		t.Fatalf("a new host's guest has an owner: %v", g)
	}
	h.Call("POST", at+"/web/assign", `{"email":"ada@example.invalid"}`, 400)
	h.Call("POST", at+"/web/assign", `{"reason":"placing it"}`, 400)
	h.Call("POST", at+"/web/assign", `{"reason":"placing it","email":"not an address"}`, 400)
	h.Call("POST", at+"/web/assign", `{"reason":"placing it","email":"ada@example.invalid","customer_id":"`+hostID+`"}`, 400)
	h.Call("POST", at+"/gone/assign", `{"reason":"placing it","email":"ada@example.invalid"}`, 404)

	// An email address nobody has makes the person and their customer, named after the address.
	g := h.Call("POST", at+"/web/assign", `{"reason":"it is Ada's","email":"Ada@Example.invalid"}`, 200)
	owner, _ := g["owner"].(map[string]any)
	if owner == nil || owner["name"] != "ada@example.invalid" || owner["customer_id"] == "" {
		t.Fatalf("assigned: %v", g)
	}
	customer := owner["customer_id"].(string)
	var verified, owns bool
	var status string
	if err := h.Pool.QueryRow(ctx, `SELECT u.email_verified, t.status,
	  EXISTS(SELECT 1 FROM identity_members m WHERE m.org_id::text=t.idp_org_id AND m.user_id=u.id AND m.role='owner')
	  FROM identity_users u, tenants t WHERE u.email='ada@example.invalid' AND t.id::text=$1`, customer).Scan(&verified, &status, &owns); err != nil || verified || status != "draft" || !owns {
		t.Fatalf("the person and their customer: verified=%v status=%q owns=%v err=%v", verified, status, owns, err)
	}

	// The same address again uses the customer the person already has, here for the panel's own guest.
	g = h.Call("POST", at+"/panel-vm/assign", `{"reason":"also Ada's","email":"ada@example.invalid","name":"Ignored"}`, 200)
	if o := g["owner"].(map[string]any); o["customer_id"] != customer || g["self"] != true {
		t.Fatalf("a second customer was made: %v", g)
	}
	var customers int
	if err := h.Pool.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE kind='customer'`).Scan(&customers); err != nil || customers != 1 {
		t.Fatalf("%d customers, want 1 (%v)", customers, err)
	}

	// The list says who owns what, and an existing customer is named by id.
	for _, item := range h.Call("GET", at, "", 200)["items"].([]any) {
		if o, _ := item.(map[string]any)["owner"].(map[string]any); o == nil || o["customer_id"] != customer {
			t.Fatalf("list: %v", item)
		}
	}
	h.Call("POST", at+"/web/assign", `{"reason":"moving it","customer_id":"`+hostID+`"}`, 400)
	h.Call("POST", at+"/web/assign", `{"reason":"moving it","customer_id":"`+customer+`"}`, 200)

	if g := h.Call("POST", at+"/web/unassign", `{"reason":"not theirs after all"}`, 200); g["owner"] != nil {
		t.Fatalf("unassigned: %v", g)
	}
	if g := h.Call("GET", at+"/web", "", 200); g["owner"] != nil {
		t.Fatalf("still assigned: %v", g)
	}
	if len(hosts.hv.calls) != 0 {
		t.Fatalf("assigning touched the hypervisor: %v", hosts.hv.calls)
	}
	if len(h.Audits) != 4 {
		t.Fatalf("%d changes audited, want 4: %v", len(h.Audits), h.Audits)
	}
}
