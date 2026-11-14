package plan_test

import "testing"

// TestDelete refuses a plan a customer is on, removes it with its services once none is, and answers 404
// after.
func TestDelete(t *testing.T) {
	h := serve(t, true)
	h.Call("POST", base, `{"id":"scratch","label":"Scratch","reason":"a plan to delete"}`, 201)
	h.Call("PUT", base+"/scratch/services/vaultix", `{"reason":"include vaultix"}`, 200)
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO tenants(id,idp_org_id,name,plan) VALUES(gen_random_uuid(),'org-acme','Acme','scratch')`); err != nil {
		t.Fatal(err)
	}
	h.Call("DELETE", base+"/scratch", `{"reason":"customer still on it"}`, 409)
	if _, err := h.Pool.Exec(t.Context(), `UPDATE tenants SET plan='pro' WHERE idp_org_id='org-acme'`); err != nil {
		t.Fatal(err)
	}
	h.Call("DELETE", base+"/scratch", ``, 400) // no reason
	h.Call("DELETE", base+"/scratch", `{"reason":"no longer sold"}`, 204)
	h.Call("GET", base+"/scratch", "", 404)
	h.Call("DELETE", base+"/scratch", `{"reason":"already gone"}`, 404)
	var cells int
	if err := h.Pool.QueryRow(t.Context(), `SELECT count(*) FROM plan_services WHERE plan_id='scratch'`).Scan(&cells); err != nil || cells != 0 {
		t.Fatalf("the plan's services stayed: %d %v", cells, err)
	}
	if n := len(h.Audits); n != 3 || h.Audits[2] != "shippin.tenancy.plan.delete planId=scratch · no longer sold" {
		t.Fatalf("audits: %q", h.Audits)
	}
	if e := h.Events[len(h.Events)-1]; len(h.Events) != 3 || e.ID != "shippin.tenancy.plan.deleted" || e.Subject["planId"] != "scratch" {
		t.Fatalf("events: %+v", h.Events)
	}
}
