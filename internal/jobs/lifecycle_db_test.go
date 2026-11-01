package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
	"testing"
	"time"
)

func TestActionAtomicAcceptanceAndLeaseFence(t *testing.T) {
	s, d, tid := testJobs(t)
	ctx := context.Background()
	ws := &db.Workspace{ID: uuid.NewString(), TenantID: tid, Name: "test", CreatedBy: "u", Host: "test.example"}
	if err := d.CreateWorkspace(ctx, ws, 22000, 22100); err != nil {
		t.Fatal(err)
	}
	if err := d.SetWorkspaceState(ctx, ws.ID, "stopped", "", ""); err != nil {
		t.Fatal(err)
	}
	j := Job{TenantID: tid, TargetID: ws.ID, CreatedBy: "u", MaxAttempts: 0, Payload: json.RawMessage(`{"action":"start"}`)}
	if _, err := s.EnqueueAction(ctx, j, "starting"); err == nil {
		t.Fatal("invalid job accepted")
	}
	got, _ := d.WorkspaceByID(ctx, ws.ID)
	if got.State != "stopped" {
		t.Fatal("claim did not roll back")
	}
	j.MaxAttempts = 3
	accepted, err := s.EnqueueAction(ctx, j, "starting")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnqueueAction(ctx, j, "starting"); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("duplicate: %v", err)
	}
	claimed, err := s.Claim(ctx, "worker", []string{KindWorkspaceAction}, time.Minute)
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	if claimed.ID != accepted.ID || claimed.LeaseOwner != "worker" {
		t.Fatal("wrong claim")
	}
	if err := s.ActionPhase(ctx, claimed, "dispatched"); err != nil {
		t.Fatal(err)
	}
	claimed.LeaseOwner = "stale"
	if err := s.ActionPhase(ctx, claimed, "acknowledged"); !errors.Is(err, ErrLostLease) {
		t.Fatalf("stale dispatch: %v", err)
	}
	if err := s.FinishAction(ctx, claimed, "running", ""); !errors.Is(err, ErrLostLease) {
		t.Fatalf("stale write: %v", err)
	}
	claimed.LeaseOwner = "worker"
	if _, err := d.Pool.Exec(ctx, `UPDATE jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ActionPhase(ctx, claimed, "acknowledged"); !errors.Is(err, ErrLostLease) {
		t.Fatalf("expired dispatch: %v", err)
	}
	if err := s.FinishAction(ctx, claimed, "running", ""); !errors.Is(err, ErrLostLease) {
		t.Fatalf("expired completion: %v", err)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE jobs SET lease_until=now()+interval '1 minute' WHERE id=$1`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishAction(ctx, claimed, "running", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnqueueAction(ctx, j, "starting"); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("live job not excluded: %v", err)
	}
	saved, _ := s.Get(ctx, claimed.ID)
	if !json.Valid(saved.Payload) {
		t.Fatal("invalid receipt")
	}
	got, _ = d.WorkspaceByID(ctx, ws.ID)
	if got.State != "running" {
		t.Fatal("completion missing")
	}

	if err := s.Succeed(ctx, claimed.ID, "worker", nil); err != nil {
		t.Fatal(err)
	}
	if err := d.SetWorkspaceState(ctx, ws.ID, "stopping", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.ReconcileWorkspaceActions(ctx, 0); err != nil {
		t.Fatal(err)
	}
	got, _ = d.WorkspaceByID(ctx, ws.ID)
	if got.State != "failed" {
		t.Fatal("abandoned action not surfaced")
	}
}
