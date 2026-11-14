package plan_test

import (
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

const base = "/v1/operator/tenancy/plans"

// serve mounts the plan actions (and a plan's service actions) on a fresh database for a caller who may
// or may not see prices.
func serve(t *testing.T, prices bool) *mcpltest.Harness {
	return mcpltest.Serve(t, actions.All, "shippin.tenancy.plan.", mcpltest.As{Actor: "tester",
		Perms: func(p string) bool { return prices || p != "catalog.prices.read" }})
}

// price is the stored price of a plan, whatever the caller may see.
func price(t *testing.T, h *mcpltest.Harness, id string) int {
	t.Helper()
	var cents int
	if err := h.Pool.QueryRow(t.Context(), `SELECT price_cents FROM plans WHERE id=$1`, id).Scan(&cents); err != nil {
		t.Fatal(err)
	}
	return cents
}
