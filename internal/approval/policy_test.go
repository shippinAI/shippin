package approval

import "testing"

func TestEmptyPolicyFailsClosed(t *testing.T) {
	if (Policy{}).Allows(Subject{Roles: []string{"founder", "op_infra"}}, "hv.lifecycle", "server") {
		t.Fatal("an empty policy must not authorize an action")
	}
}

func TestHostRuleDoesNotSpill(t *testing.T) {
	p := Policy{Enabled: true, Rules: []Rule{{
		ID: "one-host", Actions: []string{"hv.lifecycle"}, Hosts: []string{"bastion.example"},
		ApproverRoles: []string{"founder"}, NeedsApproval: true,
	}}}
	ok := Subject{Roles: []string{"founder"}}
	if !p.Allows(ok, "hv.lifecycle", "bastion.example") {
		t.Fatal("matching host")
	}
	if p.Allows(ok, "hv.lifecycle", "other.example") {
		t.Fatal("other host must not match")
	}
}

func TestRequiredReviewCannotBeDisabledByBoardDriver(t *testing.T) {
	for _, driver := range []string{"", "off", "hub", "http"} {
		p := Policy{Board: BoardConfig{Driver: driver}, Rules: []Rule{{
			ID: "lifecycle", Actions: []string{"hv.lifecycle"}, NeedsApproval: true,
		}}}
		if !p.RequiresReview("hv.lifecycle", "host-a") {
			t.Fatalf("board driver %q disabled a policy-required review", driver)
		}
	}
}

func TestOverlappingRuleCannotShadowRequiredReview(t *testing.T) {
	p := Policy{Rules: []Rule{
		{ID: "general", Actions: []string{"hv.lifecycle"}, ApproverRoles: []string{"operator"}},
		{ID: "review-required", Actions: []string{"hv.lifecycle"}, Hosts: []string{"host-a"}, NeedsApproval: true},
	}}
	if !p.RequiresReview("hv.lifecycle", "host-a") {
		t.Fatal("a matching review rule must remain mandatory even when an earlier rule also matches")
	}
	if p.RequiresReview("hv.lifecycle", "host-b") {
		t.Fatal("host-scoped review must not spill to a different host")
	}
}
