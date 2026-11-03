package config

import (
	"encoding/json"
	"os"
	"testing"
)

func shipped(t *testing.T) Workspaces {
	t.Helper()
	raw, err := os.ReadFile("../../config/shippin.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c.Workspaces
}

func TestShippedWorkspacesConfigPassesCheck(t *testing.T) {
	if err := shipped(t).Check(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspacesCheckFailsClosed(t *testing.T) {
	for name, m := range map[string]func(*Workspaces){
		"no repo hosts": func(w *Workspaces) { w.RepoHosts = nil },
		"no reserved":   func(w *Workspaces) { w.ReservedDomains = nil },
		"no prefix":     func(w *Workspaces) { w.ImagePrefix = "" },
		"preview zero":  func(w *Workspaces) { w.PreviewMin = 0 },
		"port range":    func(w *Workspaces) { w.PortMax = w.PortMin - 1 },
		"limits":        func(w *Workspaces) { w.TemplateLimits.MaxCPU = 0 },
	} {
		w := shipped(t)
		m(&w)
		if w.Check() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
