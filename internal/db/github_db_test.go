package db

import (
	"testing"
)

func TestGitHubInstallationsAndSources(t *testing.T) {
	d, ctx := testDB(t)
	a, _ := seed(t, d, ctx)
	b := Tenant{ID: newID(), IdpOrgID: newID(), Name: "b", Plan: "p"}
	if err := d.CreateTenant(ctx, b); err != nil {
		t.Fatal(err)
	}
	in := GitHubInstallation{ID: 77, TenantID: a.ID, AccountLogin: "operator", AccountType: "User", RepoSelection: "selected"}
	if err := d.BindInstallation(ctx, in, ""); err != nil {
		t.Fatal(err)
	}
	stolen := in
	stolen.TenantID = b.ID
	if err := d.BindInstallation(ctx, stolen, ""); err != ErrInstallationTaken {
		t.Fatalf("a live installation must not move tenants: %v", err)
	}
	if got, _ := d.Installations(ctx, b.ID); len(got) != 0 {
		t.Fatal("tenant B sees tenant A's installation")
	}
	pid := newID()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO projects(id, tenant_id, name, created_by) VALUES ($1,$2,'app','u')`, pid, a.ID); err != nil {
		t.Fatal(err)
	}
	src := ProjectSource{ProjectID: pid, TenantID: a.ID, Kind: "github", InstallationID: 77, RepoID: 5, RepoFullName: "operator/app", Branch: "main"}
	if err := d.SetSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	bad := src
	bad.Branch = "../x"
	if d.SetSource(ctx, bad) == nil {
		t.Fatal("branch with .. accepted")
	}
	_ = d.RenameRepo(ctx, 5, "operator/app2")
	_ = d.RevokeRepos(ctx, 77, []int64{5})
	if s, _ := d.SourceOf(ctx, pid); s == nil || s.Status != "revoked" || s.RepoFullName != "operator/app2" {
		t.Fatalf("source %+v", s)
	}
	_ = d.SyncRepoAccess(ctx, 77, []int64{5, 6})
	if s, _ := d.SourceOf(ctx, pid); s.Status != "ok" {
		t.Fatal("repo listed again restores access")
	}
	_ = d.SyncRepoAccess(ctx, 77, []int64{6})
	if s, _ := d.SourceOf(ctx, pid); s.Status != "revoked" {
		t.Fatal("repo no longer listed revokes access")
	}
	_ = d.SetInstallationState(ctx, 77, "deleted")
	if got, _ := d.Installations(ctx, a.ID); len(got) != 0 {
		t.Fatal("deleted installation still listed")
	}
	if err := d.BindInstallation(ctx, stolen, ""); err != nil {
		t.Fatalf("a deleted installation may be re-installed elsewhere: %v", err)
	}
	if fresh, _ := d.RecordDelivery(ctx, "abc-1"); !fresh {
		t.Fatal("first delivery")
	}
	if again, _ := d.RecordDelivery(ctx, "abc-1"); again {
		t.Fatal("replayed delivery accepted")
	}
}
