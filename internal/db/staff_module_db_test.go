package db

import (
	"slices"
	"testing"
)

// A module's permissions for a preset role are seeded with it on a fresh database, and only there.
func TestStaffSeedWithModulePermissions(t *testing.T) {
	d, ctx := testDB(t)
	if err := d.SeedStaffWith(ctx, map[string][]string{"staff.owner": {"module.widgets.read"}}); err != nil {
		t.Fatal(err)
	}
	rs, err := d.StaffRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		has := slices.Contains(r.Permissions, "module.widgets.read")
		if has != (r.Key == "staff.owner") {
			t.Errorf("%s holds the module permission: %v", r.Key, has)
		}
	}
	for _, p := range StaffPresets {
		if p.Key == "staff.owner" && slices.Contains(p.Permissions, "module.widgets.read") {
			t.Fatal("seeding changed the panel's own presets")
		}
	}
}
