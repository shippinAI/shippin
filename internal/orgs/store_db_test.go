package orgs

import (
	"context"
	"os"
	"testing"
	"time"

	"shippin.cloud/console/internal/db"
)

// Needs SHIPPIN_TEST_DATABASE_URL (disposable: the schema is dropped and re-migrated).
func testStore(t *testing.T) (*Store, context.Context, *time.Time) {
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	d.Pool.Close()
	if d, err = db.Open(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	return &Store{Pool: d.Pool, Now: func() time.Time { return now }}, ctx, &now
}

func TestSingleOperationsOrg(t *testing.T) {
	s, ctx, _ := testStore(t)
	a, err := s.EnsureOperations(ctx, "operations", "Operations", "")
	if err != nil || a.Type != TypeOperations {
		t.Fatal(a, err)
	}
	b, err := s.EnsureOperations(ctx, "other-ops", "Other", "")
	if err != nil || b.ID != a.ID {
		t.Fatal("second ensure must return the same org", b, err)
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO orgs(id, slug, type, name) VALUES('org_01JZZZZZZZZZZZZZZZZZZZZZZZ','x','operations','X')`)
	if err == nil {
		t.Fatal("database allowed a second Operations org")
	}
	c, err := s.EnsureCustomer(ctx, "nomen-org-1", "builder", "Builder")
	if err != nil || c.Type != TypeCustomer {
		t.Fatal(c, err)
	}
	if c2, _ := s.EnsureCustomer(ctx, "nomen-org-1", "builder", "Builder"); c2.ID != c.ID {
		t.Fatal("customer org not stable")
	}
	f, err := s.AddFolder(ctx, c.ID, "", "Products")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFolder(ctx, c.ID, f, "rrflow"); err != nil {
		t.Fatal("nested folder", err)
	}
}

func TestStaffRolesExpireAndRevoke(t *testing.T) {
	s, ctx, now := testStore(t)
	exp := now.Add(time.Hour)
	if err := s.GrantStaffRole(ctx, "sub-a", RoleOncall, "bootstrap", &exp); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantStaffRole(ctx, "sub-a", "org_owner", "x", nil); err == nil {
		t.Fatal("tenant role accepted as staff role")
	}
	if r, _ := s.StaffRolesOf(ctx, "sub-a"); len(r) != 1 || !CanReadPlatform(r) || CanAdminPlatform(r) {
		t.Fatal(r)
	}
	*now = now.Add(2 * time.Hour)
	if r, _ := s.StaffRolesOf(ctx, "sub-a"); len(r) != 0 {
		t.Fatal("expired role still active", r)
	}
}

func TestSupportGrantsAreBoundedAndRevocable(t *testing.T) {
	s, ctx, now := testStore(t)
	ops, _ := s.EnsureOperations(ctx, "operations", "Operations", "")
	c, _ := s.EnsureCustomer(ctx, "nomen-org-1", "builder", "Builder")
	max := 4 * time.Hour
	if _, err := s.GrantSupport(ctx, "sub-s", c.ID, "", "customer ticket 42: build fails", "sub-admin", 5*time.Hour, max); err != ErrTooLong {
		t.Fatal("over-long grant accepted", err)
	}
	if _, err := s.GrantSupport(ctx, "sub-s", ops.ID, "", "should never target ops", "sub-admin", time.Hour, max); err != ErrBadInput {
		t.Fatal("grant on Operations accepted", err)
	}
	if _, err := s.GrantSupport(ctx, "sub-s", c.ID, "", "short", "sub-admin", time.Hour, max); err != ErrBadInput {
		t.Fatal("reason too short accepted", err)
	}
	g, err := s.GrantSupport(ctx, "sub-s", c.ID, "", "customer ticket 42: build fails", "sub-admin", time.Hour, max)
	if err != nil || !g.Active(*now) {
		t.Fatal(g, err)
	}
	if ok, _ := s.HasSupport(ctx, "sub-s", c.ID); !ok {
		t.Fatal("grant not active")
	}
	*now = now.Add(61 * time.Minute)
	if ok, _ := s.HasSupport(ctx, "sub-s", c.ID); ok {
		t.Fatal("expired grant still active")
	}
	*now = now.Add(-61 * time.Minute)
	if r, err := s.RevokeSupport(ctx, g.ID); err != nil || r.RevokedAt == nil {
		t.Fatal(r, err)
	}
	if ok, _ := s.HasSupport(ctx, "sub-s", c.ID); ok {
		t.Fatal("revoked grant still active")
	}
	if l, _ := s.SupportGrants(ctx, 10); len(l) != 1 {
		t.Fatal(l)
	}
}
