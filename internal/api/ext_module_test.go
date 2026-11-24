package api

import (
	"context"
	"slices"
	"testing"

	"shippin.cloud/console/internal/ext"
	"shippin.cloud/console/internal/services/jobs"
)

// A module's permissions join the catalog that roles and routes use, and reach the preset roles they
// name; one the panel already declares stops the binary.
func TestModulePermissionsJoinTheCatalog(t *testing.T) {
	restore := ext.Swap(ext.Module{Name: "probe", Perms: []ext.Perm{
		{Key: "widgets.read", Section: "Widgets", Label: "View widgets", Grantable: true, Presets: []string{"staff.owner"}},
	}})
	defer func() { restore(); LoadModules() }()
	LoadModules()
	if !knownPerm("widgets.read") || !grantablePerm("widgets.read") {
		t.Fatal("the module's permission is not in the catalog")
	}
	if !slices.ContainsFunc(permCatalog, func(p Perm) bool { return p.Key == "widgets.read" && p.Section == "Widgets" }) {
		t.Error("the Team page's catalog lacks the module's permission")
	}
	if got := modulePresets()["staff.owner"]; !slices.Equal(got, []string{"widgets.read"}) {
		t.Errorf("presets: %v", got)
	}
	ext.Swap(ext.Module{Name: "clash", Perms: []ext.Perm{{Key: panelPerms[0].Key}}})
	defer func() {
		if recover() == nil {
			t.Error("a module permission with the panel's key was accepted")
		}
	}()
	LoadModules()
}

// A module's job kind runs on the panel's queue with the panel's services; a kind the panel already
// handles stops the queue from starting with the module.
func TestModuleJobsRunOnThePanelQueue(t *testing.T) {
	var got ext.Runtime
	defer ext.Swap(ext.Module{Name: "probe", Jobs: []ext.Job{{Kind: "widget.build", Handler: func(rt ext.Runtime) jobs.Handler {
		got = rt
		return func(context.Context, *jobs.Job, func(string)) (any, error) { return "built", nil }
	}}}})()
	a := &API{cfg: testCfg()}
	r := jobs.NewRunner(nil, jobs.Settings{})
	a.registerJobs(r)
	if !r.Has("widget.build") || !r.Has(jobs.KindRunnerRemove) {
		t.Fatal("the module's kind or the panel's is missing")
	}
	if got.Services == nil {
		t.Error("the module's handler was not given the panel's services")
	}

	defer ext.Swap(ext.Module{Name: "clash", Jobs: []ext.Job{{Kind: jobs.KindRunnerRemove, Handler: func(ext.Runtime) jobs.Handler { return nil }}}})()
	defer func() {
		if recover() == nil {
			t.Error("a module kind the panel handles was accepted")
		}
	}()
	a.registerJobs(jobs.NewRunner(nil, jobs.Settings{}))
}
