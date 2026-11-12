package mcpl

import (
	"strings"
	"testing"
)

func TestValidateAcceptsACanonicalAction(t *testing.T) {
	ok := []Action{
		{ID: "shippin.tenancy.plan.list", Route: "GET /v1/operator/catalog/plans", Access: Staff, Perm: "catalog.plans.read", Risk: Read, Summary: "List the plans."},
		{ID: "shippin.tenancy.plan.update", Route: "PUT /v1/operator/catalog/plans/{id}", Access: Staff, Perm: "catalog.plans.write", Risk: Write, Reason: true, Summary: "Change a plan.", Event: "shippin.tenancy.plan.updated"},
		{ID: "shippin.tenancy.plan.delete", Route: "DELETE /v1/operator/catalog/plans/{id}", Access: Staff, Perm: "catalog.plans.write", Risk: Destructive, Summary: "Remove a plan."},
		{ID: "shippin.public.waitlist.join", Route: "POST /v1/waitlist", Access: Public, Risk: Write, Summary: "Ask to join the waitlist."},
	}
	if errs := Validate(ok); len(errs) != 0 {
		t.Fatal(errs)
	}
}

func TestValidateRefusesEachBrokenRule(t *testing.T) {
	base := Action{ID: "shippin.tenancy.plan.update", Route: "PUT /v1/operator/catalog/plans/{id}", Access: Staff, Perm: "p", Risk: Write, Summary: "Change a plan."}
	cases := map[string]func(a *Action){
		"ID must be":           func(a *Action) { a.ID = "plans.update" },
		"unknown domain":       func(a *Action) { a.ID = "shippin.billing.plan.update" },
		"route must be":        func(a *Action) { a.Route = "PUT /operator/plans" },
		"a write is not":       func(a *Action) { a.Route = "DELETE /v1/x/{id}" },
		"a GET is a read":      func(a *Action) { a.Route = "GET /v1/x"; a.Risk = Write },
		"names its":            func(a *Action) { a.Perm = "" },
		"summary is one":       func(a *Action) { a.Summary = "no full stop" },
		"event must be":        func(a *Action) { a.Event = "plan.updated" },
		"event domain":         func(a *Action) { a.Event = "shippin.services.plan.updated" },
		"risk must be":         func(a *Action) { a.Risk = "maybe" },
		"only public and auth": func(a *Action) { a.Access = Public },
	}
	for want, mutate := range cases {
		a := base
		mutate(&a)
		errs := Validate([]Action{a})
		found := false
		for _, e := range errs {
			found = found || strings.Contains(e.Error(), want)
		}
		if !found {
			t.Errorf("%q not reported for %+v: %v", want, a, errs)
		}
	}
	if errs := Validate([]Action{base, base}); len(errs) < 2 {
		t.Errorf("duplicates not reported: %v", errs)
	}
	del := Action{ID: "shippin.tenancy.plan.erase", Route: "DELETE /v1/x/{id}", Access: Staff, Perm: "p", Risk: Destructive, Summary: "Remove it."}
	if errs := Validate([]Action{del}); len(errs) == 0 {
		t.Error("a DELETE with a non-delete verb passed")
	}
}

func TestRiskOf(t *testing.T) {
	for route, want := range map[string]Risk{"GET /v1/a": Read, "POST /v1/a": Write, "PUT /v1/a": Write, "DELETE /v1/a": Destructive} {
		if got := RiskOf(route); got != want {
			t.Errorf("%s: %s, want %s", route, got, want)
		}
	}
}
