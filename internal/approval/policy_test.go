package approval

import "testing"

func TestDefaultAllowsFounderNotANamedUser(t *testing.T) {
	p := Default()
	if !p.Allows(Subject{Roles: []string{"founder"}}, "hub.decide", "any-host") {
		t.Fatal("founder role must approve hub decisions on every host")
	}
	if p.Allows(Subject{Sub: "someone"}, "hub.decide", "any-host") {
		t.Fatal("a subject who is not in config must not approve")
	}
	if p.Allows(Subject{Roles: []string{"founder"}}, "hub.decide", "") {
		// founder is the default hub decider
	} else {
		t.Fatal("founder role must decide the board")
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

func TestRuntimeSubjectsAreConfig(t *testing.T) {
	p := Default().WithRuntime(func(k string) string {
		switch k {
		case "HUB_OWNER_SUB":
			return "someone|configured"
		case "HUB_OWNER_EMAIL":
			return "owner@example.test"
		default:
			return ""
		}
	})
	if !p.Allows(Subject{Sub: "someone|configured"}, "hub.decide", "") {
		t.Fatal("env subject should be copied onto the hub rule")
	}
	if !p.Allows(Subject{Email: "Owner@Example.test", EmailVerified: true}, "hub.decide", "") {
		t.Fatal("verified env e-mail should be copied onto the hub rule")
	}
	if p.Allows(Subject{Email: "Owner@Example.test"}, "hub.decide", "") {
		t.Fatal("unverified e-mail must not approve")
	}
}
