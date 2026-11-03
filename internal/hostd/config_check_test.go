package hostd

import (
	"strings"
	"testing"
)

func TestConfigCheckFailsClosed(t *testing.T) {
	if err := testServer().cfg.Check(); err != nil {
		t.Fatal(err)
	}
	for name, m := range map[string]func(*Config){
		"no prefix":     func(c *Config) { c.Edge.ImagePrefix = "" },
		"preview zero":  func(c *Config) { c.Edge.PreviewMin = 0 },
		"remote admin":  func(c *Config) { c.Edge.CaddyAdmin = "http://192.0.2.1:2019" },
		"no repo hosts": func(c *Config) { c.Edge.RepoHosts = nil },
		"no reserved":   func(c *Config) { c.Edge.ReservedDomains = nil },
		"bad cookie":    func(c *Config) { c.Edge.WSCookie = "a=b" },
		"bad cidr":      func(c *Config) { c.TenantCIDR = "x" },
	} {
		c := *testServer().cfg
		m(&c)
		if c.Check() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestShippedHostdConfigPassesCheck(t *testing.T) {
	c, err := LoadConfig("../../deploy/hostd.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if !c.reservedPort(443) || !c.reservedPort(22000) || c.reservedPort(8080) {
		t.Fatal("reserved ports")
	}
	g, ok := c.Agent.Providers["grok"]
	if !ok || len(g.Resume) != 2 || g.Resume[0] != "grok" || g.Resume[1] != "--resume" {
		t.Fatalf("grok provider: %+v", g)
	}
	if c.Agent.SessionManager {
		t.Fatal("session manager must ship off")
	}
}

func TestNamesAndIDs(t *testing.T) {
	if ValidUUID("01a1414a-4346-7d6c-ab90-87332303e90d") != nil || ValidUUID("../../etc") == nil || ValidUUID("01A1414A-4346-7d6c-ab90-87332303e90d") == nil {
		t.Fatal("uuid validation")
	}
	if ValidLabel("dev") != nil || ValidLabel("-x") == nil || ValidLabel("Dev") == nil || ValidLabel(strings.Repeat("a", 32)) == nil {
		t.Fatal("label validation")
	}
	if _, err := ValidKeys([]string{"ssh-ed25519 AAAA\nssh-rsa x"}); err == nil {
		t.Fatal("embedded newline in key accepted")
	}
}

func TestLastSegment(t *testing.T) {
	for in, want := range map[string]string{
		"/1.0/instances/w-01a1426a-c666-7bd1-b278-917f90cd67dc?project=t-01a14263-479a-7d81-80f1-4afca958c629": "w-01a1426a-c666-7bd1-b278-917f90cd67dc",
		"/1.0/storage-pools/p-x/volumes/custom/h-abc?project=t-x":                                              "h-abc",
		"/1.0/instances/w-abc": "w-abc",
	} {
		if got := lastSegment(in); got != want {
			t.Errorf("lastSegment(%q) = %q", in, got)
		}
	}
}
