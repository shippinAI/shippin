package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
)

func importCall(a *API, c *caller, tenant, body string) *httptest.ResponseRecorder {
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return c, true }
	r := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	r.SetPathValue("tenantId", tenant)
	w := httptest.NewRecorder()
	a.importRepo(w, r, c, c.grantedTenant(tenant))
	return w
}

func TestGitHubImportGuardsAndToken(t *testing.T) {
	a, d, tn, owner := githubAPI(t)
	ctx := context.Background()
	if err := d.BindInstallation(ctx, db.GitHubInstallation{ID: 7, TenantID: tn.ID, AccountLogin: "operator"}, ""); err != nil {
		t.Fatal(err)
	}
	b := db.Tenant{ID: uuid.NewString(), IdpOrgID: uuid.NewString(), Name: "B", Plan: "p"}
	_ = d.CreateTenant(ctx, b)
	intruder := &caller{id: &auth.Identity{Sub: "sub-b"}, managers: owner.managers, tenants: []tenantView{{Tenant: b, Roles: []string{"org_owner"}}}}
	if w := importCall(a, intruder, b.ID, `{"name":"stolen","installation_id":7,"repo_id":42}`); w.Code != 404 {
		t.Fatalf("tenant B imported tenant A's installation: %d %s", w.Code, w.Body)
	}
	if w := importCall(a, owner, tn.ID, `{"name":"app","installation_id":7,"repo_id":99}`); w.Code != 404 {
		t.Fatalf("repo the installation can't see: %d", w.Code)
	}
	if w := importCall(a, owner, tn.ID, `{"name":"app","installation_id":7,"repo_id":42,"branch":"../x"}`); w.Code != 400 {
		t.Fatalf("bad branch: %d", w.Code)
	}
	if w := importCall(a, owner, tn.ID, `{"installation_id":7,"repo_id":42}`); w.Code != 409 {
		t.Fatalf("no running devspace of the caller: %d %s", w.Code, w.Body)
	}
	ws := uuid.NewString()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO workspaces(id,tenant_id,name,ssh_port,created_by,state) VALUES($1,$2,'dev',40000+floor(random()*20000)::int,$3,'running')`, ws, tn.ID, owner.id.Sub); err != nil {
		t.Fatal(err)
	}
	var ops []map[string]any
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var op map[string]any
		_ = json.NewDecoder(r.Body).Decode(&op)
		ops = append(ops, op)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer host.Close()
	a.host = hostclient.New(host.URL)
	tw := &fakeTW{}
	a.n2.tw = tw
	w := importCall(a, owner, tn.ID, `{"installation_id":7,"repo_id":42}`)
	var out struct {
		ProjectID          string `json:"project_id"`
		Repo, Branch, Slug string
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 201 || out.Repo != "operator/app" || out.Branch != "main" || !strings.HasPrefix(out.Slug, "app") {
		t.Fatalf("import: %d %s", w.Code, w.Body)
	}
	if len(ops) != 1 || ops[0]["op"] != "clone" || ops[0]["path"] != "/home/dev/code/"+out.Slug || len(tw.got) != 7 {
		t.Fatalf("clone ops %v, tuples %d", ops, len(tw.got))
	}
	src, _ := d.SourceOf(ctx, out.ProjectID)
	if src == nil || src.RepoID != 42 || src.InstallationID != 7 {
		t.Fatalf("source %+v", src)
	}
	req := a.createReq(ctx, &tenantView{Tenant: tn}, db.Workspace{ProjectID: out.ProjectID})
	if req.GitToken == nil || req.GitToken.Token != "ghs_test" {
		t.Fatal("imported workspace gets an installation token on create")
	}
	if a.createReq(ctx, &tenantView{Tenant: tn}, db.Workspace{}).GitToken != nil {
		t.Fatal("non-imported workspaces get no token")
	}
	_ = d.RevokeRepos(ctx, 7, []int64{42})
	if a.gitTokenFor(ctx, out.ProjectID) != nil {
		t.Fatal("revoked source still mints tokens")
	}
}
