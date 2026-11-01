package db

import (
	"errors"
	"testing"
)

func TestTeamsLifecycle(t *testing.T) {
	d, ctx := testDB(t)
	a, _ := seed(t, d, ctx)
	b, _ := seed(t, d, ctx)
	e := AuditEntry{Actor: "u1", Action: "team.test"}
	id, err := d.CreateTeam(ctx, a.ID, "backend", "API folks", "u1", e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateTeam(ctx, a.ID, "backend", "", "u1", e); err == nil {
		t.Fatal("duplicate team name accepted")
	}
	if _, err := d.CreateTeam(ctx, a.ID, "Bad Name", "", "u1", e); err == nil {
		t.Fatal("invalid team name accepted")
	}
	if err := d.SetTeamMember(ctx, a.ID, id, "user-1", "maintainer", "u1", e); err != nil {
		t.Fatal(err)
	}
	if err := d.SetTeamMember(ctx, a.ID, id, "user-1", "member", "u1", e); err != nil {
		t.Fatal(err)
	}
	// Tenant B can't touch tenant A's team.
	if err := d.SetTeamMember(ctx, b.ID, id, "user-2", "member", "u1", e); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant add: %v", err)
	}
	if err := d.DeleteTeam(ctx, b.ID, id, e); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant delete: %v", err)
	}
	ts, err := d.Teams(ctx, a.ID)
	if err != nil || len(ts) != 1 || len(ts[0].Members) != 1 || ts[0].Members[0].Role != "member" {
		t.Fatalf("teams = %+v %v", ts, err)
	}
	if err := d.RemoveFromAllTeams(ctx, a.ID, "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveTeamMember(ctx, a.ID, id, "user-1", e); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove after purge: %v", err)
	}
	if err := d.DeleteTeam(ctx, a.ID, id, e); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action='team.test'`).Scan(&n)
	if n != 4 {
		t.Fatalf("audit rows = %d, want 4 (create, 2 sets, delete)", n)
	}
}
