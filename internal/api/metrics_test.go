package api

import (
	"testing"
)

// The metrics range is the host read permission: staff without infra.read are refused before the
// action runs, and those with it reach it.
func TestMetricsNeedInfraRead(t *testing.T) {
	const route = "GET /v1/operator/infrastructure/metrics"
	idp := newFakeIdP().add("billing", "staff.billing").add("support", "staff.support").add("infra", "staff.infra").add("norole")
	a := staffAPI(newFakeStore("enforce"), idp)
	mux := stubMux(a)
	for user, want := range map[string]int{"billing": 403, "support": 403, "norole": 403, "infra": 204} {
		if got := call(mux, route, user, false); got != want {
			t.Errorf("%s: got %d, want %d", user, got, want)
		}
	}
}
