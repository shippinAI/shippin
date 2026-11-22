package service_test

import (
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

const plans = "/v1/operator/tenancy/plans"

func serve(t *testing.T) *mcpltest.Harness {
	return mcpltest.Serve(t, actions.All, "shippin.tenancy.plan.", mcpltest.As{Actor: "tester", Perms: func(string) bool { return true }})
}

// TestUpdate includes a service in a plan with its defaults, changes it, shows it with the plan, and
// refuses a bad lock, limits that aren't an object, and a plan or service that isn't there.
func TestUpdate(t *testing.T) {
	h := serve(t)
	cell := plans + "/starter/services/chatmaton"
	h.Call("PUT", cell, `{"lock":"bounded"}`, 400) // no reason
	got := h.Call("PUT", cell, `{"reason":"include chatmaton"}`, 200)
	if got["plan_id"] != "starter" || got["service_id"] != "chatmaton" || got["enabled"] != true || got["lock"] != "bounded" ||
		len(got["limits"].(map[string]any)) != 0 {
		t.Fatalf("a new cell is enabled and bounded with no limits: %v", got)
	}
	got = h.Call("PUT", cell, `{"enabled":false,"lock":"overridable","limits":{"seats":3},"reason":"limit seats"}`, 200)
	if got["enabled"] != false || got["lock"] != "overridable" || got["limits"].(map[string]any)["seats"] != 3.0 {
		t.Fatalf("cell update: %v", got)
	}
	var found bool
	for _, s := range h.Call("GET", plans+"/starter", "", 200)["services"].([]any) {
		if m := s.(map[string]any); m["service_id"] == "chatmaton" {
			found = m["lock"] == "overridable" && m["limits"].(map[string]any)["seats"] == 3.0
		}
	}
	if !found {
		t.Fatal("the plan doesn't show its changed service")
	}
	for _, bad := range []string{`"lock":"weird"`, `"limits":[1]`, `"limits":"seats"`, `"colour":"blue"`} {
		h.Call("PUT", cell, `{`+bad+`,"reason":"validation test"}`, 400)
	}
	h.Call("PUT", plans+"/nope/services/chatmaton", `{"reason":"missing plan"}`, 404)
	h.Call("PUT", plans+"/starter/services/nope", `{"reason":"missing service"}`, 404)
	if len(h.Audits) != 2 || h.Audits[0] != "shippin.tenancy.plan.service.update planId=starter serviceId=chatmaton · include chatmaton" {
		t.Fatalf("audits: %q", h.Audits)
	}
	if len(h.Events) != 2 || h.Events[1].ID != "shippin.tenancy.plan.service.updated" || h.Events[1].Subject["planId"] != "starter" ||
		h.Events[1].Subject["serviceId"] != "chatmaton" || h.Events[1].Perm != "catalog.plans.write" {
		t.Fatalf("events: %+v", h.Events)
	}
}
