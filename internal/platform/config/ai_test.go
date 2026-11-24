package config

import "testing"

func TestAICheck(t *testing.T) {
	ok := AI{Enabled: true, IdleMinutes: 15, TouchSeconds: 60, Providers: []AIProvider{{ID: "claude", Label: "Claude", LoginCommand: "claude auth login"}}}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	if (AI{}).Check() != nil || (AI{}).Allows("t") {
		t.Fatal("disabled section is valid and allows nobody")
	}
	dup := ok
	dup.Providers = append(dup.Providers, ok.Providers[0])
	bad := ok
	bad.Providers = []AIProvider{{ID: "Claude!", Label: "x", LoginCommand: "x"}}
	for _, c := range []AI{dup, bad, {Enabled: true}} {
		if c.Check() == nil {
			t.Fatalf("must fail: %+v", c)
		}
	}
	ok.Tenants = []string{"a"}
	if !ok.Allows("a") || ok.Allows("b") {
		t.Fatal("tenant allow-list")
	}
	if _, found := ok.Provider("claude"); !found {
		t.Fatal("provider lookup")
	}
}
