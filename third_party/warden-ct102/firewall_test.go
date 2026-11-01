package main

import (
	"testing"
)

func TestParseUFWRules(t *testing.T) {
	output := `Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    Anywhere
[ 2] 80,443/tcp                 ALLOW IN    Anywhere                   # Web server
[ 3] Anywhere                   DENY IN     192.0.2.0/24
[ 4] 22/tcp                     LIMIT IN    Anywhere                   # Rate limit SSH
[ 5] 8080                       ALLOW IN    10.0.0.0/8                 # Dev server
[ 6] 22/tcp (v6)                ALLOW IN    Anywhere (v6)
`

	rules := parseUFWRules(output)
	if len(rules) != 6 {
		t.Fatalf("parseUFWRules returned %d rules, want 6", len(rules))
	}

	// Rule 1: simple allow
	if rules[0].Number != "1" {
		t.Errorf("rule 1 number = %q, want 1", rules[0].Number)
	}
	if rules[0].To != "22/tcp" {
		t.Errorf("rule 1 to = %q, want 22/tcp", rules[0].To)
	}
	if rules[0].Action != "ALLOW" {
		t.Errorf("rule 1 action = %q, want ALLOW", rules[0].Action)
	}
	if rules[0].Direction != "IN" {
		t.Errorf("rule 1 direction = %q, want IN", rules[0].Direction)
	}
	if rules[0].From != "Anywhere" {
		t.Errorf("rule 1 from = %q, want Anywhere", rules[0].From)
	}

	// Rule 2: with comment
	if rules[1].To != "80,443/tcp" {
		t.Errorf("rule 2 to = %q, want 80,443/tcp", rules[1].To)
	}
	if rules[1].Comment != "Web server" {
		t.Errorf("rule 2 comment = %q, want 'Web server'", rules[1].Comment)
	}

	// Rule 3: deny from subnet
	if rules[2].Action != "DENY" {
		t.Errorf("rule 3 action = %q, want DENY", rules[2].Action)
	}
	if rules[2].From != "192.0.2.0/24" {
		t.Errorf("rule 3 from = %q, want 192.0.2.0/24", rules[2].From)
	}

	// Rule 4: limit
	if rules[3].Action != "LIMIT" {
		t.Errorf("rule 4 action = %q, want LIMIT", rules[3].Action)
	}
}

func TestParseUFWRulesEmpty(t *testing.T) {
	output := "Status: inactive"
	rules := parseUFWRules(output)
	if len(rules) != 0 {
		t.Errorf("parseUFWRules inactive returned %d rules, want 0", len(rules))
	}
}

func TestBuildUFWRuleArgs(t *testing.T) {
	tests := []struct {
		action, dir, port, proto, from, to, comment string
		expected                                     string
	}{
		{"allow", "in", "22", "tcp", "", "", "", "allow 22/tcp"},
		{"deny", "in", "80", "any", "", "", "", "deny 80"},
		{"allow", "out", "443", "tcp", "", "", "", "allow out 443/tcp"},
		{"allow", "in", "22", "tcp", "10.0.0.0/8", "", "SSH from LAN", "allow from 10.0.0.0/8 to any port 22 proto tcp comment SSH from LAN"},
		{"deny", "in", "", "any", "192.0.2.100", "", "", "deny from 192.0.2.100"},
	}

	for _, tt := range tests {
		args := buildUFWRuleArgs(tt.action, tt.dir, tt.port, tt.proto, tt.from, tt.to, tt.comment)
		got := joinArgs(args)
		if got != tt.expected {
			t.Errorf("buildUFWRuleArgs(%q,%q,%q,%q,%q,%q,%q) = %q, want %q",
				tt.action, tt.dir, tt.port, tt.proto, tt.from, tt.to, tt.comment, got, tt.expected)
		}
	}
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}

func TestIsValidUFWAction(t *testing.T) {
	valid := []string{"allow", "deny", "reject", "limit"}
	invalid := []string{"drop", "ALLOW", "block", "", "allow;rm"}

	for _, a := range valid {
		if !isValidUFWAction(a) {
			t.Errorf("isValidUFWAction(%q) = false, want true", a)
		}
	}
	for _, a := range invalid {
		if isValidUFWAction(a) {
			t.Errorf("isValidUFWAction(%q) = true, want false", a)
		}
	}
}

func TestIsValidPort(t *testing.T) {
	valid := []string{"22", "80,443", "8000:9000", "22/tcp"}
	invalid := []string{"abc", "22;rm", "80 443", ""}

	for _, p := range valid {
		if !isValidPort(p) {
			t.Errorf("isValidPort(%q) = false, want true", p)
		}
	}
	for _, p := range invalid {
		if isValidPort(p) {
			t.Errorf("isValidPort(%q) = true, want false", p)
		}
	}
}

func TestIsValidAddress(t *testing.T) {
	valid := []string{"10.0.0.0/8", "192.0.2.1", "any", "Anywhere", "::1", "fe80::/10"}
	invalid := []string{"abc;rm", "10.0.0.0 && rm", ""}

	for _, a := range valid {
		if !isValidAddress(a) {
			t.Errorf("isValidAddress(%q) = false, want true", a)
		}
	}
	for _, a := range invalid {
		if isValidAddress(a) {
			t.Errorf("isValidAddress(%q) = true, want false", a)
		}
	}
}
