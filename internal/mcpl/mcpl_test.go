package mcpl

import (
	"strings"
	"testing"
)

func TestValidateAcceptsACanonicalAction(t *testing.T) {
	ok := []Action{
		{ID: "shippin.tenancy.plan.list", Route: "GET /v1/operator/catalog/plans", Access: Staff, Perm: "catalog.plans.read", Risk: Read, Summary: "List the plans."},
		{ID: "shippin.tenancy.plan.update", Route: "PUT /v1/operator/catalog/plans/{id}", Access: Staff, Perm: "catalog.plans.write", Risk: Write, Reason: true, Summary: "Change a plan.", Event: "shippin.tenancy.plan.updated"},
		{ID: "shippin.tenancy.plan.delete", Route: "DELETE /v1/operator/catalog/plans/{id}", Access: Staff, Perm: "catalog.plans.write", Risk: Destructive, Summary: "Remove a plan.", Event: "shippin.tenancy.plan.deleted"},
		{ID: "shippin.public.waitlist.join", Route: "POST /v1/waitlist", Access: Public, Risk: Write, Summary: "Ask to join the waitlist.", Event: "shippin.public.waitlist.joined"},
		{ID: "shippin.tenancy.coverage.get", Route: "GET /v1/tenants/{tenantId}/tenancy/coverage", Access: Member, Risk: Read, Summary: "Read the coverage."},
		{ID: "shippin.access.grant.approve", Route: "POST /v1/tenants/{tenantId}/access/grants/{grantId}/approve", Access: Member, Perm: "decide", Risk: Write, Summary: "Approve a grant.", Event: "shippin.access.grant.approved"},
	}
	if errs := Validate(ok); len(errs) != 0 {
		t.Fatal(errs)
	}
}

