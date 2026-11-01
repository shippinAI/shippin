package db

import (
	"errors"
	"testing"
)

func TestStaffSeedOnceAndRolesAreData(t *testing.T) {
	d, ctx := testDB(t)
	for i := 0; i < 2; i++ {
		if err := d.SeedStaff(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := d.StaffRoles(ctx)
	if err != nil || len(rs) != len(StaffPresets) {
		t.Fatalf("presets: %d %v", len(rs), err)
	}
	e := AuditEntry{Actor: "u1", ActorName: "U", Action: "team.role_delete"}
	if err := d.DeleteStaffRole(ctx, "staff.billing", e); err != nil {
		t.Fatal(err)
	}
	if err := d.SeedStaff(ctx); err != nil {
		t.Fatal(err)
	}
	if rs, _ := d.StaffRoles(ctx); len(rs) != len(StaffPresets)-1 {
		t.Fatal("a deleted preset must not come back on restart")
	}
	r := StaffRole{Key: "staff.dns", Name: "DNS", Permissions: []string{"dns.read"}}
	if err := d.SaveStaffRole(ctx, r, true, e); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveStaffRole(ctx, r, true, e); !errors.Is(err, ErrNotFound) {
		t.Fatalf("duplicate key: %v", err)
	}
	r.Name, r.Permissions = "DNS editors", []string{"dns.read", "dns.write"}
	if err := d.SaveStaffRole(ctx, r, false, e); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveStaffRole(ctx, StaffRole{Key: "BAD KEY", Name: "x"}, true, e); err == nil {
		t.Fatal("the key format is enforced by the table too")
	}
	es, _ := d.AllAudit(ctx, "", 50)
	if len(es) != 3 {
		t.Fatalf("each change commits with its audit entry: %d entries", len(es))
	}
}

func TestStaffSettingsAndSeen(t *testing.T) {
	d, ctx := testDB(t)
	if s, err := d.StaffSettings(ctx); err != nil || s != DefaultStaffSettings {
		t.Fatalf("defaults without a row: %+v %v", s, err)
	}
	s := DefaultStaffSettings
	s.Mode, s.IdleDays = "enforce", 30
	if err := d.SaveStaffSettings(ctx, s, "u1", AuditEntry{Actor: "u1", ActorName: "U", Action: "team.settings"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.StaffSettings(ctx); got != s {
		t.Fatalf("settings round trip: %+v", got)
	}
	if err := d.TouchStaff(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if m, _ := d.StaffSeen(ctx); m["u1"].IsZero() {
		t.Fatal("last activity not recorded")
	}
	q := StaffRequest{ID: newID(), Kind: "roles", Target: "u2", Payload: []byte(`{}`), Reason: "r", RequestedBy: "u1", RequestedName: "U"}
	if err := d.CreateStaffRequest(ctx, q); err != nil {
		t.Fatal(err)
	}
	if err := d.DecideStaffRequest(ctx, q.ID, "approved", "u3"); err != nil {
		t.Fatal(err)
	}
	if err := d.DecideStaffRequest(ctx, q.ID, "rejected", "u3"); !errors.Is(err, ErrNotFound) {
		t.Fatal("a decided request can't be decided again")
	}
}
