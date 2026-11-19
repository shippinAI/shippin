package ext

import (
	"slices"
	"strings"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/settings"
)

type probeIn struct{}

type probeOut struct {
	OK bool `json:"ok"`
}

func probe(id string) mcpl.Registered {
	return mcpl.Register(id, mcpl.Def[probeIn, probeOut]{
		Spec: mcpl.Spec{Summary: "Answer the probe.", Access: mcpl.Public, Singleton: true},
		Run:  func(*mcpl.Call, probeIn) (probeOut, error) { return probeOut{OK: true}, nil },
	})
}

func panics(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: no panic", name)
		}
	}()
	f()
}

// With no module, ext is exactly the panel's generated registry.
func TestNoModuleIsThePanelAlone(t *testing.T) {
	defer Swap()()
	if len(Actions()) != len(actions.All) || len(Reactions()) != len(actions.Reactions) ||
		len(Providers()) != len(actions.Providers) || len(Settings()) != len(actions.Settings) {
		t.Fatal("ext added something with no module registered")
	}
}

// A module's actions and settings join the panel's, actions in ID order.
func TestAModuleJoinsThePanel(t *testing.T) {
	defer Swap()()
	Register(Module{Name: "probe", Actions: []mcpl.Registered{probe("shippin.public.ext_probe.get")},
		Settings: []settings.Section{{Key: "ext_probe"}}})
	all := Actions()
	if len(all) != len(actions.All)+1 || !slices.IsSortedFunc(all, func(a, b mcpl.Registered) int { return strings.Compare(a.ID, b.ID) }) {
		t.Fatalf("actions: %d, sorted %v", len(all), slices.IsSortedFunc(all, func(a, b mcpl.Registered) int { return strings.Compare(a.ID, b.ID) }))
	}
	if !slices.ContainsFunc(all, func(g mcpl.Registered) bool { return g.ID == "shippin.public.ext_probe.get" }) {
		t.Error("the module's action is missing")
	}
	if s := Settings(); s[len(s)-1].Key != "ext_probe" {
		t.Error("the module's settings section is missing")
	}
	if ms := Modules(); len(ms) != 1 || ms[0].Name != "probe" {
		t.Errorf("modules: %+v", ms)
	}
}

// A clash stops the binary at start instead of shadowing a route or a section.
func TestAClashIsRefused(t *testing.T) {
	defer Swap()()
	Register(Module{Name: "one", Actions: []mcpl.Registered{probe("shippin.public.ext_probe.get")}})
	panics(t, "no name", func() { Register(Module{}) })
	panics(t, "same name", func() { Register(Module{Name: "one"}) })
	panics(t, "same action as another module", func() {
		Register(Module{Name: "two", Actions: []mcpl.Registered{probe("shippin.public.ext_probe.get")}})
	})
	if len(actions.All) > 0 {
		panics(t, "same action as the panel", func() {
			Register(Module{Name: "three", Actions: []mcpl.Registered{actions.All[0]}})
		})
	}
	if len(actions.Settings) > 0 {
		panics(t, "same settings section as the panel", func() {
			Register(Module{Name: "four", Settings: []settings.Section{{Key: actions.Settings[0].Key}}})
		})
	}
	if ms := Modules(); len(ms) != 1 {
		t.Errorf("a refused module was kept: %+v", ms)
	}
}

// Permissions and job kinds clash between modules like actions do; one without a key or kind is refused.
func TestModulePermissionsAndJobsAreChecked(t *testing.T) {
	defer Swap()()
	job := func(kind string) Job {
		return Job{Kind: kind, Handler: func(Runtime) jobs.Handler { return nil }}
	}
	Register(Module{Name: "one", Perms: []Perm{{Key: "widgets.read"}}, Jobs: []Job{job("widget.build")}})
	if p := Perms(); len(p) != 1 || p[0].Key != "widgets.read" {
		t.Errorf("perms: %+v", p)
	}
	if j := Jobs(); len(j) != 1 || j[0].Kind != "widget.build" {
		t.Errorf("jobs: %+v", j)
	}
	panics(t, "same permission", func() { Register(Module{Name: "two", Perms: []Perm{{Key: "widgets.read"}}}) })
	panics(t, "same job kind", func() { Register(Module{Name: "three", Jobs: []Job{job("widget.build")}}) })
	panics(t, "permission without a key", func() { Register(Module{Name: "four", Perms: []Perm{{Label: "x"}}}) })
	panics(t, "job without a handler", func() { Register(Module{Name: "five", Jobs: []Job{{Kind: "widget.paint"}}}) })
	if len(Modules()) != 1 {
		t.Errorf("a refused module was kept: %+v", Modules())
	}
}
