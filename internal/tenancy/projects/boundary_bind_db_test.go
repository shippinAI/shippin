package projects

import (
	"encoding/json"
	"testing"
)

// A planned boundary bound to the tenant VM becomes ready and takes the pending projects;
// the same VM can't back a second boundary.
func TestBindVM(t *testing.T) {
	s, ctx, tid := testStore(t)
	pid, err := s.CreateProject(ctx, tid, "site", "dev", "sub-1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.PlanBoundary(ctx, tid, 4, 8192, 40, 100)
	if err != nil {
		t.Fatal(err)
	}
	planned, err := s.PlannedBoundaries(ctx)
	if err != nil || len(planned) != 1 || planned[0].ID != b || planned[0].Tenant != tid {
		t.Fatalf("planned: %v %v", planned, err)
	}
	if err := s.SetBoundaryState(ctx, tid, b, "provisioning", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.BindVM(ctx, tid, b, "tvm-x", "16", "32GiB", "100GiB", "1TiB"); err != nil {
		t.Fatal(err)
	}
	state, vm, err := s.BoundaryState(ctx, tid, b)
	if err != nil || state != "ready" || vm != "tvm-x" {
		t.Fatalf("after bind: %s %s %v", state, vm, err)
	}
	var p struct{ State string }
	raw, _ := s.Project(ctx, tid, pid)
	_ = json.Unmarshal(raw, &p)
	if p.State != "ready" {
		t.Fatalf("pending project not placed: %s", raw)
	}
	b2, _ := s.PlanBoundary(ctx, tid, 4, 8192, 40, 100)
	if err := s.BindVM(ctx, tid, b2, "tvm-x", "16", "32GiB", "100GiB", "1TiB"); err != ErrVMTaken {
		t.Fatalf("second boundary on the same VM: %v", err)
	}
	if err := s.BindVM(ctx, tid, b, "tvm-x", "16", "32GiB", "100GiB", "1TiB"); err != ErrNotFound {
		t.Fatalf("rebinding a ready boundary: %v", err)
	}
	if _, _, err := s.BoundaryState(ctx, "00000000-0000-0000-0000-000000000000", b); err != ErrNotFound {
		t.Fatalf("other tenant sees the boundary: %v", err)
	}
}
