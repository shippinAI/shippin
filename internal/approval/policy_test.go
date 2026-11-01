package approval

import "testing"

func TestDefaultAllowsFounderNotANamedUser(t *testing.T) {
	p := Default()
	if !p.Allows(Subject{Roles: []string{"founder"}}, "gold.ticket", "any-host") {
		t.Fatal("founder role must approve gold tickets on every host")
	}
	if p.Allows(Subject{Sub: "someone"}, "gold.relay", "gold.example") {
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
		ID: "one-host", Actions: []string{"gold.ticket"}, Hosts: []string{"bastion.example"},
		ApproverRoles: []string{"founder"}, NeedsApproval: true,
	}}}
	ok := Subject{Roles: []string{"founder"}}
	if !p.Allows(ok, "gold.ticket", "bastion.example") {
		t.Fatal("matching host")
	}
	if p.Allows(ok, "gold.ticket", "other.example") {
		t.Fatal("other host must not match")
	}
}

func TestRuntimeSubjectsAreConfig(t *testing.T) {
	p := Default().WithRuntime(func(k string) string {
		switch k {
		case "GOLDSIGN_OWNER_SUB":
			return "nomen|configured"
		case "HUB_OWNER_EMAIL":
			return "owner@example.test"
		default:
			return ""
		}
	})
	if !p.Allows(Subject{Sub: "nomen|configured"}, "gold.passkey", "") {
		t.Fatal("env subject should be copied onto the gold rule")
	}
	if !p.Allows(Subject{Email: "Owner@Example.test", EmailVerified: true}, "hub.decide", "") {
		t.Fatal("verified env e-mail should be copied onto the hub rule")
	}
	if p.Allows(Subject{Email: "Owner@Example.test"}, "hub.decide", "") {
		t.Fatal("unverified e-mail must not approve")
	}
}
