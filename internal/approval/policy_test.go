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
