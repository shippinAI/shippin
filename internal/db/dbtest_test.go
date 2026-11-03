package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// testDB opens SHIPPIN_TEST_DATABASE_URL (a disposable database; the schema is dropped and
// re-migrated) or skips. Never point it at a real database.
func testDB(t *testing.T) (*DB, context.Context) {
	t.Helper()
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	d, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := d.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	return d, ctx
}

func newID() string { id, _ := uuid.NewV7(); return id.String() }

// seed creates an active tenant with one workspace.
func seed(t *testing.T, d *DB, ctx context.Context) (Tenant, Workspace) {
	t.Helper()
	tn := Tenant{ID: newID(), IdpOrgID: newID(), Name: "t", Plan: "p"}
	if err := d.CreateTenant(ctx, tn); err != nil {
		t.Fatal(err)
	}
	if err := d.SetTenantStatus(ctx, tn.ID, "active", "", ""); err != nil {
		t.Fatal(err)
	}
	ws := Workspace{ID: newID(), TenantID: tn.ID, Name: "w", CreatedBy: "owner", Host: newID() + ".ws.test"}
	if err := d.CreateWorkspace(ctx, &ws, 22000, 22999); err != nil {
		t.Fatal(err)
	}
	return tn, ws
}

func reqGrant(tn Tenant, ws, who string, status string, secs int) Grant {
	now := time.Now()
	return Grant{ID: newID(), TenantID: tn.ID, WorkspaceID: ws, Kind: "request", Requester: who, RequesterName: who,
		Reason: "testing access", DurationS: secs, Status: status, NoticeAt: &now}
}
