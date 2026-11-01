package api

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/authz"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/projects"
)

type fakeTW struct {
	err error
	got []authz.Tuple
}

func (f *fakeTW) Write(_ context.Context, ts []authz.Tuple) error {
	f.got = append(f.got, ts...)
	return f.err
}

// A failed ownership write leaves no project behind; a good one writes the 8-tuple chain with
// the Nomen org and owner, and a folder in an already-bound devspace doesn't queue rrflow again.
func TestCreateOwnedTuplesOrUndo(t *testing.T) {
	a, d, tn := notifyAPI(t)
	ctx := context.Background()
	ws := uuid.NewString()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO workspaces(id,tenant_id,name,ssh_port,created_by,state) VALUES($1,$2,'dev',40000+floor(random()*20000)::int,'t','running')`, ws, tn.ID); err != nil {
		t.Fatal(err)
	}
	st := &projects.Store{Pool: d.Pool}
	in := ownedReq{TenantID: tn.ID, WorkspaceID: ws, OwnerSub: "395290028818825218", By: "op",
		Found: hostclient.Found{Name: "Box_Work", Path: "/home/dev/Box_Work", Git: true, Remote: "https://github.com/x/box-work.git"}}

	if _, err := a.createOwned(ctx, in); !errors.Is(err, errNoTupleWriter) {
		t.Fatalf("no writer: %v", err)
	}
	a.n2.tw = &fakeTW{err: errors.New("fga down")}
	if _, err := a.createOwned(ctx, in); err == nil {
		t.Fatal("tuple failure must fail the create")
	}
	if taken, _ := st.SlugTaken(ctx, tn.ID, "box-work"); taken {
		t.Fatal("project kept without ownership tuples")
	}

	legacy, err := st.CreateProject(ctx, tn.ID, "legacy", "dev", "x")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = d.Pool.Exec(ctx, `UPDATE workspaces SET project_id=$1 WHERE id=$2`, legacy, ws)
	tw := &fakeTW{}
	a.n2.tw = tw
	res, err := a.createOwned(ctx, in)
	if err != nil || res.Slug != "box-work" || res.RRFlow != "unchanged" {
		t.Fatalf("create: %+v %v", res, err)
	}
	// the devspace was already bound (legacy): no devspace link; and never an org role.
	want := map[string]bool{"project:" + res.Project.ID + "#owner@user:395290028818825218": true,
		"project:" + res.Project.ID + "#org@org:" + tn.IdpOrgID: true}
	for _, x := range tw.got {
		delete(want, x.Object+"#"+x.Relation+"@"+x.User)
		if (x.Object == "org:"+tn.IdpOrgID && x.Relation != "tenant") || x.Object == "devspace:"+ws {
			t.Errorf("unexpected tuple %v", x)
		}
	}
	if len(tw.got) != 5 || len(want) != 0 {
		t.Fatalf("tuples %d, missing %v", len(tw.got), want)
	}
	res2, err := a.createOwned(ctx, in)
	if err == nil {
		t.Fatalf("same folder twice: %+v", res2)
	}
	other := in
	other.Found = hostclient.Found{Name: "x", Path: "/home/dev/x"}
	other.RequireDevspaceOwner = true // devspace created_by is 't', not the adopter
	if _, err := a.createOwned(ctx, other); !errors.Is(err, projects.ErrNotDevspaceOwner) {
		t.Fatalf("adopt from someone else's devspace: %v", err)
	}
}
