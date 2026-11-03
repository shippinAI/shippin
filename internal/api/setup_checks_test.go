package api

import (
	"testing"

	"shippin.cloud/console/internal/managed"
)

func TestRRFlowCheck(t *testing.T) {
	p := setupProject{ID: "p1", Name: "web", Workspaces: 1}
	ready := managed.Instance{Kind: managed.KindRRFlow, State: "ready", Bindings: []managed.Binding{{State: "bound"}}}
	pending := managed.Instance{Kind: managed.KindRRFlow, State: "provisioning"}
	cases := []struct {
		name  string
		p     setupProject
		insts []managed.Instance
		on    bool
		state string
		fix   string
	}{
		{"off", p, nil, false, "off", ""},
		{"no devspace", setupProject{ID: "p1", Name: "web"}, nil, true, "todo", "devspace"},
		{"missing", p, nil, true, "todo", "rrflow_attach"},
		{"pending", p, []managed.Instance{pending}, true, "todo", ""},
		{"error", p, []managed.Instance{{Kind: managed.KindRRFlow, State: "error"}}, true, "error", "rrflow_attach"},
		{"ready", p, []managed.Instance{ready}, true, "ok", ""},
		{"migrating (pre-D36)", p, []managed.Instance{{Kind: managed.KindRRFlow, State: "migrating"}, ready}, true, "todo", ""},
		{"other kind ignored", p, []managed.Instance{{Kind: "x", State: "ready"}}, true, "todo", "rrflow_attach"},
	}
	for _, c := range cases {
		got := rrflowCheck(c.p, c.insts, c.on)
		if got.State != c.state || got.Fix != c.fix {
			t.Errorf("%s: got %s/%s, want %s/%s", c.name, got.State, got.Fix, c.state, c.fix)
		}
	}
}

func TestAutoAttachCheck(t *testing.T) {
	for _, c := range []struct {
		on, auto bool
		want     string
	}{{false, true, "off"}, {true, false, "off"}, {true, true, "ok"}} {
		if got := autoAttachCheck(c.on, c.auto).State; got != c.want {
			t.Errorf("on=%v auto=%v: got %s want %s", c.on, c.auto, got, c.want)
		}
	}
}

func TestVMRRFlowCheck(t *testing.T) {
	if vmRRFlowCheck(nil).State != "todo" {
		t.Fatal("no instance should be todo")
	}
	if got := vmRRFlowCheck([]managed.Instance{{Kind: managed.KindRRFlow, Scope: "vm", State: "ready"}}); got.State != "ok" {
		t.Fatalf("ready: %s", got.State)
	}
	if vmRRFlowCheck([]managed.Instance{{Kind: managed.KindRRFlow, Scope: "vm", State: "error"}}).State != "error" {
		t.Fatal("error not surfaced")
	}
}
