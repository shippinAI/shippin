package jobs

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
	"testing"
	"time"
)

func TestProvisionCannotOverwriteNewerLifecycle(t *testing.T) {
	s, d, tid := testJobs(t)
	ctx := context.Background()
	ws := &db.Workspace{ID: uuid.NewString(), TenantID: tid, Name: "test", CreatedBy: "u", Host: "test.example"}
	if e := d.CreateWorkspace(ctx, ws, 22000, 22100); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.Enqueue(ctx, job(tid, ws.ID)); e != nil {
		t.Fatal(e)
	}
	first, e := s.Claim(ctx, "old", []string{KindWorkspaceProvision}, time.Minute)
	if e != nil || first == nil {
		t.Fatal(e)
	}
	if _, e := d.Pool.Exec(ctx, `UPDATE jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, first.ID); e != nil {
		t.Fatal(e)
	}
	next, e := s.Claim(ctx, "new", []string{KindWorkspaceProvision}, time.Minute)
	if e != nil || next == nil {
		t.Fatal(e)
	}
	for _, state := range []string{"running", "failed"} {
		if e := s.FinishProvision(ctx, first, state, "", ""); !errors.Is(e, ErrLostLease) {
			t.Fatal("stale provision", e)
		}
	}
	if e := s.FinishProvision(ctx, next, "running", "10.0.0.2", ""); e != nil {
		t.Fatal(e)
	}
	for _, newState := range []string{"starting", "stopping", "deleting", "deleted"} {
		if e := d.SetWorkspaceState(ctx, ws.ID, newState, "", ""); e != nil {
			t.Fatal(e)
		}
		for _, state := range []string{"running", "failed"} {
			if e := s.FinishProvision(ctx, next, state, "", ""); !errors.Is(e, ErrLostLease) {
				t.Fatal("unexpected provisioning state accepted", e)
			}
		}
		var got string
		if e := d.Pool.QueryRow(ctx, `SELECT state FROM workspaces WHERE id=$1`, ws.ID).Scan(&got); e != nil {
			t.Fatal(e)
		}
		if got != newState {
			t.Fatal("new state overwritten")
		}
	}
}
