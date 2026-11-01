package api

import (
	"testing"

	"shippin.cloud/console/internal/db"
)

func TestCheckRules(t *testing.T) {
	c := testWS()
	ok := []db.Rule{{Protocol: "tcp", Port: 8080}, {Protocol: "udp", Port: 8080, Source: "203.0.113.0/24"}, {Protocol: "tcp", Port: 5432, Source: "198.51.100.7"}}
	if msg := checkRules(ok, c); msg != "" {
		t.Fatalf("valid rules rejected: %s", msg)
	}
	bad := map[string][]db.Rule{
		"edge https":   {{Protocol: "tcp", Port: 443}},
		"edge http":    {{Protocol: "tcp", Port: 80}},
		"ssh":          {{Protocol: "tcp", Port: 22}},
		"ssh range lo": {{Protocol: "tcp", Port: 22000}},
		"ssh range hi": {{Protocol: "tcp", Port: 22999}},
		"zero":         {{Protocol: "tcp", Port: 0}},
		"too high":     {{Protocol: "tcp", Port: 65536}},
		"icmp":         {{Protocol: "icmp", Port: 8080}},
		"ipv6 source":  {{Protocol: "tcp", Port: 8080, Source: "2001:db8::/32"}},
		"junk source":  {{Protocol: "tcp", Port: 8080, Source: "1.2.3.4; drop"}},
		"duplicate":    {{Protocol: "tcp", Port: 8080}, {Protocol: "tcp", Port: 8080}},
	}
	for name, rs := range bad {
		if checkRules(rs, c) == "" {
			t.Errorf("%s: accepted", name)
		}
	}
	many := make([]db.Rule, 21)
	for i := range many {
		many[i] = db.Rule{Protocol: "tcp", Port: 9000 + i}
	}
	if checkRules(many, c) == "" {
		t.Error("21 rules accepted")
	}
}

func TestCheckDomains(t *testing.T) {
	c := testWS()
	ds := []string{" App.Customer.COM "}
	if msg := checkDomains(ds, c); msg != "" || ds[0] != "app.customer.com" {
		t.Fatalf("valid domain: %q %q", msg, ds[0])
	}
	for _, d := range []string{"example.test", "console.example.test", "x.ws.example.test", "a..b", "-x.com", "x", "https://x.com", "dup.com,dup.com"} {
		in := []string{d}
		if d == "dup.com,dup.com" {
			in = []string{"dup.com", "dup.com"}
		}
		if checkDomains(in, c) == "" {
			t.Errorf("%q accepted", d)
		}
	}
}

func TestCheckPreviews(t *testing.T) {
	c := testWS()
	if checkPreviews([]db.Preview{{Port: 5173}, {Port: 8000, Public: true}}, c) != "" {
		t.Fatal("valid previews rejected")
	}
	for name, ps := range map[string][]db.Preview{
		"privileged": {{Port: 80}}, "ide": {{Port: 3000}}, "dup": {{Port: 5173}, {Port: 5173}}, "range": {{Port: 70000}},
	} {
		if checkPreviews(ps, c) == "" {
			t.Errorf("%s accepted", name)
		}
	}
}
