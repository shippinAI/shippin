package customer_test

import (
	"context"
	"strings"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

const base = "/v1/operator/tenancy/customers"

// serve mounts the customer actions for a caller who holds every field permission except those in missing.
func serve(t *testing.T, missing ...string) *mcpltest.Harness {
	return mcpltest.Serve(t, actions.All, "shippin.tenancy.customer.", mcpltest.As{Actor: "tester", Perms: func(p string) bool {
		for _, m := range missing {
			if p == m {
				return false
			}
		}
		return true
	}})
}

func TestCustomerActions(t *testing.T) {
	h := serve(t)
	ctx := context.Background()
	if n := len(h.Call("GET", base, "", 200)["items"].([]any)); n != 0 {
		t.Fatalf("a fresh install lists %d customers", n)
	}
	acme := `"name":" Acme ","plan_id":"pro","contacts":[{"name":"Ann","email":"ann@acme.test","role":"owner"}],"endpoints":[{"kind":"lan","host":"10.0.0.2","note":""}]`
	h.Call("POST", base, `{`+acme+`}`, 400) // no reason
	for _, bad := range []string{
		`"name":"","plan_id":"pro"`,
		`"name":"` + strings.Repeat("x", 121) + `","plan_id":"pro"`,
		`"name":"X","plan_id":"nope"`,
		`"name":"X","plan_id":"pro","coverage":"nope"`,
		`"name":"X","plan_id":"pro","contacts":[{"name":"Ann","email":"ann.example"}]`,
		`"name":"X","plan_id":"pro","contacts":{"name":"Ann"}`,
		`"name":"X","plan_id":"pro","status":"active"`,
		`"id":"x","name":"X","plan_id":"pro"`,
	} {
		h.Call("POST", base, `{`+bad+`,"reason":"validation test"}`, 400)
	}
	c := h.Call("POST", base, `{`+acme+`,"reason":"new customer signed"}`, 201)
	id := c["id"].(string)
	var planCoverage string
	if err := h.Pool.QueryRow(ctx, `SELECT coverage FROM plans WHERE id='pro'`).Scan(&planCoverage); err != nil {
		t.Fatal(err)
	}
	if c["name"] != "Acme" || c["status"] != "draft" || c["idp_linked"] != false || c["coverage"] != nil || c["coverage_effective"] != planCoverage ||
		c["workspaces"] != 0.0 || c["last_intent"] != nil || c["notes"] != "" || len(c["contacts"].([]any)) != 1 || len(c["endpoints"].([]any)) != 1 {
		t.Fatalf("new customer: %v", c)
	}
	var org string
	if err := h.Pool.QueryRow(ctx, `SELECT idp_org_id FROM tenants WHERE id::text=$1`, id).Scan(&org); err != nil || org != "unlinked:"+id {
		t.Fatalf("a draft is not linked to a sign-in organization: %q %v", org, err)
	}

	h.Call("PUT", base+"/"+id, `{"name":"Acme","plan_id":"pro","coverage":"nope","reason":"missing tier test"}`, 400)
	h.Call("PUT", base+"/"+id, `{"id":"`+id+`","name":"Acme","plan_id":"pro","reason":"id belongs in the path"}`, 400)
	moved := h.Call("PUT", base+"/"+id, `{"name":"Acme","plan_id":"pro","coverage":"managed","notes":"vip","reason":"sold managed coverage"}`, 200)
	if moved["coverage"] != "managed" || moved["coverage_effective"] != "managed" || moved["notes"] != "vip" || len(moved["contacts"].([]any)) != 0 {
		t.Fatalf("an update replaces the fields it is given: %v", moved)
	}
	back := h.Call("PUT", base+"/"+id, `{"name":"Acme Inc","plan_id":"pro","coverage":"","reason":"back to the plan's coverage"}`, 200)
	if back["coverage"] != nil || back["coverage_effective"] != planCoverage || back["name"] != "Acme Inc" {
		t.Fatalf("an empty coverage returns to the plan's: %v", back)
	}
	for _, missing := range []string{"00000000-0000-0000-0000-000000000000", "nope"} {
		h.Call("GET", base+"/"+missing, "", 404)
		h.Call("PUT", base+"/"+missing, `{"name":"X","plan_id":"pro","reason":"missing customer test"}`, 404)
		h.Call("DELETE", base+"/"+missing, `{"reason":"missing customer test"}`, 404)
	}
	if got := h.Call("GET", base+"/"+id, "", 200); got["name"] != "Acme Inc" {
		t.Fatalf("get: %v", got)
	}
	if items := h.Call("GET", base, "", 200)["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != id {
		t.Fatalf("list: %v", items)
	}

	// Only drafts never linked to a sign-in organization are deleted here; a live organization is refused.
	live := h.Call("POST", base, `{"name":"Live","plan_id":"pro","reason":"customer to provision"}`, 201)["id"].(string)
	if _, err := h.Pool.Exec(ctx, `UPDATE tenants SET status='active', idp_org_id='org-live' WHERE id::text=$1`, live); err != nil {
		t.Fatal(err)
	}
	h.Call("DELETE", base+"/"+live, `{"reason":"live customer test"}`, 409)
	h.Call("DELETE", base+"/"+id, ``, 400) // no reason
	h.Call("DELETE", base+"/"+id, `{"reason":"signed by mistake"}`, 204)
	h.Call("GET", base+"/"+id, "", 404)

	// Only changes that happened are audited, each with its reason; refused attempts are not.
	want := []string{
		"shippin.tenancy.customer.create id=" + id + " · new customer signed",
		"shippin.tenancy.customer.update customerId=" + id + " · sold managed coverage",
		"shippin.tenancy.customer.update customerId=" + id + " · back to the plan's coverage",
		"shippin.tenancy.customer.create id=" + live + " · customer to provision",
		"shippin.tenancy.customer.delete customerId=" + id + " · signed by mistake",
	}
	if strings.Join(h.Audits, "\n") != strings.Join(want, "\n") {
		t.Fatalf("audits:\n%s", strings.Join(h.Audits, "\n"))
	}
	if len(h.Events) != 5 || h.Events[0].ID != "shippin.tenancy.customer.created" || h.Events[0].Subject["id"] != id ||
		h.Events[4].ID != "shippin.tenancy.customer.deleted" || h.Events[4].Subject["customerId"] != id ||
		h.Events[4].Access != "staff" || h.Events[4].Perm != "customers.write" || h.Events[4].Actor != "tester" {
		t.Fatalf("events: %+v", h.Events)
	}
}

// A caller without customers.contacts.read and customers.notes.read neither sees nor sets contacts,
// endpoints and notes, and an update of theirs keeps the stored ones.
func TestCustomerFieldPermissions(t *testing.T) {
	h := serve(t, "customers.contacts.read", "customers.notes.read")
	ctx := context.Background()
	for _, hidden := range []string{`"notes":"vip"`, `"contacts":[{"name":"Ann"}]`, `"endpoints":[{"kind":"lan","host":"x","note":""}]`} {
		h.Call("POST", base, `{"name":"Acme","plan_id":"pro",`+hidden+`,"reason":"hidden field test"}`, 403)
	}
	c := h.Call("POST", base, `{"name":"Acme","plan_id":"pro","contacts":[],"notes":"","reason":"empty values are not a write"}`, 201)
	for _, k := range []string{"contacts", "endpoints", "notes"} {
		if _, ok := c[k]; ok {
			t.Fatalf("%s returned without permission: %v", k, c)
		}
	}
	id := c["id"].(string)
	if _, err := h.Pool.Exec(ctx, `UPDATE tenants SET contacts='[{"name":"Ann","email":"","role":""}]', endpoints='[{"kind":"lan","host":"x","note":""}]',
		notes='vip' WHERE id::text=$1`, id); err != nil {
		t.Fatal(err)
	}
	got := h.Call("PUT", base+"/"+id, `{"name":"Acme Inc","plan_id":"pro","contacts":[],"endpoints":[],"notes":"","reason":"rename without seeing contacts"}`, 200)
	if _, ok := got["notes"]; ok || got["name"] != "Acme Inc" {
		t.Fatalf("update: %v", got)
	}
	for _, item := range h.Call("GET", base, "", 200)["items"].([]any) {
		if _, ok := item.(map[string]any)["contacts"]; ok {
			t.Fatalf("contacts listed without permission: %v", item)
		}
	}
	var contacts, endpoints, notes string
	_ = h.Pool.QueryRow(ctx, `SELECT contacts::text, endpoints::text, notes FROM tenants WHERE id::text=$1`, id).Scan(&contacts, &endpoints, &notes)
	if !strings.Contains(contacts, "Ann") || !strings.Contains(endpoints, "lan") || notes != "vip" {
		t.Fatalf("hidden fields blanked by an update: %s %s %q", contacts, endpoints, notes)
	}
	h.As.Perms = func(string) bool { return true }
	if got := h.Call("GET", base+"/"+id, "", 200); got["notes"] != "vip" || len(got["contacts"].([]any)) != 1 {
		t.Fatalf("a caller with every permission sees them: %v", got)
	}
}
