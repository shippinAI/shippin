package projects

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
)

// Needs SHIPPIN_TEST_DATABASE_URL (disposable: the schema is dropped and re-migrated).
func testStore(t *testing.T) (*Store, context.Context, string) {
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	d.Pool.Close()
	if d, err = db.Open(ctx, url); err != nil {
		t.Fatal(err)
	}
	tid := uuid.NewString()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan) VALUES($1,$2,'operator','founder')`, tid, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	return &Store{Pool: d.Pool}, ctx, tid
}

func TestProjectLifecycle(t *testing.T) {
	s, ctx, tid := testStore(t)
	id, err := s.CreateProject(ctx, tid, "site-dev", "dev", "sub-1")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		State, Name  string
		Placed       bool
		Environments []struct{ Name string }
	}
	raw, _ := s.Project(ctx, tid, id)
	_ = json.Unmarshal(raw, &p)
	if p.State != "pending" || p.Placed || len(p.Environments) != 1 || p.Environments[0].Name != "dev" {
		t.Fatalf("no boundary yet: %s", raw)
	}
	if _, err := s.CreateProject(ctx, tid, "site-dev", "dev", "sub-1"); err != ErrNameTaken {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := s.Place(ctx, tid, id, "dev"); err == nil {
		t.Fatal("pending project must not take workspaces")
	}
	b, err := s.PlanBoundary(ctx, tid, 16, 32768, 20, 150)
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := s.CreateProject(ctx, tid, "site", "dev", "sub-1")
	if _, err := s.Place(ctx, tid, id2, "dev"); err == nil {
		t.Fatal("a project on a planned boundary must stay pending")
	}
	if err := s.MarkBoundaryReady(ctx, tid, b); err != nil {
		t.Fatal(err)
	}
	pl, err := s.Place(ctx, tid, id2, "dev")
	if err != nil || pl.Boundary == nil || *pl.Boundary != b {
		t.Fatalf("auto-placement: %v %+v", err, pl)
	}
	if _, err := s.Project(ctx, uuid.NewString(), id2); err != ErrNotFound {
		t.Fatal("other tenant must not see the project")
	}
	if _, err := s.AddEnvironment(ctx, tid, id2, "prod"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddEnvironment(ctx, tid, id2, "prod"); err != ErrNameTaken {
		t.Fatalf("duplicate environment: %v", err)
	}
	if _, err := s.AddEnvironment(ctx, tid, uuid.NewString(), "prod"); err != ErrNotFound {
		t.Fatalf("unknown project: %v", err)
	}
	if err := s.DeleteProject(ctx, tid, id2); err != nil {
		t.Fatal(err)
	}
	if raw, _ := s.Projects(ctx, tid); string(raw) == "[]" {
		t.Fatal("site-dev should still list")
	}
}

func TestAdoptVM(t *testing.T) {
	s, ctx, tid := testStore(t)
	pj, _ := s.CreateProject(ctx, tid, "site-dev", "dev", "sub-1")
	b1, err := s.AdoptVM(ctx, tid, "tvm-"+tid, "16", "32GiB", "20GiB", "150GiB")
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := s.AdoptVM(ctx, tid, "tvm-"+tid, "16", "32GiB", "20GiB", "150GiB")
	if b1 != b2 {
		t.Fatal("adopt must be idempotent")
	}
	if pl, err := s.Place(ctx, tid, pj, "dev"); err != nil || pl.Boundary == nil || *pl.Boundary != b1 {
		t.Fatalf("pending project not placed: %v", err)
	}
	var mem int
	_ = s.Pool.QueryRow(ctx, `SELECT memory_mib FROM boundaries WHERE id=$1`, b1).Scan(&mem)
	if mem != 32768 {
		t.Fatalf("memory %d", mem)
	}
}

// 015: an environment's tenant must equal its project's tenant.
func TestEnvironmentTenantMatchesProject(t *testing.T) {
	s, ctx, tid := testStore(t)
	pj, err := s.CreateProject(ctx, tid, "app", "dev", "sub-1")
	if err != nil {
		t.Fatal(err)
	}
	other := uuid.NewString()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan) VALUES($1,$2,'x','founder')`, other, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO environments(id,project_id,tenant_id,name) VALUES($1,$2,$3,'prod')`,
		uuid.NewString(), pj, other); err == nil {
		t.Fatal("environment with a foreign tenant accepted")
	}
}
