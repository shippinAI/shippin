package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shippin.cloud/console/internal/mcpl"
)

// What the factory writes is what the generator finds: the same ID, route and spec, for every shape.
func TestScaffoldIsFoundWhereItSays(t *testing.T) {
	root := t.TempDir()
	cases := []New{
		{Path: "tenancy/plan/list", Spec: mcpl.Spec{Summary: "List the plans.", Access: mcpl.Staff, Perm: "catalog.plans.read"}},
		{Path: "tenancy/plan/update", Spec: mcpl.Spec{Summary: "Change a plan.", Access: mcpl.Staff, Perm: "catalog.plans.write"}},
		{Path: "tenancy/plan/delete", Spec: mcpl.Spec{Summary: "Remove a plan.", Access: mcpl.Staff, Perm: "catalog.plans.write"}},
		{Path: "tenancy/plan/publish", Spec: mcpl.Spec{Summary: "Publish a plan.", Access: mcpl.Staff, Perm: "catalog.plans.write"}},
		{Path: "tenancy/plan/service/create", Spec: mcpl.Spec{Summary: "Add a service to a plan.", Access: mcpl.Staff, Perm: "catalog.plans.write"}},
		{Path: "tenancy/setting/update", Spec: mcpl.Spec{Summary: "Change the organization's settings.", Access: mcpl.Member, Perm: "manage", Singleton: true}},
		{Path: "identity/token/rotate", Collection: true, Spec: mcpl.Spec{Summary: "Rotate your tokens.", Access: mcpl.SignedIn}},
	}
	want := map[string]string{}
	for _, c := range cases {
		files, err := Scaffold(root, c)
		if err != nil {
			t.Fatalf("%s: %v", c.Path, err)
		}
		if !strings.HasSuffix(files[len(files)-1], "_db_test.go") {
			t.Errorf("%s: no test written: %v", c.Path, files)
		}
		c.Spec.Collection = c.Spec.Collection || c.Collection
		id := "shippin." + strings.ReplaceAll(c.Path, "/", ".")
		want[id], _ = mcpl.RouteFor(id, c.Spec)
	}
	found, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != len(want) {
		t.Fatalf("found %d actions, want %d", len(found), len(want))
	}
	for _, f := range found {
		if f.Route != want[f.ID] {
			t.Errorf("%s: found at %s, the factory said %s", f.ID, f.Route, want[f.ID])
		}
	}
	src, _ := os.ReadFile(filepath.Join(root, "internal", "tenancy", "plan", "service", "service.go"))
	if !strings.Contains(string(src), "PlanID string `path:\"planId\"") {
		t.Errorf("a nested entity names its parent from the route:\n%s", src)
	}
	if _, err := Scaffold(root, cases[0]); err == nil || !strings.Contains(err.Error(), "never overwrites") {
		t.Errorf("overwrote an action: %v", err)
	}
}

// The factory refuses what the rules refuse, before writing anything.
func TestScaffoldRefuses(t *testing.T) {
	root := t.TempDir()
	for path, s := range map[string]mcpl.Spec{
		"billing/plan/list":    {Summary: "List.", Access: mcpl.Staff, Perm: "p"},
		"tenancy/plan/list":    {Summary: "List.", Access: mcpl.Staff},
		"tenancy/Plan/list":    {Summary: "List.", Access: mcpl.Staff, Perm: "p"},
		"tenancy/list":         {Summary: "List.", Access: mcpl.Staff, Perm: "p"},
		"tenancy/x/list":       {Summary: "List.", Access: mcpl.Member, Singleton: true},
		"tenancy/plan/create":  {Summary: "Make one.", Access: mcpl.Public},
		"identity/key/create":  {Summary: "no full stop", Access: mcpl.SignedIn},
		"identity/key/rotate":  {Summary: "Rotate.", Access: mcpl.SignedIn, Perm: "x"},
		"tenancy/plan/approve": {Summary: "Approve.", Access: mcpl.Member, Perm: "owner"},
	} {
		if _, err := Scaffold(root, New{Path: path, Spec: s}); err == nil {
			t.Errorf("%s %+v was accepted", path, s)
		}
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("a refused request wrote files: %v", entries)
	}
}
