package projects

import (
	"testing"

	"github.com/google/uuid"
)

// Adopt two folders from one devspace: the first binds it, the second shares it; slugs are
// org-unique, the same folder can't be adopted twice, and Undo leaves nothing behind.
func TestCreateOperatorAdoptFlow(t *testing.T) {
	s, ctx, tid := testStore(t)
	ws := uuid.NewString()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO workspaces(id,tenant_id,name,ssh_port,created_by,state) VALUES($1,$2,'dev',42001,'t','running')`, ws, tid); err != nil {
		t.Fatal(err)
	}
	mk := func(slug, path string) (*Created, error) {
		return s.CreateOperator(ctx, OpProject{TenantID: tid, Slug: slug, OwnerSub: "395290028818825218", WorkspaceID: ws,
			CreatedBy: "op", SourcePath: path, Limits: Limits{CPU: "2", Mem: "4GiB"}, CostCenter: "operator"})
	}
	a, err := mk("box-work", "/home/dev/box-work")
	if err != nil || !a.BoundDevspace || !ValidPublicID(a.PublicID) {
		t.Fatalf("first: %+v %v", a, err)
	}
	b, err := mk("notes-site", "/home/dev/notes-site")
	if err != nil || b.BoundDevspace {
		t.Fatalf("second must share the devspace: %+v %v", b, err)
	}
	if _, err := mk("box-work-2", "/home/dev/box-work"); err != ErrNameTaken {
		t.Fatalf("same folder twice: %v", err)
	}
	if taken, _ := s.SlugTaken(ctx, tid, "notes-site"); !taken {
		t.Fatal("notes-site should be taken in the org")
	}
	if got, _ := s.AdoptedPaths(ctx, tid); got["/home/dev/box-work"] != a.ID || got["/home/dev/notes-site"] != b.ID || len(got) != 2 {
		t.Fatalf("adopted: %v", got)
	}
	var bound string
	_ = s.Pool.QueryRow(ctx, `SELECT project_id::text FROM workspaces WHERE id=$1`, ws).Scan(&bound)
	if bound != a.ID {
		t.Fatalf("devspace bound to %s, want %s", bound, a.ID)
	}
	if _, err := s.CreateOperator(ctx, OpProject{TenantID: tid, Slug: "vm-one", WorkspaceID: ws, Placement: "bare-metal"}); err != ErrPlacement {
		t.Fatalf("placement: %v", err)
	}
	if err := s.UndoOperator(ctx, tid, a.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM workspaces WHERE id=$1 AND project_id IS NULL`, ws).Scan(&n)
	if taken, _ := s.SlugTaken(ctx, tid, "box-work"); taken || n != 1 {
		t.Fatalf("undo left state: taken=%v unbound=%d", taken, n)
	}
}