func TestValidateRefusesEachBrokenRule(t *testing.T) {
	base := Action{ID: "shippin.tenancy.plan.update", Route: "PUT /v1/operator/catalog/plans/{id}", Access: Staff, Perm: "p", Risk: Write, Summary: "Change a plan.", Event: "shippin.tenancy.plan.updated"}
	cases := map[string]func(a *Action){
		"ID must be":              func(a *Action) { a.ID = "plans.update" },
		"unknown domain":          func(a *Action) { a.ID = "shippin.billing.plan.update" },
		"route must be":           func(a *Action) { a.Route = "PUT /operator/plans" },
		"update is a PUT":         func(a *Action) { a.Route = "DELETE /v1/x/{id}" },
		"is a PUT":                func(a *Action) { a.Route = "GET /v1/x"; a.Risk = Write },
		"names its":               func(a *Action) { a.Perm = "" },
		"summary is one":          func(a *Action) { a.Summary = "no full stop" },
		"event must be":           func(a *Action) { a.Event = "plan.updated" },
		"event domain":            func(a *Action) { a.Event = "shippin.services.plan.updated" },
		"write action, not maybe": func(a *Action) { a.Risk = "maybe" },
		"only public and auth":    func(a *Action) { a.Access = Public; a.Perm = "" },
		"has no permission":       func(a *Action) { a.Access = SignedIn },
		"member action's":         func(a *Action) { a.Access = Member; a.Perm = "catalog.plans.write" },
		"a change emits":          func(a *Action) { a.Event = "" },
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
	del := Action{ID: "shippin.tenancy.plan.erase", Route: "DELETE /v1/x/{id}", Access: Staff, Perm: "p", Risk: Destructive, Summary: "Remove it.", Event: "shippin.tenancy.plan.erased"}
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

func TestRouteFor(t *testing.T) {
	cases := []struct {
		id   string
		spec Spec
		want string
	}{
		{"shippin.tenancy.coverage_tier.update", Spec{Access: Staff}, "PUT /v1/operator/tenancy/coverage-tiers/{coverageTierId}"},
		{"shippin.tenancy.coverage_tier.list", Spec{Access: Staff}, "GET /v1/operator/tenancy/coverage-tiers"},
		{"shippin.identity.ssh_key.delete", Spec{Access: SignedIn}, "DELETE /v1/me/identity/ssh-keys/{sshKeyId}"},
		{"shippin.tenancy.coverage.get", Spec{Access: Member, Singleton: true}, "GET /v1/tenants/{tenantId}/tenancy/coverage"},
		{"shippin.runtime.workspace.firewall.update", Spec{Access: Member, Singleton: true}, "PUT /v1/tenants/{tenantId}/runtime/workspaces/{workspaceId}/firewall"},
		{"shippin.runtime.workspace.restart", Spec{Access: Member}, "POST /v1/tenants/{tenantId}/runtime/workspaces/{workspaceId}/restart"},
		{"shippin.public.waitlist.join", Spec{Access: Public, Collection: true}, "POST /v1/public/waitlists/join"},
		{"shippin.access.policy.check", Spec{Access: Staff, Collection: true}, "POST /v1/operator/access/policies/check"},
	}
	for _, c := range cases {
		got, err := RouteFor(c.id, c.spec)
		if err != nil || got != c.want {
			t.Errorf("%s: %q %v, want %q", c.id, got, err, c.want)
		}
	}
	if _, err := RouteFor("shippin.tenancy.coverage.list", Spec{Access: Member, Singleton: true}); err == nil {
		t.Error("a singleton list was allowed")
	}
	for _, verb := range []string{"list", "get", "create", "update", "delete"} {
		if r, err := RouteFor("shippin.tenancy.plan."+verb, Spec{Access: Staff, Collection: true}); err == nil {
			t.Errorf("Collection on %s was allowed: %s", verb, r)
		}
	}
}

func TestEvents(t *testing.T) {
	for verb, want := range map[string]string{"create": "created", "update": "updated", "delete": "deleted", "revoke": "revoked",
		"approve": "approved", "deny": "denied", "send": "sent", "stop": "stopped", "take_over": "taken_over", "reset": "reset",
		"join": "joined", "copy": "copied", "play": "played"} {
		if got := PastTense(verb); got != want {
			t.Errorf("%s: %s, want %s", verb, got, want)
		}
	}
	if got := EventFor("shippin.identity.ssh_key.create"); got != "shippin.identity.ssh_key.created" {
		t.Error(got)
	}
	var b Bus
	ch, cancel := b.Subscribe()
	b.Publish(Event{ID: "shippin.x.y.created"})
	if e := <-ch; e.ID != "shippin.x.y.created" {
		t.Error(e)
	}
	cancel()
	b.Publish(Event{ID: "after cancel"}) // must not panic on a closed channel
}

func TestInputSchema(t *testing.T) {
	type fields struct {
		Label string `json:"label"`
		Price *int   `json:"price_cents,omitempty"`
	}
	type in struct {
		ID string `path:"coverageTierId" json:"-"`
		fields
	}
	g := Register("shippin.tenancy.coverage_tier.update", Def[in, None]{Spec: Spec{Summary: "Change it.", Access: Staff, Perm: "p"}})
	s := InputSchema(g)
	props := s["properties"].(Schema)
	for _, want := range []string{"coverageTierId", "label", "price_cents", "reason"} {
		if props[want] == nil {
			t.Errorf("%s missing from %v", want, props)
		}
	}
	if props["ID"] != nil || props["fields"] != nil {
		t.Errorf("path field or embedded struct leaked: %v", props)
	}
	if req := s["required"].([]string); len(req) != 2 || req[0] != "coverageTierId" || req[1] != "reason" {
		t.Errorf("required: %v", req)
	}
	read := InputSchema(Register("shippin.tenancy.coverage_tier.get", Def[in, None]{Spec: Spec{Summary: "Read it.", Access: Staff, Perm: "p"}}))
	if rp := read["properties"].(Schema); rp["label"] != nil || rp["reason"] != nil {
		t.Errorf("a read takes no body or reason: %v", rp)
	}
}
