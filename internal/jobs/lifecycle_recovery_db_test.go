package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
	"testing"
)

func TestSameRunnerReclaimAndFinalReceipt(t *testing.T) {
	for _, state := range []string{"running", "failed"} {
		t.Run(state, func(t *testing.T) {
			s, d, tid := testJobs(t)
			ctx := context.Background()
			ws := &db.Workspace{ID: uuid.NewString(), TenantID: tid, Name: "test", CreatedBy: "u", Host: "test.example"}
			if e := d.CreateWorkspace(ctx, ws, 22000, 22100); e != nil {
				t.Fatal(e)
			}
			if e := d.SetWorkspaceState(ctx, ws.ID, "stopped", "", ""); e != nil {
				t.Fatal(e)
			}
			if _, e := s.EnqueueAction(ctx, Job{TenantID: tid, TargetID: ws.ID, CreatedBy: "u", MaxAttempts: 1, Payload: json.RawMessage(`{"action":"start"}`)}, "starting"); e != nil {
				t.Fatal(e)
			}
			r := NewRunner(s, fast)
			first, e := r.claim(ctx, []string{KindWorkspaceAction})
			if e != nil || first == nil {
				t.Fatal(e)
			}
			message := ""
			if state == "failed" {
				message = "provider outcome uncertain"
			}
			if e := s.FinishAction(ctx, first, state, message); e != nil {
				t.Fatal(e)
			}
			if _, e := d.Pool.Exec(ctx, `UPDATE jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, first.ID); e != nil {
				t.Fatal(e)
			}
			next, e := r.claim(ctx, []string{KindWorkspaceAction})
			if e != nil || next == nil {
				t.Fatal(e)
			}
			if first.LeaseOwner == next.LeaseOwner || next.Attempts != 2 {
				t.Fatal("claim token/budget", next)
			}
			if e := s.ActionPhase(ctx, first, "dispatched"); !errors.Is(e, ErrLostLease) {
				t.Fatal("stale phase", e)
			}
			if e := s.FinishAction(ctx, first, "deleted", ""); !errors.Is(e, ErrLostLease) {
				t.Fatal("stale completion", e)
			}
			if e := s.Succeed(ctx, first.ID, first.LeaseOwner, nil); !errors.Is(e, ErrLostLease) {
				t.Fatal("stale success", e)
			}
			r.Register(KindWorkspaceAction, func(context.Context, *Job, func(string)) (any, error) {
				t.Error("receipt replay dispatched handler")
				return nil, nil
			})
			r.runOne(ctx, next)
			saved, e := s.Get(ctx, next.ID)
			if e != nil {
				t.Fatal(e)
			}
			want := Succeeded
			if state == "failed" {
				want = Failed
			}
			if saved.Status != want {
				t.Fatalf("%+v", saved)
			}
			row, e := d.WorkspaceByID(ctx, ws.ID)
			if e != nil || row.State != state {
				t.Fatalf("workspace=%+v err=%v", row, e)
			}
		})
	}
}
func TestActionReceiptSurvivesResponseSaveFailure(t *testing.T) {
	s, d, tid := testJobs(t)
	ctx := context.Background()
	scope, key := tid+":u", uuid.NewString()
	ws := &db.Workspace{ID: uuid.NewString(), TenantID: tid, Name: "test", CreatedBy: "u", Host: "test.example"}
	if e := d.CreateWorkspace(ctx, ws, 22000, 22100); e != nil {
		t.Fatal(e)
	}
	if e := d.SetWorkspaceState(ctx, ws.ID, "running", "", ""); e != nil {
		t.Fatal(e)
	}
	if _, e := d.ClaimIdemKey(ctx, scope, key, "hash", fast.Lease); e != nil {
		t.Fatal(e)
	}
	j, e := s.EnqueueAction(ctx, Job{TenantID: tid, TargetID: ws.ID, CreatedBy: "u", MaxAttempts: 1, Payload: json.RawMessage(`{"action":"restart"}`)}, "starting", ActionAcceptance{Scope: scope, Key: key, Response: func(j *Job) ([]byte, error) { return json.Marshal(map[string]string{"job": j.ID}) }})
	if e != nil {
		t.Fatal(e)
	}
	// Simulate a post-commit response-save error and middleware cleanup. The durable
	// transaction already owns the response, so cleanup must not free the key.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e := d.SaveIdemKey(canceled, scope, key, 202, nil, nil); e == nil {
		t.Fatal("expected canceled save")
	}
	if e := d.DropIdemKey(ctx, scope, key); e != nil {
		t.Fatal(e)
	}
	if _, e := d.Pool.Exec(ctx, `UPDATE jobs SET status='succeeded' WHERE id=$1`, j.ID); e != nil {
		t.Fatal(e)
	}
	if e := d.SetWorkspaceState(ctx, ws.ID, "running", "", ""); e != nil {
		t.Fatal(e)
	}
	rec, e := d.ClaimIdemKey(ctx, scope, key, "hash", fast.Lease)
	if e != nil || rec == nil || rec.Status != 202 {
		t.Fatalf("receipt=%+v err=%v", rec, e)
	}
	var body map[string]string
	if json.Unmarshal(rec.Body, &body) != nil || body["job"] != j.ID {
		t.Fatal("lost operation identity")
	}
	// A missing reservation rolls back both workspace claim and job insertion.
	_, e = s.EnqueueAction(ctx, Job{TenantID: tid, TargetID: ws.ID, CreatedBy: "u", MaxAttempts: 1, Payload: json.RawMessage(`{"action":"restart"}`)}, "starting", ActionAcceptance{Scope: scope, Key: uuid.NewString(), Response: func(j *Job) ([]byte, error) { return []byte(`{}`), nil }})
	if e == nil {
		t.Fatal("missing reservation accepted")
	}
	row, _ := d.WorkspaceByID(ctx, ws.ID)
	if row.State != "running" {
		t.Fatal("transaction did not roll back")
	}
}
