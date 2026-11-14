package coverage_test

import (
	"context"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

func TestOwnCoverage(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.tenancy.coverage.", mcpltest.As{Actor: "ada"})
	ctx := context.Background()
	var tenant string
	if err := h.Pool.QueryRow(ctx, `INSERT INTO tenants(id, name, idp_org_id, plan) VALUES(gen_random_uuid(), 'Acme', 'org-acme', 'business')
		RETURNING id::text`).Scan(&tenant); err != nil {
		t.Fatal(err)
	}
	h.As.Tenant = tenant
	got := h.Call("GET", "/v1/tenants/"+tenant+"/tenancy/coverage", "", 200)["coverage"].(map[string]any)
	if got["id"] != "advanced" {
		t.Fatalf("coverage: %v", got)
	}
	if _, ok := got["price_cents"]; ok {
		t.Fatal("a member sees the tier's price")
	}
}
