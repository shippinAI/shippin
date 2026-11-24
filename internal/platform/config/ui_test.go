package config

import (
	"strings"
	"testing"
)

func TestUIMergeAppliesOverridesByID(t *testing.T) {
	base := UI{
		DefaultSite: "main",
		Text:        map[string]string{"a.title": "A", "b.title": "B"},
		Nav:         UINav{Items: []UINavEntry{{ID: "operator.hosts", Label: "Hosts"}, {ID: "operator.health", Hidden: true}}},
		Providers:   []UIProvider{{Kind: "incus", Label: "One"}, {Kind: "proxmox", Label: "Two"}},
	}
	got := base.Merge(UI{
		Text:      map[string]string{"b.title": "B2", "c.title": "C"},
		Nav:       UINav{Items: []UINavEntry{{ID: "operator.hosts", Label: "Nodes", Order: 2}, {ID: "tenant.home", Hidden: true}}},
		Providers: []UIProvider{{Kind: "proxmox", Label: "PVE"}},
	})
	if got.DefaultSite != "main" {
		t.Errorf("default site %q", got.DefaultSite)
	}
	if got.Text["a.title"] != "A" || got.Text["b.title"] != "B2" || got.Text["c.title"] != "C" {
		t.Errorf("text %v", got.Text)
	}
	if len(got.Nav.Items) != 3 || got.Nav.Items[0].Label != "Nodes" || got.Nav.Items[0].Order != 2 || !got.Nav.Items[1].Hidden || got.Nav.Items[2].ID != "tenant.home" {
		t.Errorf("nav %+v", got.Nav.Items)
	}
	if p, ok := got.Provider("proxmox"); !ok || p.Label != "PVE" {
		t.Errorf("proxmox %+v", p)
	}
	if p, ok := got.Provider("incus"); !ok || p.Label != "One" {
		t.Errorf("incus %+v", p)
	}
	if base.Nav.Items[0].Label != "Hosts" || base.Text["b.title"] != "B" {
		t.Error("merge changed the config file value")
	}
}

func TestUICheckRejectsWhatCouldBreakThePanel(t *testing.T) {
	long := strings.Repeat("x", uiTextMax+1)
	for name, ui := range map[string]UI{
		"nav id":      {Nav: UINav{Items: []UINavEntry{{ID: "../etc"}}}},
		"section id":  {Nav: UINav{Sections: []UINavEntry{{ID: "A B"}}}},
		"long label":  {Nav: UINav{Items: []UINavEntry{{ID: "a.b", Label: strings.Repeat("x", uiLabelMax+1)}}}},
		"newline":     {Nav: UINav{Items: []UINavEntry{{ID: "a.b", Label: "a\nb"}}}},
		"text key":    {Text: map[string]string{"<script>": "x"}},
		"long text":   {Text: map[string]string{"a.b": long}},
		"kind":        {Providers: []UIProvider{{Kind: "Not A Kind"}}},
		"field name":  {Providers: []UIProvider{{Kind: "incus", Fields: []UIField{{Name: "a b"}}}}},
		"field value": {Providers: []UIProvider{{Kind: "incus", Fields: []UIField{{Name: "a", Help: long}}}}},
		"site":        {DefaultSite: "Not A Site"},
	} {
		if err := ui.Check(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	ok := UI{
		DefaultSite: "main",
		Text:        map[string]string{"infra.title": "Nodes"},
		Nav:         UINav{Items: []UINavEntry{{ID: "operator.hosts", Label: "Nodes"}}},
		Providers:   []UIProvider{{Kind: "incus", Label: "Incus", Fields: []UIField{{Name: "base_url", Label: "API URL"}}}},
	}
	if err := ok.Check(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
}
