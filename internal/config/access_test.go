package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func shippedConfig(t *testing.T) Config {
	t.Helper()
	raw, err := os.ReadFile("../../config/shippin.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestShippedConfigPassesCheck(t *testing.T) {
	c := shippedConfig(t)
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestShippedConfigDoesNotSeedLocalApprovals(t *testing.T) {
	c := shippedConfig(t)
	if c.Approval.Enabled || len(c.Approval.Rules) != 0 || c.Approval.Board.Driver != "" {
		t.Fatalf("example config must not seed a local approver or board: %+v", c.Approval)
	}
}

// A stranger's install must not come with one installation's products. Keys may name an optional
// integration; what shows to a person may not.
func TestShippedConfigNamesNoOnesProducts(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "shippin.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	named := regexp.MustCompile(`\b(Nomen|Vaultix|Clyffy|Chatmaton|Devagotchi|AgentHub|Zitadel|Infisical)\b`)
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				walk(path+"."+k, c)
			}
		case []any:
			for i, c := range x {
				walk(fmt.Sprintf("%s[%d]", path, i), c)
			}
		case string:
			if named.MatchString(x) {
				t.Errorf("%s = %q", path, x)
			}
		}
	}
	walk("", doc)
}

func TestAnIdentityProviderIsOptional(t *testing.T) {
	c := shippedConfig(t)
	c.IDP.Name, c.IDP.ClientID = "", ""
	if err := c.Check(); err != nil {
		t.Fatalf("a panel with no identity provider must start: %v", err)
	}
	c.IDP.ClientID = "client"
	if err := c.Check(); err == nil {
		t.Fatal("one that is configured needs a name to show")
	}
}

func TestAccessCheckFailsClosed(t *testing.T) {
	for name, m := range map[string]func(*Access){
		"no durations":       func(a *Access) { a.Durations = nil },
		"bad duration":       func(a *Access) { a.Durations = []string{"forever"} },
		"too long":           func(a *Access) { a.Durations = []string{"200h"} },
		"negative":           func(a *Access) { a.Durations = []string{"-1h"} },
		"no reason bounds":   func(a *Access) { a.ReasonMin = 0 },
		"no sweep":           func(a *Access) { a.SweepSeconds = 0 },
		"no bg duration":     func(a *Access) { a.BreakGlass.Duration = "" },
		"no reauth":          func(a *Access) { a.BreakGlass.ReauthMinutes = 0 },
		"no categories":      func(a *Access) { a.BreakGlass.Categories = nil },
		"dup category":       func(a *Access) { a.BreakGlass.Categories = append(a.BreakGlass.Categories, a.BreakGlass.Categories[0]) },
		"delay without hold": func(a *Access) { a.BreakGlass.Categories[0].NoticeDelay = "1h" },
		"bad delay":          func(a *Access) { a.BreakGlass.Categories[3].NoticeDelay = "soon" },
		"no justification":   func(a *Access) { a.BreakGlass.JustificationMin = 0 },
		"no notice window":   func(a *Access) { a.BreakGlass.NoticeHours = 0 },
		"no log window":      func(a *Access) { a.LogWindow = "" },
	} {
		c := shippedConfig(t)
		m(&c.Access)
		if c.Access.Check() == nil {
			t.Errorf("%s: Check passed, want error", name)
		}
	}
}

func TestRolesCheckFailsClosed(t *testing.T) {
	for name, m := range map[string]func(*Config){
		"no iam":            func(c *Config) { c.Roles.IAM = nil },
		"no managers":       func(c *Config) { c.Roles.Managers = nil },
		"no admin role":     func(c *Config) { c.Operator.Role = "" },
		"admin unreachable": func(c *Config) { c.Operator.Role = "op_ghost" },
		"empty mapping":     func(c *Config) { c.Roles.Org = append(c.Roles.Org, RoleMap{From: "X"}) },
	} {
		c := shippedConfig(t)
		m(&c)
		if c.Check() == nil {
			t.Errorf("%s: Check passed, want error", name)
		}
	}
}

func TestOnlyLegalHoldMayWithhold(t *testing.T) {
	for _, cat := range shippedConfig(t).Access.BreakGlass.Categories {
		if cat.WithholdNotice != (cat.ID == "legal_hold") {
			t.Errorf("category %s: withhold_notice=%v", cat.ID, cat.WithholdNotice)
		}
	}
}
